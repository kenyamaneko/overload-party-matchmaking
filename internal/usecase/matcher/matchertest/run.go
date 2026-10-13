package matchertest

import (
	"context"
	"sync"
	"testing"
	"time"
)

const (
	// WaitLimit は条件が満たされるのを待つ時間の上限です。
	WaitLimit = 5 * time.Second
	// PollInterval は条件を確かめ直す間隔です。
	PollInterval = 2 * time.Millisecond

	quietPolls = 8

	// 時計を進めて待ち中の呼び出しを全て期限切れにするため、どの待ち時間より長くする。
	shutdownClockAdvance = time.Hour
)

// Gate は Open を呼ぶまで、待っている呼び出しを止める門です。
type Gate struct {
	once   sync.Once
	opened chan struct{}
}

// NewGate は閉じた門を生成します。
func NewGate() *Gate {
	return &Gate{opened: make(chan struct{})}
}

// Open は門を開きます。何度呼んでも構いません。
func (g *Gate) Open() {
	g.once.Do(func() { close(g.opened) })
}

// Opened は門が開くと閉じられるチャネルを返します。
func (g *Gate) Opened() <-chan struct{} {
	return g.opened
}

// WaitContext は WaitLimit で期限が切れる ctx を返します。
func WaitContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), WaitLimit)
	t.Cleanup(cancel)
	return ctx
}

// WaitUntilQuiet は activity が返す値が一定の間変わらなくなるまで待ちます。
func WaitUntilQuiet(t *testing.T, activity func() int) {
	t.Helper()
	deadline := time.Now().Add(WaitLimit)
	last := activity()
	stable := 0
	for stable < quietPolls && time.Now().Before(deadline) {
		time.Sleep(PollInterval)
		current := activity()
		stable++
		if current != last {
			last = current
			stable = 0
		}
	}
	if stable < quietPolls {
		t.Errorf("activity did not become quiet within %s", WaitLimit)
	}
}

// Running は別のゴルーチンで動いている定期処理の繰り返しです。
type Running struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// StartRun は run を別のゴルーチンで開始します。
// テストの終了時にシャットダウンを始め、run が返り、かつ isIdle が true を返すまで待ちます。
func StartRun(t *testing.T, clock *Clock, isIdle func() bool, run func(ctx context.Context)) *Running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &Running{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(r.done)
		run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		deadline := time.Now().Add(WaitLimit)
		for time.Now().Before(deadline) && !(r.IsDone() && isIdle()) {
			clock.Advance(shutdownClockAdvance)
			time.Sleep(PollInterval)
		}
		if !r.IsDone() || !isIdle() {
			t.Errorf("run did not finish within %s after shutdown", WaitLimit)
		}
	})
	return r
}

// Shutdown はシャットダウンを始めます。
func (r *Running) Shutdown() {
	r.cancel()
}

// IsDone は run が返ったかを返します。
func (r *Running) IsDone() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}
