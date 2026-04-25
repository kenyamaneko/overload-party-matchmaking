package matcher

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
)

// DefaultDrainTimeout はシャットダウン時のドレインタイムアウトの既定値です。
const DefaultDrainTimeout = 10 * time.Second

// Matcher はマッチメイキングループとサーキットブレーカーを管理します。
type Matcher struct {
	queue        port.Queue
	publisher    port.RawEventPublisher
	eventBuilder port.EventBuilder
	interval     time.Duration

	mu                 sync.Mutex
	failureCount       int
	circuitOpen        bool
	circuitOpenedAt    time.Time
	circuitLogThisTick bool

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
func New(queue port.Queue, publisher port.RawEventPublisher, eventBuilder port.EventBuilder, opts Options) *Matcher {
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
		eventBuilder: eventBuilder,
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
			log.Printf("matcher: draining (timeout=%s)", m.drainTimeout)
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
			select {
			case <-done:
				log.Printf("matcher: drain complete")
			case <-time.After(m.drainTimeout):
				log.Printf("matcher: drain timeout exceeded, forcing exit")
				tickCancel()
			}
			return
		case <-ticker.C:
			runTick()
		}
	}
}

// CircuitOpen はサーキットブレーカーが開いているかどうかを返します。
func (m *Matcher) CircuitOpen() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.circuitOpen
}

func (m *Matcher) allowTick(now time.Time) (allow bool, logSkip bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.circuitOpen {
		m.circuitLogThisTick = false
		return true, false
	}
	if now.Sub(m.circuitOpenedAt) >= m.cooldown {
		return true, false
	}
	if !m.circuitLogThisTick {
		m.circuitLogThisTick = true
		return false, true
	}
	return false, false
}

func (m *Matcher) recordSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureCount = 0
	if m.circuitOpen {
		log.Printf("matcher: circuit closed after successful publish")
	}
	m.circuitOpen = false
	m.circuitLogThisTick = false
}

func (m *Matcher) recordFailure(now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureCount++
	if m.failureCount >= m.threshold {
		if !m.circuitOpen {
			log.Printf("matcher: circuit OPEN (consecutive failures=%d, threshold=%d, cooldown=%s)",
				m.failureCount, m.threshold, m.cooldown)
		} else {
			log.Printf("matcher: circuit trial failed, reopening (failures=%d)", m.failureCount)
		}
		m.circuitOpen = true
		m.circuitOpenedAt = now
		m.circuitLogThisTick = false
		return true
	}
	return false
}

func (m *Matcher) tick(ctx context.Context) {
	allow, logSkip := m.allowTick(time.Now())
	if !allow {
		if logSkip {
			log.Printf("matcher: circuit open, skipping")
		}
		return
	}

	pair, err := m.queue.PopPair(ctx)
	if err != nil {
		log.Printf("matcher: pop pair: %v", err)
		return
	}
	if len(pair) != 2 {
		return
	}

	matchID := newMatchID()
	players := []port.MatchedPlayer{
		{PlayerID: pair[0].PlayerID, DeckID: pair[0].DeckID},
		{PlayerID: pair[1].PlayerID, DeckID: pair[1].DeckID},
	}
	event, err := m.eventBuilder.BuildMatchMade(matchID, players)
	if err != nil {
		log.Printf("matcher: build match_made: %v", err)
		m.recordFailure(time.Now())
		m.reenqueueWithRetry(ctx, matchID, pair)
		return
	}

	if err := m.publisher.Publish(ctx, event.Topic, event.Payload); err != nil {
		log.Printf("matcher: publish failed, re-enqueueing pair: %v", err)
		m.recordFailure(time.Now())
		m.reenqueueWithRetry(ctx, matchID, pair)
		return
	}
	m.recordSuccess()
	log.Printf("matcher: match_made matchId=%s players=%s,%s", matchID, pair[0].PlayerID, pair[1].PlayerID)
}

func (m *Matcher) reenqueueWithRetry(ctx context.Context, matchID string, pair []domain.QueueEntry) {
	const maxAttempts = 5
	backoff := 100 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := m.queue.Reenqueue(ctx, pair); err == nil {
			return
		} else {
			log.Printf("matcher: re-enqueue attempt %d/%d failed: %v", attempt, maxAttempts, err)
		}
		if ctx.Err() != nil {
			m.bestEffortReenqueue(matchID, pair)
			return
		}
		select {
		case <-ctx.Done():
			m.bestEffortReenqueue(matchID, pair)
			return
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	players := playerIDs(pair)
	log.Printf("matcher: LOST pair after %d re-enqueue attempts matchId=%s players=%v",
		maxAttempts, matchID, players)
}

// shutdown 中に pop 済みペアを失わないための最終試行
func (m *Matcher) bestEffortReenqueue(matchID string, pair []domain.QueueEntry) {
	players := playerIDs(pair)
	log.Printf("matcher: shutdown during retry, final re-enqueue attempt matchId=%s players=%v",
		matchID, players)
	cctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.queue.Reenqueue(cctx, pair); err != nil {
		log.Printf("matcher: LOST pair during shutdown matchId=%s players=%v err=%v",
			matchID, players, err)
		return
	}
	log.Printf("matcher: shutdown re-enqueue OK matchId=%s", matchID)
}

func playerIDs(pair []domain.QueueEntry) []string {
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
