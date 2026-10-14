package matcher

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/presenter"
)

const (
	// reenqueueMaxAttempts は発行に失敗したペアをキューへ書き戻す試行回数の上限。
	reenqueueMaxAttempts = 5
	// reenqueueInitialBackoff は 1 回目の書き戻しに失敗したあとの待ち時間。以降は失敗のたびに倍にする。
	reenqueueInitialBackoff = 100 * time.Millisecond
	reenqueueBackoffFactor  = 2
	// shutdownReenqueueTimeout はシャットダウン中の最後の書き戻しに許す時間。
	shutdownReenqueueTimeout = 2 * time.Second
)

// Matcher はマッチメイキングループとサーキットブレーカーを管理します。
type Matcher struct {
	queue        port.Queue
	publisher    port.RawEventPublisher
	clock        port.Clock
	matchIDs     port.MatchIDGenerator
	interval     time.Duration
	circuit      *circuitBreaker
	drainTimeout time.Duration
}

// Options は Matcher の動作パラメータです。
type Options struct {
	Interval         time.Duration
	CircuitThreshold int
	CircuitCooldown  time.Duration
	DrainTimeout     time.Duration
}

func (o Options) validate() error {
	if o.Interval <= 0 {
		return fmt.Errorf("matcher: Interval must be > 0, got %s", o.Interval)
	}
	if o.CircuitThreshold <= 0 {
		return fmt.Errorf("matcher: CircuitThreshold must be > 0, got %d", o.CircuitThreshold)
	}
	if o.CircuitCooldown <= 0 {
		return fmt.Errorf("matcher: CircuitCooldown must be > 0, got %s", o.CircuitCooldown)
	}
	if o.DrainTimeout <= 0 {
		return fmt.Errorf("matcher: DrainTimeout must be > 0, got %s", o.DrainTimeout)
	}
	return nil
}

// New は Matcher を生成します。opts に 0 以下の値があるときはエラーを返します。
func New(queue port.Queue, publisher port.RawEventPublisher, clock port.Clock, matchIDs port.MatchIDGenerator, opts Options) (*Matcher, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	return &Matcher{
		queue:        queue,
		publisher:    publisher,
		clock:        clock,
		matchIDs:     matchIDs,
		interval:     opts.Interval,
		circuit:      newCircuitBreaker(opts.CircuitThreshold, opts.CircuitCooldown),
		drainTimeout: opts.DrainTimeout,
	}, nil
}

// Run はマッチングループを開始し、ctx がキャンセルされるまで繰り返します。
func (m *Matcher) Run(ctx context.Context) {
	ticker := m.clock.NewTicker(m.interval)
	defer ticker.Stop()

	var wg sync.WaitGroup

	// tick 実行中も ctx.Done() を検知できるよう、tick 自体は goroutine で非同期に走らせる。
	// tickRunning で同時に 1 tick までに制限し、完了は tickDone (バッファ 1、Run 終了後の送信でも
	// ブロックしない) で受け取る。
	tickRunning := false
	tickDone := make(chan struct{}, 1)
	startTick := func() {
		tickRunning = true
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.tick(ctx)
			tickDone <- struct{}{}
		}()
	}

	for {
		// tick 実行中は ticker を読まない。完了直後の select で ticker と tickDone が同時に ready のとき、
		// 乱択で ticker が先に選ばれて通知を捨てるのを防ぐ。実行中に来た通知は ticker の中に残り、完了後に実行する。
		var tickerC <-chan time.Time
		if !tickRunning {
			tickerC = ticker.C()
		}
		select {
		case <-ctx.Done():
			slog.Info("matcher draining", "timeout", m.drainTimeout)
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
			select {
			case <-done:
				slog.Info("matcher drain complete")
			case <-m.clock.After(m.drainTimeout):
				slog.Warn("matcher drain timeout exceeded, forcing exit")
			}
			return
		case <-tickerC:
			startTick()
		case <-tickDone:
			tickRunning = false
		}
	}
}

// IsCircuitOpen はサーキットブレーカーが開いているかどうかを返します。
func (m *Matcher) IsCircuitOpen() bool {
	return m.circuit.isCircuitOpen()
}

