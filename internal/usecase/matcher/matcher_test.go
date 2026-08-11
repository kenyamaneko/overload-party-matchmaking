package matcher

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

type fakeQueue struct {
	pair []domain.QueueEntry
}

func (q *fakeQueue) Enqueue(ctx context.Context, playerID string, deckID int64, name string, level int64, gatewayInstanceID string) (int64, error) {
	return 0, nil
}

func (q *fakeQueue) Cancel(ctx context.Context, playerID string) (bool, error) {
	return false, nil
}

func (q *fakeQueue) Size(ctx context.Context) (int64, error) {
	return 0, nil
}

func (q *fakeQueue) PopPair(ctx context.Context) ([]domain.QueueEntry, string, error) {
	return q.pair, "gateway-test", nil
}

func (q *fakeQueue) Reenqueue(ctx context.Context, entries []domain.QueueEntry, gatewayInstanceID string) (bool, error) {
	return true, nil
}

func newTestPair() []domain.QueueEntry {
	return []domain.QueueEntry{
		{PlayerID: "player-1", DeckID: 1, Name: "Player One", Level: 10},
		{PlayerID: "player-2", DeckID: 2, Name: "Player Two", Level: 20},
	}
}

// fakePublisher は Publish 呼び出しを任意のタイミングまでブロックし続けられるフェイク実装。
type fakePublisher struct {
	mu   sync.Mutex
	sent [][]byte

	started     chan struct{}
	startedOnce sync.Once

	release     chan struct{}
	releaseOnce sync.Once
}

func newFakePublisher() *fakePublisher {
	return &fakePublisher{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (p *fakePublisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	p.startedOnce.Do(func() { close(p.started) })
	<-p.release
	p.mu.Lock()
	p.sent = append(p.sent, payload)
	p.mu.Unlock()
	return nil
}

func (p *fakePublisher) unblock() {
	p.releaseOnce.Do(func() { close(p.release) })
}

func (p *fakePublisher) sentPayloads() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([][]byte, len(p.sent))
	copy(out, p.sent)
	return out
}

func waitForPublishStart(t *testing.T, publisher *fakePublisher) {
	t.Helper()
	select {
	case <-publisher.started:
	case <-time.After(time.Second):
		t.Fatal("publish が呼び出されなかった")
	}
}

func TestRun(t *testing.T) {
	t.Run("シャットダウン時のドレイン", func(t *testing.T) {
		t.Run("送出中の通知がある状態でコンテキストをキャンセルすると、その送出が完了してから処理が呼び出し元に戻る", func(t *testing.T) {
			queue := &fakeQueue{pair: newTestPair()}
			publisher := newFakePublisher()
			t.Cleanup(publisher.unblock)

			m := New(queue, publisher, Options{
				Interval:     5 * time.Millisecond,
				DrainTimeout: 2 * time.Second,
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			runDone := make(chan struct{})
			go func() {
				m.Run(ctx)
				close(runDone)
			}()

			waitForPublishStart(t, publisher)
			cancel()

			select {
			case <-runDone:
				t.Fatal("送出が完了する前に処理が呼び出し元に戻った")
			case <-time.After(100 * time.Millisecond):
			}

			publisher.unblock()

			select {
			case <-runDone:
			case <-time.After(time.Second):
				t.Fatal("送出完了後も処理が呼び出し元に戻らなかった")
			}
		})

		t.Run("送出中の通知がある状態でコンテキストをキャンセルすると、処理が呼び出し元に戻った時点でそのメッセージは送出済み一覧に記録されている", func(t *testing.T) {
			queue := &fakeQueue{pair: newTestPair()}
			publisher := newFakePublisher()
			t.Cleanup(publisher.unblock)

			m := New(queue, publisher, Options{
				Interval:     5 * time.Millisecond,
				DrainTimeout: 2 * time.Second,
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			runDone := make(chan struct{})
			go func() {
				m.Run(ctx)
				close(runDone)
			}()

			waitForPublishStart(t, publisher)
			cancel()

			select {
			case <-runDone:
				t.Fatal("送出が完了する前に処理が呼び出し元に戻った")
			case <-time.After(100 * time.Millisecond):
			}

			publisher.unblock()

			select {
			case <-runDone:
			case <-time.After(time.Second):
				t.Fatal("処理が呼び出し元に戻らなかった")
			}

			sent := publisher.sentPayloads()
			require.Len(t, sent, 1)
			var decoded apimatchmaking.MatchMadeEvent
			require.NoError(t, json.Unmarshal(sent[0], &decoded))
			require.Len(t, decoded.Players, 2)
			require.Equal(t, apimatchmaking.MatchedPlayer{PlayerID: "player-1", DeckID: 1, Name: "Player One", Level: 10}, decoded.Players[0])
			require.Equal(t, apimatchmaking.MatchedPlayer{PlayerID: "player-2", DeckID: 2, Name: "Player Two", Level: 20}, decoded.Players[1])
		})

		t.Run("ドレインタイムアウトを過ぎても送出中の通知が完了しないとき、完了を待たずに処理が呼び出し元に戻る", func(t *testing.T) {
			queue := &fakeQueue{pair: newTestPair()}
			publisher := newFakePublisher()
			t.Cleanup(publisher.unblock)

			const drainTimeout = 50 * time.Millisecond
			m := New(queue, publisher, Options{
				Interval:     5 * time.Millisecond,
				DrainTimeout: drainTimeout,
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			runDone := make(chan struct{})
			go func() {
				m.Run(ctx)
				close(runDone)
			}()

			waitForPublishStart(t, publisher)
			cancel()

			select {
			case <-runDone:
			case <-time.After(drainTimeout + time.Second):
				t.Fatal("ドレインタイムアウトを過ぎても処理が呼び出し元に戻らなかった")
			}
		})
	})
}
