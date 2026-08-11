package matcher_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher"
)

func TestMatcherReenqueue(t *testing.T) {
	t.Run("送出失敗時のマッチメイキングキューへの書き戻し", func(t *testing.T) {
		t.Run("gatewayインスタンス識別子が現在の保持値と不一致な状態で書き戻すと、取り出した2人はマッチメイキングキューに戻らない", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			q.reenqueueRejectInstanceID = true
			pub := &fakePublisher{err: assert.AnError}
			m := matcher.New(q, pub, newOptions(matcher.Options{}))
			startMatcher(t, m)

			waitForQueueSize(t, q, 0)
			assert.Never(t, func() bool { return q.size() != 0 }, 50*time.Millisecond, time.Millisecond)
		})

		t.Run("書き戻しが4回連続で失敗したあと5回目の再試行で成功すると、そのペアはマッチメイキングキューに戻る", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			q.reenqueueFailRemaining = 4
			q.reenqueueFailErr = assert.AnError
			pub := &fakePublisher{err: assert.AnError}
			m := matcher.New(q, pub, newOptions(matcher.Options{}))
			startMatcher(t, m)

			// size は開始時点で既に 2 のため、0 への遷移を経由せずに待つと
			// pop 前の状態を誤って検出しうる。
			require.Eventually(t, func() bool { return q.size() == 0 }, time.Second, time.Millisecond)
			require.Eventually(t, func() bool { return q.size() == 2 }, 5*time.Second, 10*time.Millisecond)
		})

		t.Run("書き戻しが5回とも失敗すると、そのペアはマッチメイキングキューに戻らないままになる", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			q.reenqueueFailRemaining = 5
			q.reenqueueFailErr = assert.AnError
			pub := &fakePublisher{err: assert.AnError}
			m := matcher.New(q, pub, newOptions(matcher.Options{}))
			startMatcher(t, m)

			require.Eventually(t, func() bool { return q.size() == 0 }, time.Second, time.Millisecond)
			// 再試行は指数バックオフの実時間を伴うため、その総時間より長く監視する。
			assert.Never(t, func() bool { return q.size() != 0 }, 4*time.Second, 10*time.Millisecond)
		})

		t.Run("再試行中にシャットダウン(コンテキストのキャンセル)が始まり、その後の最後の書き戻しが成功すると、そのペアはマッチメイキングキューに戻る", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			q.reenqueueFailRemaining = 1
			q.reenqueueFailErr = assert.AnError
			pub := &fakePublisher{gate: make(chan error)}
			m := matcher.New(q, pub, newOptions(matcher.Options{}))
			cancel := startMatcher(t, m)

			pub.gate <- assert.AnError
			waitForCalls(t, pub, 1)
			// 1 回目の書き戻し失敗は上記の Reenqueue 呼び出し内で同期的に起こるため、
			// この時点でバックオフの待機に入っている。
			cancel()

			require.Eventually(t, func() bool { return q.size() == 2 }, time.Second, time.Millisecond)
		})

		t.Run("再試行中にシャットダウンが始まり、その後の最後の書き戻しが失敗すると、そのペアはマッチメイキングキューに戻らないままになる", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			q.reenqueueFailRemaining = 2
			q.reenqueueFailErr = assert.AnError
			pub := &fakePublisher{gate: make(chan error)}
			m := matcher.New(q, pub, newOptions(matcher.Options{}))
			cancel := startMatcher(t, m)

			pub.gate <- assert.AnError
			waitForCalls(t, pub, 1)
			cancel()

			require.Eventually(t, func() bool { return q.remainingReenqueueFailures() == 0 }, time.Second, time.Millisecond)
			assert.Equal(t, 0, q.size())
		})
	})
}
