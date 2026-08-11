package matcher_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher"
)

// circuitTestThreshold は各ケースで使う「設定した閾値」の具体値。
// 仕様上はどの正の整数でもよく、値そのものに意味は無い。
const circuitTestThreshold = 3

func TestMatcherCircuitBreaker(t *testing.T) {
	t.Run("サーキットブレーカー", func(t *testing.T) {
		t.Run("設定した閾値をNとしたとき", func(t *testing.T) {
			t.Run("送出が失敗し続ける状態で、周期処理をN-1回繰り返しても、サーキットブレーカーは閉じたままになる", func(t *testing.T) {
				q := newFakeQueueWithPairs(1)
				pub := &fakePublisher{gate: make(chan error)}
				m := matcher.New(q, pub, newOptions(matcher.Options{CircuitThreshold: circuitTestThreshold}))
				startMatcher(t, m)

				for i := 0; i < circuitTestThreshold-1; i++ {
					pub.gate <- assert.AnError
				}
				waitForCalls(t, pub, circuitTestThreshold-1)

				waitForCircuitState(t, m, false)
			})

			t.Run("送出が失敗し続ける状態で、周期処理をN回繰り返すと、サーキットブレーカーが開く", func(t *testing.T) {
				q := newFakeQueueWithPairs(1)
				pub := &fakePublisher{gate: make(chan error)}
				m := matcher.New(q, pub, newOptions(matcher.Options{CircuitThreshold: circuitTestThreshold}))
				startMatcher(t, m)

				openCircuit(t, pub, m, circuitTestThreshold)
			})

			t.Run("サーキットブレーカーが閉じている状態で、送出がN-1回失敗したあと1回成功し、そのあとさらにN-1回失敗するよう周期処理を繰り返しても、サーキットブレーカーは閉じたままになる", func(t *testing.T) {
				// 成功ラウンドでペアが 1 組消費されるため、失敗ラウンドで使い回す分と
				// 合わせて 2 組を用意する。
				q := newFakeQueueWithPairs(2)
				pub := &fakePublisher{gate: make(chan error)}
				m := matcher.New(q, pub, newOptions(matcher.Options{CircuitThreshold: circuitTestThreshold}))
				startMatcher(t, m)

				for i := 0; i < circuitTestThreshold-1; i++ {
					pub.gate <- assert.AnError
				}
				pub.gate <- nil
				for i := 0; i < circuitTestThreshold-1; i++ {
					pub.gate <- assert.AnError
				}
				waitForCalls(t, pub, (circuitTestThreshold-1)*2+1)

				waitForCircuitState(t, m, false)
			})
		})

		t.Run("サーキットブレーカーが開いている状態で、設定したクールダウン時間が経過する前にマッチメイキングキューに2人並べて周期処理を動かすと、2人は取り出されないままキューサイズは2のままになる", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			pub := &fakePublisher{gate: make(chan error)}
			m := matcher.New(q, pub, newOptions(matcher.Options{CircuitThreshold: circuitTestThreshold, CircuitCooldown: time.Hour}))
			startMatcher(t, m)

			openCircuit(t, pub, m, circuitTestThreshold)
			waitForQueueSize(t, q, 2)

			assert.Never(t, func() bool { return q.size() != 2 }, 50*time.Millisecond, time.Millisecond)
		})

		t.Run("サーキットブレーカーが開いている状態で、設定したクールダウン時間が経過したあとマッチメイキングキューに2人並べて周期処理を動かすと、2人は取り出されキューサイズが0になる", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			pub := &fakePublisher{gate: make(chan error)}
			m := matcher.New(q, pub, newOptions(matcher.Options{CircuitThreshold: circuitTestThreshold, CircuitCooldown: 20 * time.Millisecond}))
			startMatcher(t, m)

			openCircuit(t, pub, m, circuitTestThreshold)
			waitForQueueSize(t, q, 2)

			pub.gate <- nil
			waitForCalls(t, pub, circuitTestThreshold+1)

			waitForQueueSize(t, q, 0)
		})

		t.Run("サーキットブレーカーが開きクールダウンが経過したあと、送出に成功する状態で周期処理を動かすと、サーキットブレーカーは閉じる", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			pub := &fakePublisher{gate: make(chan error)}
			m := matcher.New(q, pub, newOptions(matcher.Options{CircuitThreshold: circuitTestThreshold, CircuitCooldown: 20 * time.Millisecond}))
			startMatcher(t, m)

			openCircuit(t, pub, m, circuitTestThreshold)

			pub.gate <- nil
			waitForCalls(t, pub, circuitTestThreshold+1)

			waitForCircuitState(t, m, false)
		})

		t.Run("サーキットブレーカーが開きクールダウンが経過したあと、送出に失敗する状態で周期処理を動かすと、サーキットブレーカーは開いたままになる", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			pub := &fakePublisher{gate: make(chan error)}
			m := matcher.New(q, pub, newOptions(matcher.Options{CircuitThreshold: circuitTestThreshold, CircuitCooldown: 20 * time.Millisecond}))
			startMatcher(t, m)

			openCircuit(t, pub, m, circuitTestThreshold)

			pub.gate <- assert.AnError
			waitForCalls(t, pub, circuitTestThreshold+1)

			waitForCircuitState(t, m, true)
		})
	})
}
