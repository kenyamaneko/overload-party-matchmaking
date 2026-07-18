package matcher

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

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

func (f *fakeQueue) Enqueue(ctx context.Context, playerID string, deckID int64, name string, level int64) error {
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
		{PlayerID: "p1", DeckID: 1, Name: "alice", Level: 7, JoinedAt: time.Now()},
		{PlayerID: "p2", DeckID: 2, Name: "bob", Level: 12, JoinedAt: time.Now()},
	}
}

func TestTick(t *testing.T) {
	t.Run("マッチメイキング tick", func(t *testing.T) {
		t.Run("キュー先頭に 2 名揃うとき、MatchMadeEvent を publish し re-enqueue しない", func(t *testing.T) {
			q := &fakeQueue{pair: samplePair()}
			p := &fakePublisher{}
			m := New(q, p, defaultOpts())

			m.tick(context.Background())

			require.Len(t, p.publishes, 1)
			require.Equal(t, apimatchmaking.EventTypeMatchMade, p.publishes[0].eventType)
			ev := p.publishes[0].decode(t)
			require.Equal(t, apimatchmaking.EventTypeMatchMade, ev.EventType)
			require.Len(t, ev.Players, 2)
			require.Equal(t, "p1", ev.Players[0].PlayerID)
			require.Equal(t, int64(1), ev.Players[0].DeckID)
			require.Equal(t, "alice", ev.Players[0].Name)
			require.Equal(t, int64(7), ev.Players[0].Level)
			require.Equal(t, "p2", ev.Players[1].PlayerID)
			require.Equal(t, int64(2), ev.Players[1].DeckID)
			require.Equal(t, "bob", ev.Players[1].Name)
			require.Equal(t, int64(12), ev.Players[1].Level)
			require.True(t, strings.HasPrefix(ev.MatchID, "mch_"))
			require.Empty(t, q.reentry)
			require.False(t, m.IsCircuitOpen())
		})

		t.Run("連続する tick で複数ペアを処理するとき、各マッチ ID が一意になる", func(t *testing.T) {
			q := &fakeQueue{}
			p := &fakePublisher{}
			m := New(q, p, defaultOpts())

			q.setPair([]domain.QueueEntry{
				{PlayerID: "p1", DeckID: 1, Name: "alice", Level: 1, JoinedAt: time.Now()},
				{PlayerID: "p2", DeckID: 2, Name: "bob", Level: 2, JoinedAt: time.Now()},
			})
			m.tick(context.Background())

			q.setPair([]domain.QueueEntry{
				{PlayerID: "p3", DeckID: 3, Name: "carol", Level: 3, JoinedAt: time.Now()},
				{PlayerID: "p4", DeckID: 4, Name: "dave", Level: 4, JoinedAt: time.Now()},
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
			require.False(t, m.IsCircuitOpen())
		})

		t.Run("キューが空のとき、何も publish しない", func(t *testing.T) {
			q := &fakeQueue{}
			p := &fakePublisher{}
			m := New(q, p, defaultOpts())

			m.tick(context.Background())

			require.Empty(t, p.publishes)
		})

		t.Run("publish が失敗するとき、pop したペアを re-enqueue し単発失敗では circuit を open しない", func(t *testing.T) {
			q := &fakeQueue{pair: samplePair()}
			p := &fakePublisher{failN: 1}
			m := New(q, p, defaultOpts())

			m.tick(context.Background())

			require.Empty(t, p.publishes, "publish failed, so no event should be recorded")
			require.Len(t, q.reentry, 2, "both players must be re-enqueued")
			require.Equal(t, "p1", q.reentry[0].PlayerID)
			require.Equal(t, "p2", q.reentry[1].PlayerID)
			require.False(t, m.IsCircuitOpen(), "single failure does not open circuit")
		})

		t.Run("PopPair がエラーを返すとき、publish も re-enqueue もしない", func(t *testing.T) {
			q := &fakeQueue{popErr: errors.New("boom")}
			p := &fakePublisher{}
			m := New(q, p, defaultOpts())

			m.tick(context.Background())

			require.Empty(t, p.publishes)
			require.Empty(t, q.reentry)
		})

		t.Run("re-enqueue が transient エラーで失敗するとき、指数バックオフリトライで最終的にペアが戻る", func(t *testing.T) {
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
		})

		t.Run("配信に失敗したペアの戻しが5回すべて失敗するとき、ペアはキューに戻らない", func(t *testing.T) {
			q := &countingReenqueueQueue{
				fakeQueue:           fakeQueue{pair: samplePair()},
				reenqueueFailsUntil: 5,
			}
			p := &fakePublisher{failN: 1}
			m := New(q, p, defaultOpts())

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			m.tick(ctx)

			require.Equal(t, 5, q.reenqueueAttempts, "リトライ上限 5 回で打ち切られる")
			require.Empty(t, q.reentry, "全ての戻し試行が失敗したペアはキューに残らない")
		})

		t.Run("シャットダウンでキャンセルされた後でも、配信に失敗したペアはキューに戻る", func(t *testing.T) {
			q := &cancelAwareReenqueueQueue{fakeQueue: fakeQueue{pair: samplePair()}}
			p := &fakePublisher{failN: 1}
			m := New(q, p, defaultOpts())

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			m.tick(ctx)

			require.Len(t, q.reentry, 2, "キャンセル済み ctx 経由の戻しが失敗しても、別 ctx での最終試行でペアが戻る")
		})
	})
}

func TestCircuitBreaker(t *testing.T) {
	t.Run("サーキットブレーカー", func(t *testing.T) {
		t.Run("publish が CircuitThreshold 回連続で失敗するとき、circuit が open になる", func(t *testing.T) {
			q := &fakeQueue{}
			p := &fakePublisher{alwaysFail: true}
			opts := defaultOpts()
			opts.CircuitThreshold = 3
			m := New(q, p, opts)

			for i := 0; i < 3; i++ {
				q.setPair(samplePair())
				m.tick(context.Background())
			}

			require.True(t, m.IsCircuitOpen(), "circuit must open after threshold failures")
		})

		t.Run("circuit が open の間、tick は PopPair を呼ばずキュー内のプレイヤーが残る", func(t *testing.T) {
			q := &fakeQueue{}
			p := &fakePublisher{alwaysFail: true}
			opts := defaultOpts()
			opts.CircuitThreshold = 1
			opts.CircuitCooldown = time.Hour
			m := New(q, p, opts)

			q.setPair(samplePair())
			m.tick(context.Background()) // opens circuit
			require.True(t, m.IsCircuitOpen())

			// キューに新しいペアがあるが、サーキットが開いているため pop されない
			q.setPair(samplePair())
			m.tick(context.Background())

			q.mu.Lock()
			remainingPair := q.pair
			q.mu.Unlock()
			require.Len(t, remainingPair, 2, "circuit open must prevent PopPair")
		})

		t.Run("cooldown 経過後の trial tick が成功するとき、circuit が close し publish が再開する", func(t *testing.T) {
			q := &fakeQueue{}
			p := &fakePublisher{alwaysFail: true}
			opts := defaultOpts()
			opts.CircuitThreshold = 1
			opts.CircuitCooldown = 1 * time.Millisecond
			m := New(q, p, opts)

			q.setPair(samplePair())
			m.tick(context.Background()) // fail and open
			require.True(t, m.IsCircuitOpen())

			time.Sleep(5 * time.Millisecond) // cooldown elapses
			p.mu.Lock()
			p.alwaysFail = false
			p.mu.Unlock()

			q.setPair(samplePair())
			m.tick(context.Background()) // trial succeeds

			require.False(t, m.IsCircuitOpen(), "successful trial must close circuit")
			require.Len(t, p.publishes, 1)
		})

		t.Run("cooldown 経過後の trial tick も失敗するとき、circuit が再び open する", func(t *testing.T) {
			q := &fakeQueue{}
			p := &fakePublisher{alwaysFail: true}
			opts := defaultOpts()
			opts.CircuitThreshold = 1
			opts.CircuitCooldown = 1 * time.Millisecond
			m := New(q, p, opts)

			q.setPair(samplePair())
			m.tick(context.Background())
			require.True(t, m.IsCircuitOpen())

			time.Sleep(5 * time.Millisecond)

			// trial も失敗するケース
			q.setPair(samplePair())
			m.tick(context.Background())

			require.True(t, m.IsCircuitOpen(), "failed trial must reopen circuit")
		})

		t.Run("閾値未満の連続失敗の後に配信が成功すると、失敗の数え直しになり、あらためて閾値回連続で失敗するまでサーキットは開かない", func(t *testing.T) {
			q := &fakeQueue{}
			p := &fakePublisher{}
			opts := defaultOpts()
			opts.CircuitThreshold = 3
			m := New(q, p, opts)

			p.mu.Lock()
			p.alwaysFail = true
			p.mu.Unlock()
			q.setPair(samplePair())
			m.tick(context.Background())
			q.setPair(samplePair())
			m.tick(context.Background())

			p.mu.Lock()
			p.alwaysFail = false
			p.mu.Unlock()
			q.setPair(samplePair())
			m.tick(context.Background())

			p.mu.Lock()
			p.alwaysFail = true
			p.mu.Unlock()
			q.setPair(samplePair())
			m.tick(context.Background())
			q.setPair(samplePair())
			m.tick(context.Background())
			require.False(t, m.IsCircuitOpen(), "成功による数え直し後の2回連続失敗ではまだ開かない")

			q.setPair(samplePair())
			m.tick(context.Background())
			require.True(t, m.IsCircuitOpen(), "数え直し後にあらためて閾値回連続で失敗すると開く")
		})

		t.Run("サーキットが開いてからクールダウンちょうど経過した時点のとき、マッチングの再試行が許可される", func(t *testing.T) {
			opts := defaultOpts()
			opts.CircuitThreshold = 1
			m := New(&fakeQueue{}, &fakePublisher{}, opts)
			t0 := time.Now()
			m.recordFailure(t0)

			isAllowed, _ := m.allowTick(t0.Add(m.cooldown))

			require.True(t, isAllowed)
		})

		t.Run("クールダウン経過の直前のとき、再試行は許可されない", func(t *testing.T) {
			opts := defaultOpts()
			opts.CircuitThreshold = 1
			m := New(&fakeQueue{}, &fakePublisher{}, opts)
			t0 := time.Now()
			m.recordFailure(t0)

			isAllowed, _ := m.allowTick(t0.Add(m.cooldown - time.Nanosecond))

			require.False(t, isAllowed)
		})
	})
}

func TestRun(t *testing.T) {
	t.Run("グレースフルドレイン", func(t *testing.T) {
		t.Run("ctx キャンセル時、in-flight tick の完了を DrainTimeout 以内に待って正常終了する", func(t *testing.T) {
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
		})
	})
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

// cancelAwareReenqueueQueue は実 Redis client の挙動を模し、キャンセル済み ctx での
// Reenqueue を ctx.Err() で失敗させる。生きた ctx (シャットダウン中の最終試行) は成功させる。
type cancelAwareReenqueueQueue struct {
	fakeQueue
}

func (f *cancelAwareReenqueueQueue) Reenqueue(ctx context.Context, entries []domain.QueueEntry) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return f.fakeQueue.Reenqueue(ctx, entries)
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
