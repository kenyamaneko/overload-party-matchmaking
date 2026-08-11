package matcher_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher"
)

// fakePairQueue は最初の PopPair 呼び出しでのみペアを返し、以降は空を返す。
type fakePairQueue struct {
	mu       sync.Mutex
	pair     []domain.QueueEntry
	returned bool
}

func (q *fakePairQueue) Enqueue(context.Context, string, int64, string, int64, string) (int64, error) {
	panic("not used")
}
func (q *fakePairQueue) Cancel(context.Context, string) (bool, error) { panic("not used") }
func (q *fakePairQueue) Size(context.Context) (int64, error)          { panic("not used") }

func (q *fakePairQueue) PopPair(context.Context) ([]domain.QueueEntry, string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.returned {
		return nil, "", nil
	}
	q.returned = true
	return q.pair, "gw-1", nil
}

func (q *fakePairQueue) Reenqueue(context.Context, []domain.QueueEntry, string) (bool, error) {
	return true, nil
}

// blockingPublisher は entered を close して呼び出しを通知したあと、release が close されるまで Publish をブロックする。
type blockingPublisher struct {
	entered chan struct{}
	release chan struct{}

	mu        sync.Mutex
	published []string
}

func newBlockingPublisher() *blockingPublisher {
	return &blockingPublisher{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (p *blockingPublisher) Publish(_ context.Context, eventType string, _ []byte) error {
	close(p.entered)
	<-p.release
	p.mu.Lock()
	p.published = append(p.published, eventType)
	p.mu.Unlock()
	return nil
}

func (p *blockingPublisher) publishedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.published)
}

func testPair() []domain.QueueEntry {
	return []domain.QueueEntry{
		{PlayerID: "player-1", DeckID: 1, Name: "one", Level: 1},
		{PlayerID: "player-2", DeckID: 2, Name: "two", Level: 1},
	}
}

func TestRun(t *testing.T) {
	t.Run("シャットダウン時のドレイン", func(t *testing.T) {
		t.Run("送出中の通知がある状態でコンテキストをキャンセルすると、その送出が完了してからRun(ctx)の呼び出しから制御が戻る", func(t *testing.T) {
			queue := &fakePairQueue{pair: testPair()}
			publisher := newBlockingPublisher()
			m := matcher.New(queue, publisher, matcher.Options{
				Interval:     5 * time.Millisecond,
				DrainTimeout: 2 * time.Second,
			})

			ctx, cancel := context.WithCancel(context.Background())
			runReturned := make(chan struct{})
			go func() {
				m.Run(ctx)
				close(runReturned)
			}()

			select {
			case <-publisher.entered:
			case <-time.After(time.Second):
				t.Fatal("publish was not invoked in time")
			}

			cancel()

			select {
			case <-runReturned:
				t.Fatal("Run returned before the in-flight publish completed")
			case <-time.After(50 * time.Millisecond):
			}

			close(publisher.release)

			select {
			case <-runReturned:
			case <-time.After(time.Second):
				t.Fatal("Run did not return after the in-flight publish completed")
			}
		})

		t.Run("送出中の通知がある状態でコンテキストをキャンセルすると、Run(ctx)の呼び出しから制御が戻った時点で、そのメッセージは送出済み一覧に記録されている", func(t *testing.T) {
			queue := &fakePairQueue{pair: testPair()}
			publisher := newBlockingPublisher()
			m := matcher.New(queue, publisher, matcher.Options{
				Interval:     5 * time.Millisecond,
				DrainTimeout: 2 * time.Second,
			})

			ctx, cancel := context.WithCancel(context.Background())
			runReturned := make(chan struct{})
			go func() {
				m.Run(ctx)
				close(runReturned)
			}()

			select {
			case <-publisher.entered:
			case <-time.After(time.Second):
				t.Fatal("publish was not invoked in time")
			}

			cancel()
			close(publisher.release)

			select {
			case <-runReturned:
			case <-time.After(time.Second):
				t.Fatal("Run did not return")
			}

			assert.Equal(t, 1, publisher.publishedCount())
		})

		t.Run("ドレインタイムアウトを過ぎても送出中の通知が完了しないとき、完了を待たずにRun(ctx)の呼び出しから制御が戻る", func(t *testing.T) {
			queue := &fakePairQueue{pair: testPair()}
			publisher := newBlockingPublisher() // release を close しないため、Publish は戻らない

			m := matcher.New(queue, publisher, matcher.Options{
				Interval:     5 * time.Millisecond,
				DrainTimeout: 20 * time.Millisecond,
			})

			ctx, cancel := context.WithCancel(context.Background())
			runReturned := make(chan struct{})
			go func() {
				m.Run(ctx)
				close(runReturned)
			}()

			select {
			case <-publisher.entered:
			case <-time.After(time.Second):
				t.Fatal("publish was not invoked in time")
			}

			cancel()

			select {
			case <-runReturned:
			case <-time.After(time.Second):
				t.Fatal("Run did not return after the drain timeout elapsed")
			}

			require.Equal(t, 0, publisher.publishedCount())
		})
	})
}
