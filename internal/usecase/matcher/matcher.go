package matcher

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/presenter"
)

// DefaultDrainTimeout はシャットダウン時のドレインタイムアウトの既定値です。
const DefaultDrainTimeout = 10 * time.Second

// Matcher はマッチメイキングループとサーキットブレーカーを管理します。
type Matcher struct {
	queue     port.Queue
	publisher port.RawEventPublisher
	interval  time.Duration

	mu                sync.Mutex
	failureCount      int
	isCircuitOpen     bool
	circuitOpenedAt   time.Time
	hasLoggedThisTick bool

	threshold    int
	cooldown     time.Duration
	drainTimeout time.Duration
}

// Options は Matcher の動作パラメータです。
type Options struct {
	Interval         time.Duration
	CircuitThreshold int
	CircuitCooldown  time.Duration
	DrainTimeout     time.Duration
}

// New は Matcher を生成します。
func New(queue port.Queue, publisher port.RawEventPublisher, opts Options) *Matcher {
	if opts.Interval <= 0 {
		opts.Interval = time.Second
	}
	if opts.CircuitThreshold <= 0 {
		opts.CircuitThreshold = 5
	}
	if opts.CircuitCooldown <= 0 {
		opts.CircuitCooldown = 30 * time.Second
	}
	if opts.DrainTimeout <= 0 {
		opts.DrainTimeout = DefaultDrainTimeout
	}
	return &Matcher{
		queue:        queue,
		publisher:    publisher,
		interval:     opts.Interval,
		threshold:    opts.CircuitThreshold,
		cooldown:     opts.CircuitCooldown,
		drainTimeout: opts.DrainTimeout,
	}
}

// Run はマッチングループを開始し、ctx がキャンセルされるまで繰り返します。
func (m *Matcher) Run(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	var wg sync.WaitGroup
	tickCtx, tickCancel := context.WithCancel(ctx)
	defer tickCancel()

	runTick := func() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.tick(tickCtx)
		}()
		wg.Wait()
	}

	for {
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
			case <-time.After(m.drainTimeout):
				slog.Warn("matcher drain timeout exceeded, forcing exit")
				tickCancel()
			}
			return
		case <-ticker.C:
			runTick()
		}
	}
}

// IsCircuitOpen はサーキットブレーカーが開いているかどうかを返します。
func (m *Matcher) IsCircuitOpen() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isCircuitOpen
}

func (m *Matcher) allowTick(now time.Time) (isAllowed bool, shouldLogSkip bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isCircuitOpen {
		m.hasLoggedThisTick = false
		return true, false
	}
	if now.Sub(m.circuitOpenedAt) >= m.cooldown {
		return true, false
	}
	if !m.hasLoggedThisTick {
		m.hasLoggedThisTick = true
		return false, true
	}
	return false, false
}

func (m *Matcher) recordSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureCount = 0
	if m.isCircuitOpen {
		slog.Info("matcher circuit closed after successful publish")
	}
	m.isCircuitOpen = false
	m.hasLoggedThisTick = false
}

func (m *Matcher) recordFailure(now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureCount++
	if m.failureCount >= m.threshold {
		if !m.isCircuitOpen {
			slog.Warn("matcher circuit opened",
				"consecutive_failures", m.failureCount, "threshold", m.threshold, "cooldown", m.cooldown)
		} else {
			slog.Warn("matcher circuit trial failed, reopening", "failures", m.failureCount)
		}
		m.isCircuitOpen = true
		m.circuitOpenedAt = now
		m.hasLoggedThisTick = false
		return true
	}
	return false
}

func (m *Matcher) tick(ctx context.Context) {
	isAllowed, shouldLogSkip := m.allowTick(time.Now())
	if !isAllowed {
		if shouldLogSkip {
			slog.Warn("matcher circuit open, skipping tick")
		}
		return
	}

	pair, gatewayInstanceID, err := m.queue.PopPair(ctx)
	if err != nil {
		slog.Error("matcher pop pair failed", "error", err)
		return
	}
	if len(pair) != 2 {
		return
	}

	matchID := newMatchID()
	event := domain.MatchMadeEvent{
		MatchID: matchID,
		Players: []domain.MatchedPlayer{
			{PlayerID: pair[0].PlayerID, DeckID: pair[0].DeckID, Name: pair[0].Name, Level: pair[0].Level},
			{PlayerID: pair[1].PlayerID, DeckID: pair[1].DeckID, Name: pair[1].Name, Level: pair[1].Level},
		},
	}
	eventType, payload, err := presenter.ToMatchMadeWire(event)
	if err != nil {
		slog.Error("matcher build match_made failed", "error", err)
		m.recordFailure(time.Now())
		m.reenqueueWithRetry(ctx, matchID, pair, gatewayInstanceID)
		return
	}

	if err := m.publisher.Publish(ctx, eventType, payload); err != nil {
		slog.Error("matcher publish failed, re-enqueueing pair", "error", err)
		m.recordFailure(time.Now())
		m.reenqueueWithRetry(ctx, matchID, pair, gatewayInstanceID)
		return
	}
	m.recordSuccess()
	slog.Info("matcher match_made", "match_id", matchID, "player1", pair[0].PlayerID, "player2", pair[1].PlayerID)
}

func (m *Matcher) reenqueueWithRetry(ctx context.Context, matchID string, pair []domain.QueueEntry, gatewayInstanceID string) {
	const maxAttempts = 5
	backoff := 100 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		reenqueued, err := m.queue.Reenqueue(ctx, pair, gatewayInstanceID)
		if err == nil {
			if !reenqueued {
				logDroppedPair(matchID, pair)
			}
			return
		}
		slog.Warn("matcher re-enqueue attempt failed", "attempt", attempt, "max_attempts", maxAttempts, "error", err)
		if ctx.Err() != nil {
			m.bestEffortReenqueue(matchID, pair, gatewayInstanceID)
			return
		}
		select {
		case <-ctx.Done():
			m.bestEffortReenqueue(matchID, pair, gatewayInstanceID)
			return
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	players := collectPlayerIDs(pair)
	slog.Error("matcher lost pair after re-enqueue attempts",
		"max_attempts", maxAttempts, "match_id", matchID, "players", players)
}

// shutdown 中に pop 済みペアを失わないための最終試行
func (m *Matcher) bestEffortReenqueue(matchID string, pair []domain.QueueEntry, gatewayInstanceID string) {
	players := collectPlayerIDs(pair)
	slog.Warn("matcher shutdown during retry, final re-enqueue attempt",
		"match_id", matchID, "players", players)
	cctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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

func newMatchID() string {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)
	return fmt.Sprintf("mch_%s", id.String())
}

// ErrCircuitOpen はサーキットブレーカーが開いている場合に返されるエラーです。
var ErrCircuitOpen = errors.New("matchmaking publish circuit open")
