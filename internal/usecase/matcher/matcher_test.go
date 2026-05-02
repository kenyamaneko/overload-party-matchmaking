package matcher

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	mmpubsub "github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

// newTestEventBuilder は production と同型の EventBuilder を構築する。
func newTestEventBuilder(t *testing.T) *mmpubsub.EventBuilder {
	t.Helper()
	return mmpubsub.NewEventBuilder()
}

// publishCall は fakePublisher が記録する 1 回の Publish 呼び出し。
type publishCall struct {
	eventType string
	payload   []byte
}

// decode は payload を apimatchmaking.MatchMadeEvent にデコードする。
func (c publishCall) decode(t *testing.T) apimatchmaking.MatchMadeEvent {
	t.Helper()
	var ev apimatchmaking.MatchMadeEvent
	require.NoError(t, json.Unmarshal(c.payload, &ev))
	return ev
}

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
	publishes  []publishCall
	failN      int // fail the next N publishes, then succeed
	alwaysFail bool
}

func (f *fakePublisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.alwaysFail {
		return errors.New("publish failed (always)")
	}
	if f.failN > 0 {
		f.failN--
		return errors.New("publish failed")
	}
	f.publishes = append(f.publishes, publishCall{
		eventType: eventType,
		payload:   append([]byte(nil), payload...),
	})
	return nil
}

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

// TestTickPublishesWhenPairReady はキュー先頭に 2 名揃っているとき、tick が MatchMadeEvent を publish し
// re-enqueue も発生しないことを検証する。
func TestTickPublishesWhenPairReady(t *testing.T) {
	q := &fakeQueue{pair: samplePair()}
	p := &fakePublisher{}
	m := New(q, p, newTestEventBuilder(t), defaultOpts())

	m.tick(context.Background())

	require.Len(t, p.publishes, 1)
	require.Equal(t, apimatchmaking.EventTypeMatchMade, p.publishes[0].eventType)
	ev := p.publishes[0].decode(t)
	require.Equal(t, apimatchmaking.EventTypeMatchMade, ev.Type)
	require.Len(t, ev.Players, 2)
	require.Equal(t, "p1", ev.Players[0].PlayerID)
	require.Equal(t, int64(1), ev.Players[0].DeckID)
	require.Equal(t, "p2", ev.Players[1].PlayerID)
	require.Equal(t, int64(2), ev.Players[1].DeckID)
	require.Empty(t, q.reentry)
	require.False(t, m.CircuitOpen())
}

// TestTickProcessesMultiplePairsAcrossTicks は連続する tick で複数ペアが順次 publish され、
// 各マッチ ID が一意であることを検証する。
func TestTickProcessesMultiplePairsAcrossTicks(t *testing.T) {
	q := &fakeQueue{}
	p := &fakePublisher{}
	m := New(q, p, newTestEventBuilder(t), defaultOpts())

	q.setPair([]domain.QueueEntry{
		{PlayerID: "p1", DeckID: 1, JoinedAt: time.Now()},
		{PlayerID: "p2", DeckID: 2, JoinedAt: time.Now()},
	})
	m.tick(context.Background())

	q.setPair([]domain.QueueEntry{
		{PlayerID: "p3", DeckID: 3, JoinedAt: time.Now()},
		{PlayerID: "p4", DeckID: 4, JoinedAt: time.Now()},
	})
	m.tick(context.Background())

	require.Len(t, p.publishes, 2)
	ev0 := p.publishes[0].decode(t)
	ev1 := p.publishes[1].decode(t)
	require.Equal(t, "p1", ev0.Players[0].PlayerID)
	require.Equal(t, "p2", ev0.Players[1].PlayerID)
	require.Equal(t, "p3", ev1.Players[0].PlayerID)
	require.Equal(t, "p4", ev1.Players[1].PlayerID)
	require.NotEqual(t, ev0.MatchID, ev1.MatchID, "match IDs must be unique across ticks")
	require.Empty(t, q.reentry)
	require.False(t, m.CircuitOpen())
}

// TestTickNoopWhenQueueEmpty はキューが空のとき、tick が何も publish しないことを検証する。
func TestTickNoopWhenQueueEmpty(t *testing.T) {
	q := &fakeQueue{}
	p := &fakePublisher{}
	m := New(q, p, newTestEventBuilder(t), defaultOpts())

	m.tick(context.Background())

	require.Empty(t, p.publishes)
}

// TestTickReenqueuesOnPublishFailure は publish 失敗時に pop 済みペアがキューに戻され、
// 単発失敗では circuit が open しないことを検証する。
func TestTickReenqueuesOnPublishFailure(t *testing.T) {
	q := &fakeQueue{pair: samplePair()}
	p := &fakePublisher{failN: 1}
	m := New(q, p, newTestEventBuilder(t), defaultOpts())

	m.tick(context.Background())

	require.Empty(t, p.publishes, "publish failed, so no event should be recorded")
	require.Len(t, q.reentry, 2, "both players must be re-enqueued")
	require.Equal(t, "p1", q.reentry[0].PlayerID)
	require.Equal(t, "p2", q.reentry[1].PlayerID)
	require.False(t, m.CircuitOpen(), "single failure does not open circuit")
}

