package matcher

import (
	"log/slog"
	"sync"
	"time"
)

// circuitBreaker は取り出しと発行の連続失敗を別々に数え、どちらかが閾値に達すると定期処理を止めます。
type circuitBreaker struct {
	threshold int
	cooldown  time.Duration

	mu sync.Mutex
	// 空の取り出しの成功で発行の失敗の数が消えないよう、失敗の数は取り出しと発行で別に持つ。
	popFailureCount     int
	publishFailureCount int
	isOpen              bool
	openedAt            time.Time
	hasLoggedSkip       bool
}

func newCircuitBreaker(threshold int, cooldown time.Duration) *circuitBreaker {
	return &circuitBreaker{threshold: threshold, cooldown: cooldown}
}

func (c *circuitBreaker) isCircuitOpen() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isOpen
}

// allowTick は定期処理で取り出しを試みてよいかを返す。開いている間は、開いてから cooldown が
// 経つまで止める。止めている間の最初の 1 回だけ shouldLogSkip を true にする。
func (c *circuitBreaker) allowTick(now time.Time) (isAllowed bool, shouldLogSkip bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.isOpen {
		return true, false
	}
	if now.Sub(c.openedAt) >= c.cooldown {
		return true, false
	}
	if !c.hasLoggedSkip {
		c.hasLoggedSkip = true
		return false, true
	}
	return false, false
}

func (c *circuitBreaker) recordPopSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.popFailureCount = 0
	c.closeIfRecovered()
}

func (c *circuitBreaker) recordPopFailure(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.popFailureCount++
	if c.popFailureCount >= c.threshold {
		c.open(now, c.popFailureCount)
	}
}

func (c *circuitBreaker) recordPublishSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.publishFailureCount = 0
	c.closeIfRecovered()
}

func (c *circuitBreaker) recordPublishFailure(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.publishFailureCount++
	if c.publishFailureCount >= c.threshold {
		c.open(now, c.publishFailureCount)
	}
}

// open は閾値に達した失敗の時点から cooldown を数え直す。すでに開いているときも数え直す。
func (c *circuitBreaker) open(now time.Time, consecutiveFailures int) {
	if c.isOpen {
		slog.Warn("matcher circuit trial failed, reopening", "failures", consecutiveFailures)
	} else {
		slog.Warn("matcher circuit opened",
			"consecutive_failures", consecutiveFailures, "threshold", c.threshold, "cooldown", c.cooldown)
	}
	c.isOpen = true
	c.openedAt = now
	c.hasLoggedSkip = false
}

func (c *circuitBreaker) closeIfRecovered() {
	if !c.isOpen {
		return
	}
	if c.popFailureCount >= c.threshold || c.publishFailureCount >= c.threshold {
		return
	}
	slog.Info("matcher circuit closed after recovery")
	c.isOpen = false
	c.hasLoggedSkip = false
}
