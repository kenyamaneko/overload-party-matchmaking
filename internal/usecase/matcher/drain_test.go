package matcher_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher"
)

func TestMatcherDrain(t *testing.T) {
	t.Run("シャットダウン時のドレイン", func(t *testing.T) {
		t.Run("送出中の通知がある状態でコンテキストをキャンセルすると、その送出が完了してからシャットダウン処理が完了する", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			pub := &fakePublisher{gate: make(chan error), started: make(chan struct{})}
			m := matcher.New(q, pub, newOptions(matcher.Options{DrainTimeout: 5 * time.Second}))

			ctx, cancel := context.WithCancel(context.Background())
			runReturned := make(chan struct{})
			go func() {
				defer close(runReturned)
				m.Run(ctx)
			}()

			<-pub.started
			cancel()

			select {
			case <-runReturned:
				t.Fatal("Run returned before the in-flight publish completed")
			case <-time.After(100 * time.Millisecond):
			}

			pub.gate <- nil
			select {
			case <-runReturned:
			case <-time.After(time.Second):
				t.Fatal("Run did not return after the in-flight publish completed")
			}
		})

		t.Run("送出中の通知がある状態でコンテキストをキャンセルすると、シャットダウン処理が完了した時点で、そのメッセージは送出済み一覧に記録されている", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			pub := &fakePublisher{gate: make(chan error), started: make(chan struct{})}
			m := matcher.New(q, pub, newOptions(matcher.Options{DrainTimeout: 5 * time.Second}))

			ctx, cancel := context.WithCancel(context.Background())
			runReturned := make(chan struct{})
			go func() {
				defer close(runReturned)
				m.Run(ctx)
			}()

			<-pub.started
			cancel()
			pub.gate <- nil
			<-runReturned

			assert.Len(t, pub.publishedMessages(), 1)
		})

		t.Run("ドレインタイムアウトを過ぎても送出中の通知が完了しないとき、完了を待たずにシャットダウン処理が完了する", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			pub := &fakePublisher{gate: make(chan error), started: make(chan struct{})}
			m := matcher.New(q, pub, newOptions(matcher.Options{DrainTimeout: 20 * time.Millisecond}))

			ctx, cancel := context.WithCancel(context.Background())
			runReturned := make(chan struct{})
			go func() {
				defer close(runReturned)
				m.Run(ctx)
			}()
			t.Cleanup(func() {
				// ドレインタイムアウト超過後も in-flight の Publish はブロックされた
				// ままなので、テスト終了時に解放してゴルーチンをリークさせない。
				select {
				case pub.gate <- nil:
				default:
				}
			})

			<-pub.started
			cancel()

			select {
			case <-runReturned:
			case <-time.After(2 * time.Second):
				t.Fatal("Run did not return after the drain timeout elapsed")
			}
		})
	})
}
