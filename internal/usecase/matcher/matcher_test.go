package matcher

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
)

type fakeQueue struct {
	mu      sync.Mutex
	pair    []domain.QueueEntry
	popErr  error
	reentry []domain.QueueEntry
	reErr   error
}

func (f *fakeQueue) Enqueue(ctx context.Context, playerID string, deckID int64) error {
	return nil
}
func (f *fakeQueue) Cancel(ctx context.Context, playerID string) (bool, error) { return false, nil }
func (f *fakeQueue) Size(ctx context.Context) (int64, error)                   { return 0, nil }
func (f *fakeQueue) PopPair(ctx context.Context) ([]domain.QueueEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.popErr != nil {
		return nil, f.popErr
	}
	out := f.pair
	f.pair = nil
	return out, nil
}
func (f *fakeQueue) Reenqueue(ctx context.Context, entries []domain.QueueEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reentry = append(f.reentry, entries...)
	return f.reErr
}

func (f *fakeQueue) setPair(pair []domain.QueueEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pair = pair
}

type fakePublisher struct {
	mu         sync.Mutex
	events     []domain.MatchMadeEvent
	failN      int // fail the next N publishes, then succeed
	alwaysFail bool
	closed     bool
}

func (f *fakePublisher) PublishMatchMade(ctx context.Context, event domain.MatchMadeEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.alwaysFail {
		return errors.New("publish failed (always)")
	}
	if f.failN > 0 {
		f.failN--
		return errors.New("publish failed")
	}
	f.events = append(f.events, event)
	return nil
}
func (f *fakePublisher) Close() error { f.closed = true; return nil }

func defaultOpts() Options {
	return Options{
		Interval:         time.Second,
		CircuitThreshold: 5,
		CircuitCooldown:  30 * time.Second,
		DrainTimeout:     100 * time.Millisecond,
	}
}

func samplePair() []domain.QueueEntry {
	return []domain.QueueEntry{
		{PlayerID: "p1", DeckID: 1, JoinedAt: time.Now()},
		{PlayerID: "p2", DeckID: 2, JoinedAt: time.Now()},
	}
}

func TestTickPublishesWhenPairReady(t *testing.T) {
	q := &fakeQueue{pair: samplePair()}
	p := &fakePublisher{}
	m := New(q, p, defaultOpts())

	m.tick(context.Background())

	require.Len(t, p.events, 1)
	require.Equal(t, "match_made", p.events[0].Type)
	require.Len(t, p.events[0].Players, 2)
	require.Equal(t, "p1", p.events[0].Players[0].PlayerID)
	require.Equal(t, int64(1), p.events[0].Players[0].DeckID)
	require.Equal(t, "p2", p.events[0].Players[1].PlayerID)
	require.Equal(t, int64(2), p.events[0].Players[1].DeckID)
	require.Empty(t, q.reentry)
	require.False(t, m.CircuitOpen())
}

func TestTickNoopWhenQueueEmpty(t *testing.T) {
	q := &fakeQueue{}
	p := &fakePublisher{}
	m := New(q, p, defaultOpts())

	m.tick(context.Background())

	require.Empty(t, p.events)
}

func TestTickReenqueuesOnPublishFailure(t *testing.T) {
	q := &fakeQueue{pair: samplePair()}
	p := &fakePublisher{failN: 1}
	m := New(q, p, defaultOpts())

	m.tick(context.Background())

	require.Empty(t, p.events, "publish failed, so no event should be recorded")
	require.Len(t, q.reentry, 2, "both players must be re-enqueued")
	require.Equal(t, "p1", q.reentry[0].PlayerID)
	require.Equal(t, "p2", q.reentry[1].PlayerID)
	require.False(t, m.CircuitOpen(), "single failure does not open circuit")
}

func TestTickPropagatesPopError(t *testing.T) {
	q := &fakeQueue{popErr: errors.New("boom")}
	p := &fakePublisher{}
	m := New(q, p, defaultOpts())

	m.tick(context.Background())

	require.Empty(t, p.events)
	require.Empty(t, q.reentry)
}

type countingReenqueueQueue struct {
	fakeQueue
	reenqueueFailsUntil int
	reenqueueAttempts   int
}

