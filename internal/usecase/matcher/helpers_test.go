package matcher_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher"
)

// testInterval は Run のティック間隔として使う短い周期。実時間ベースの
// テストを高速化しつつ、tickRunning によるゲートで tick は 1 件ずつ順に
// しか進まないため、非決定性は生まれない。
const testInterval = 2 * time.Millisecond

// startMatcher は m.Run をバックグラウンドで開始し、cleanup で ctx をキャンセルして
// Run の復帰を待つ。復帰を待たずにテストが終わると次のケースに tick が漏れうるため。
func startMatcher(t *testing.T, m *matcher.Matcher) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("matcher: Run did not return after ctx cancel")
		}
	})
}

// waitForCalls は pub への Publish 呼び出し回数がちょうど n に達し、かつ
// tick 側の後続処理 (recordFailure/recordSuccess) が完了する猶予を与えてから
// 戻る。ラウンド数はゲート送信で正確に制御するため、ここでの待機は
// 「その場で終わるはずの後続処理を待つ」ためだけの短い猶予であり、
// ティック発火回数を推測するものではない。
func waitForCalls(t *testing.T, pub *fakePublisher, n int) {
	t.Helper()
	require.Eventually(t, func() bool { return pub.calls() == n }, time.Second, time.Millisecond)
}

// waitForCircuitState は、直前の Publish 呼び出しが返した後の recordFailure /
// recordSuccess (ごく短い同期処理) が反映されるまでの猶予を与えてから状態を見る。
func waitForCircuitState(t *testing.T, m *matcher.Matcher, want bool) {
	t.Helper()
	require.Eventually(t, func() bool { return m.IsCircuitOpen() == want }, time.Second, time.Millisecond)
}

func waitForQueueSize(t *testing.T, q *fakeQueue, want int) {
	t.Helper()
	require.Eventually(t, func() bool { return q.size() == want }, time.Second, time.Millisecond)
}

// openCircuit はゲート付き publisher に threshold 回分の失敗を供給し、
// サーキットブレーカーが開くまで進める。
func openCircuit(t *testing.T, pub *fakePublisher, m *matcher.Matcher, threshold int) {
	t.Helper()
	for i := 0; i < threshold; i++ {
		pub.gate <- assert.AnError
	}
	waitForCalls(t, pub, threshold)
	waitForCircuitState(t, m, true)
}

func newOptions(overrides matcher.Options) matcher.Options {
	opts := matcher.Options{
		Interval:         testInterval,
		CircuitThreshold: 3,
		CircuitCooldown:  time.Hour,
		DrainTimeout:     matcher.DefaultDrainTimeout,
	}
	if overrides.Interval > 0 {
		opts.Interval = overrides.Interval
	}
	if overrides.CircuitThreshold > 0 {
		opts.CircuitThreshold = overrides.CircuitThreshold
	}
	if overrides.CircuitCooldown > 0 {
		opts.CircuitCooldown = overrides.CircuitCooldown
	}
	if overrides.DrainTimeout > 0 {
		opts.DrainTimeout = overrides.DrainTimeout
	}
	return opts
}