// TestTickPropagatesPopError は PopPair がエラーを返したとき、publish も re-enqueue も行わずに
// tick を終えることを検証する。
func TestTickPropagatesPopError(t *testing.T) {
	q := &fakeQueue{popErr: errors.New("boom")}
	p := &fakePublisher{}
	m := New(q, p, newTestEventBuilder(t), defaultOpts())

	m.tick(context.Background())

	require.Empty(t, p.publishes)
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

// TestTickReenqueueRetriesTransientFailures は re-enqueue が transient エラーで失敗しても、
// 指数バックオフリトライで最終的にペアがキューへ戻ることを検証する。
func TestTickReenqueueRetriesTransientFailures(t *testing.T) {
	q := &countingReenqueueQueue{
		fakeQueue:           fakeQueue{pair: samplePair()},
		reenqueueFailsUntil: 2,
	}
	p := &fakePublisher{failN: 1}
	m := New(q, p, newTestEventBuilder(t), defaultOpts())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m.tick(ctx)

	require.Equal(t, 3, q.reenqueueAttempts, "should retry until success (fails 2 + succeeds on 3)")
	require.Len(t, q.reentry, 2, "final re-enqueue must persist the pair")
}

// TestCircuitOpensAfterNConsecutiveFailures は publish が CircuitThreshold 回連続で失敗したとき、
// サーキットが open に遷移することを検証する。
func TestCircuitOpensAfterNConsecutiveFailures(t *testing.T) {
	q := &fakeQueue{}
	p := &fakePublisher{alwaysFail: true}
	opts := defaultOpts()
	opts.CircuitThreshold = 3
	m := New(q, p, newTestEventBuilder(t), opts)

	for i := 0; i < 3; i++ {
		q.setPair(samplePair())
		m.tick(context.Background())
	}

	require.True(t, m.CircuitOpen(), "circuit must open after threshold failures")
}

// TestCircuitShortCircuitsTickWhenOpen はサーキット open の間、tick が PopPair を呼ばず
// キュー内のプレイヤーが取り出されないことを検証する。
func TestCircuitShortCircuitsTickWhenOpen(t *testing.T) {
	q := &fakeQueue{}
	p := &fakePublisher{alwaysFail: true}
	opts := defaultOpts()
	opts.CircuitThreshold = 1
	opts.CircuitCooldown = time.Hour
	m := New(q, p, newTestEventBuilder(t), opts)

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

// TestCircuitClosesAfterSuccessfulTrial は cooldown 経過後の trial tick が成功した場合に
// サーキットが close し、通常 publish が再開することを検証する。
func TestCircuitClosesAfterSuccessfulTrial(t *testing.T) {
	q := &fakeQueue{}
	p := &fakePublisher{alwaysFail: true}
	opts := defaultOpts()
	opts.CircuitThreshold = 1
	opts.CircuitCooldown = 1 * time.Millisecond
	m := New(q, p, newTestEventBuilder(t), opts)

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
	require.Len(t, p.publishes, 1)
}

// TestCircuitReopensAfterFailedTrial は cooldown 経過後の trial tick も失敗した場合、
// サーキットが再 open することを検証する。
func TestCircuitReopensAfterFailedTrial(t *testing.T) {
	q := &fakeQueue{}
	p := &fakePublisher{alwaysFail: true}
	opts := defaultOpts()
	opts.CircuitThreshold = 1
	opts.CircuitCooldown = 1 * time.Millisecond
	m := New(q, p, newTestEventBuilder(t), opts)

	q.setPair(samplePair())
	m.tick(context.Background())
	require.True(t, m.CircuitOpen())

	time.Sleep(5 * time.Millisecond)

	// trial も失敗するケース
	q.setPair(samplePair())
	m.tick(context.Background())

	require.True(t, m.CircuitOpen(), "failed trial must reopen circuit")
}

// TestRunDrainsCurrentTick は ctx キャンセル発火時、Run が in-flight tick の完了を
// DrainTimeout 以内に待って正常終了することを検証する。
func TestRunDrainsCurrentTick(t *testing.T) {
	q := &blockingQueue{
		block:   make(chan struct{}),
		release: make(chan struct{}),
	}
	p := &fakePublisher{}
	opts := defaultOpts()
	opts.Interval = 10 * time.Millisecond
	opts.DrainTimeout = 500 * time.Millisecond
	m := New(q, p, newTestEventBuilder(t), opts)

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