func (m *Matcher) tick(ctx context.Context) {
	// circuit open 中は pop 自体を止める。publish が壊れている間も pop を続けると、
	// プレイヤーをキューから取り出した上で publish も re-enqueue も失敗する窓が生まれるため。
	isAllowed, shouldLogSkip := m.circuit.allowTick(m.clock.Now())
	if !isAllowed {
		if shouldLogSkip {
			slog.Warn("matcher circuit open, skipping tick")
		}
		return
	}

	pair, gatewayInstanceID, err := m.queue.PopPair(ctx)
	if err != nil {
		slog.Error("matcher pop pair failed", "error", err)
		m.circuit.recordPopFailure(m.clock.Now())
		return
	}
	m.circuit.recordPopSuccess()
	if len(pair) == 0 {
		return
	}
	if len(pair) != domain.MatchPlayerCount {
		slog.Error("matcher popped unexpected pair size",
			"size", len(pair), "expected", domain.MatchPlayerCount, "players", collectPlayerIDs(pair))
		return
	}

	matchID := m.matchIDs.NewMatchID()
	players := make([]domain.MatchedPlayer, 0, len(pair))
	for _, e := range pair {
		players = append(players, domain.MatchedPlayer{PlayerID: e.PlayerID, DeckID: e.DeckID, Name: e.Name, Level: e.Level})
	}
	eventType, payload, err := presenter.ToMatchMadeWire(domain.MatchMadeEvent{MatchID: matchID, Players: players})
	if err != nil {
		slog.Error("matcher build match_made failed", "error", err)
		m.circuit.recordPublishFailure(m.clock.Now())
		m.reenqueueWithRetry(ctx, matchID, pair, gatewayInstanceID)
		return
	}

	if err := m.publisher.Publish(ctx, eventType, payload); err != nil {
		slog.Error("matcher publish failed, re-enqueueing pair", "error", err)
		m.circuit.recordPublishFailure(m.clock.Now())
		m.reenqueueWithRetry(ctx, matchID, pair, gatewayInstanceID)
		return
	}
	m.circuit.recordPublishSuccess()
	slog.Info("matcher match_made", "match_id", matchID, "player1", pair[0].PlayerID, "player2", pair[1].PlayerID)
}

func (m *Matcher) reenqueueWithRetry(ctx context.Context, matchID string, pair []domain.QueueEntry, gatewayInstanceID string) {
	backoff := reenqueueInitialBackoff
	for attempt := 1; attempt <= reenqueueMaxAttempts; attempt++ {
		reenqueued, err := m.queue.Reenqueue(ctx, pair, gatewayInstanceID)
		if err == nil {
			if !reenqueued {
				logDroppedPair(matchID, pair)
			}
			return
		}
		slog.Warn("matcher re-enqueue attempt failed", "attempt", attempt, "max_attempts", reenqueueMaxAttempts, "error", err)
		if ctx.Err() != nil {
			m.bestEffortReenqueue(matchID, pair, gatewayInstanceID)
			return
		}
		if attempt == reenqueueMaxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			m.bestEffortReenqueue(matchID, pair, gatewayInstanceID)
			return
		case <-m.clock.After(backoff):
		}
		backoff *= reenqueueBackoffFactor
	}
	slog.Error("matcher lost pair after re-enqueue attempts",
		"max_attempts", reenqueueMaxAttempts, "match_id", matchID, "players", collectPlayerIDs(pair))
}

// shutdown 中に pop 済みペアを失わないための最終試行
func (m *Matcher) bestEffortReenqueue(matchID string, pair []domain.QueueEntry, gatewayInstanceID string) {
	players := collectPlayerIDs(pair)
	slog.Warn("matcher shutdown during retry, final re-enqueue attempt",
		"match_id", matchID, "players", players)
	cctx, cancel := context.WithTimeout(context.Background(), shutdownReenqueueTimeout)
	defer cancel()
	reenqueued, err := m.queue.Reenqueue(cctx, pair, gatewayInstanceID)
	if err != nil {
		slog.Error("matcher lost pair during shutdown",
			"match_id", matchID, "players", players, "error", err)
		return
	}
	if !reenqueued {
		logDroppedPair(matchID, pair)
		return
	}
	slog.Info("matcher shutdown re-enqueue ok", "match_id", matchID)
}

// logDroppedPair は pop 時点から gateway プロセスが切り替わり、re-enqueue を見送ったペアを記録する。
func logDroppedPair(matchID string, pair []domain.QueueEntry) {
	// ペアの相手は古い gateway プロセスと共に接続が失われているため、待機に戻しても救えない。
	slog.Warn("matcher dropped stale pair instead of re-enqueueing",
		"match_id", matchID, "players", collectPlayerIDs(pair))
}

func collectPlayerIDs(pair []domain.QueueEntry) []string {
	out := make([]string, 0, len(pair))
	for _, e := range pair {
		out = append(out, e.PlayerID)
	}
	return out
}