func (f *countingReenqueueQueue) Reenqueue(ctx context.Context, entries []domain.QueueEntry) error {
	f.reenqueueAttempts++
	if f.reenqueueAttempts <= f.reenqueueFailsUntil {
		return errors.New("transient redis error")
	}
	return f.fakeQueue.Reenqueue(ctx, entries)
}

func TestTickReenqueueRetriesTransientFailures(t *testing.T) {
	q := &countingReenqueueQueue{
		fakeQueue:           fakeQueue{pair: samplePair()},
		reenqueueFailsUntil: 2,
	}
	p := &fakePublisher{failN: 1}
	m := New(q, p, defaultOpts())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m.tick(ctx)

	require.Equal(t, 3, q.reenqueueAttempts, "should retry until success (fails 2 + succeeds on 3)")
	require.Len(t, q.reentry, 2, "final re-enqueue must persist the pair")
}

func TestCircuitOpensAfterNConsecutiveFailures(t *testing.T) {
	q := &fakeQueue{}
	p := &fakePublisher{alwaysFail: true}
	opts := defaultOpts()
	opts.CircuitThreshold = 3
	m := New(q, p, opts)

	for i := 0; i < 3; i++ {
		q.setPair(samplePair())
		m.tick(context.Background())
	}

	require.True(t, m.CircuitOpen(), "circuit must open after threshold failures")
}

func TestCircuitShortCircuitsTickWhenOpen(t *testing.T) {
	q := &fakeQueue{}
	p := &fakePublisher{alwaysFail: true}
	opts := defaultOpts()
	opts.CircuitThreshold = 1
	opts.CircuitCooldown = time.Hour
	m := New(q, p, opts)

	q.setPair(samplePair())
	m.tick(context.Background()) // opens circuit
	require.True(t, m.CircuitOpen())

	// キューに新しいペアがあるが、サーキットが開いているため pop されない
	q.setPair(samplePair())
	m.tick(context.Background())

	q.mu.Lock()
	remainingPair := q.pair
	q.mu.Unlock()
	require.Len(t, remainingPair, 2, "circuit open must prevent PopPair")
}

func TestCircuitClosesAfterSuccessfulTrial(t *testing.T) {
	q := &fakeQueue{}
	p := &fakePublisher{alwaysFail: true}
	opts := defaultOpts()
	opts.CircuitThreshold = 1
	opts.CircuitCooldown = 1 * time.Millisecond
	m := New(q, p, opts)

	q.setPair(samplePair())
	m.tick(context.Background()) // fail and open
	require.True(t, m.CircuitOpen())

	time.Sleep(5 * time.Millisecond) // cooldown elapses
	p.mu.Lock()
	p.alwaysFail = false
	p.mu.Unlock()

	q.setPair(samplePair())
	m.tick(context.Background()) // trial succeeds

	require.False(t, m.CircuitOpen(), "successful trial must close circuit")
	require.Len(t, p.events, 1)
}

func TestCircuitReopensAfterFailedTrial(t *testing.T) {
	q := &fakeQueue{}
	p := &fakePublisher{alwaysFail: true}
	opts := defaultOpts()
	opts.CircuitThreshold = 1
	opts.CircuitCooldown = 1 * time.Millisecond
	m := New(q, p, opts)

	q.setPair(samplePair())
	m.tick(context.Background())
	require.True(t, m.CircuitOpen())

	time.Sleep(5 * time.Millisecond)

	// trial も失敗するケース
	q.setPair(samplePair())
	m.tick(context.Background())

	require.True(t, m.CircuitOpen(), "failed trial must reopen circuit")
}

func TestRunDrainsCurrentTick(t *testing.T) {
	q := &blockingQueue{
		block:   make(chan struct{}),
		release: make(chan struct{}),
	}
	p := &fakePublisher{}
	opts := defaultOpts()
	opts.Interval = 10 * time.Millisecond
	opts.DrainTimeout = 500 * time.Millisecond
	m := New(q, p, opts)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()

	// tick が PopPair でブロックされるのを待つ
	<-q.block

	// シャットダウンを発火
	cancel()

	// tick を解放してドレインを正常完了させる
	close(q.release)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after drain")
	}
}

type blockingQueue struct {
	fakeQueue
	block   chan struct{}
	release chan struct{}
}

func (b *blockingQueue) PopPair(ctx context.Context) ([]domain.QueueEntry, error) {
	select {
	case b.block <- struct{}{}:
	default:
	}
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return nil, nil
}
