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

// TestTickPublishesWhenPairReady はキュー先頭に 2 名揃っているとき、tick が MatchMadeEvent を publish し
// re-enqueue も発生しないことを検証する (正常系)。
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

// TestTickProcessesMultiplePairsAcrossTicks は連続する tick で複数ペアが順次 publish され、
// circuit が closed のまま・re-enqueue も発生せず、各マッチ ID が一意であることを検証する
// (1 組成立後も matcher の内部状態が正しく次のマッチを処理できること)。
func TestTickProcessesMultiplePairsAcrossTicks(t *testing.T) {
	q := &fakeQueue{}
	p := &fakePublisher{}
	m := New(q, p, defaultOpts())

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

	require.Len(t, p.events, 2)
	require.Equal(t, "p1", p.events[0].Players[0].PlayerID)
	require.Equal(t, "p2", p.events[0].Players[1].PlayerID)
	require.Equal(t, "p3", p.events[1].Players[0].PlayerID)
	require.Equal(t, "p4", p.events[1].Players[1].PlayerID)
	require.NotEqual(t, p.events[0].MatchID, p.events[1].MatchID, "match IDs must be unique across ticks")
	require.Empty(t, q.reentry)
	require.False(t, m.CircuitOpen())
}

// TestTickNoopWhenQueueEmpty はキューが空のとき、tick が何も publish しないことを検証する。
func TestTickNoopWhenQueueEmpty(t *testing.T) {
	q := &fakeQueue{}
	p := &fakePublisher{}
	m := New(q, p, defaultOpts())

	m.tick(context.Background())

	require.Empty(t, p.events)
}

// TestTickReenqueuesOnPublishFailure は publish が失敗したとき、pop 済みペアがキューに戻されることを検証する
// (プレイヤーを暗黙 drop しない契約)。単発失敗では circuit は open しない。
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

// TestTickPropagatesPopError は PopPair がエラーを返したとき、publish も re-enqueue も行わずに
// tick を終えることを検証する (ペアを手にしていないので戻すべきものが無い)。
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

// TestTickReenqueueRetriesTransientFailures は re-enqueue 自体が transient なエラーで失敗しても、
// 指数バックオフリトライで最終的にペアがキューへ戻ることを検証する。
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

// TestCircuitOpensAfterNConsecutiveFailures は publish 失敗が CircuitThreshold 回連続で起きたとき、
// サーキットブレーカーが open 状態に遷移することを検証する。
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

// TestCircuitShortCircuitsTickWhenOpen はサーキットが open の間、tick が PopPair を呼ばず
// キュー内のプレイヤーが取り出されないことを検証する (publish 不能な状態でペアを pop すると喪失リスクが増えるため)。
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

// TestCircuitClosesAfterSuccessfulTrial はサーキット open 後、cooldown 経過後の trial tick が
// 成功した場合にサーキットが close し、通常 publish が再開することを検証する。
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

// TestCircuitReopensAfterFailedTrial は cooldown 経過後の trial tick も失敗した場合、
// サーキットが再 open して pop を再び止めることを検証する。
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

// TestRunDrainsCurrentTick は ctx キャンセル発火時、Run が in-flight tick の完了を待って
// (DrainTimeout 以内に) 正常終了することを検証する (graceful drain 契約)。
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
