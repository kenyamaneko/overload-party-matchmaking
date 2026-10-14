// Package matchertest は matcher のテストが時間と ID 発行を決定的に扱うための偽物を提供する。
package matchertest

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
)

const unreadTickPollInterval = time.Millisecond

var _ port.Clock = (*Clock)(nil)

// Clock は Advance を呼ぶまで時刻が進まない port.Clock の偽物です。
type Clock struct {
	mu      sync.Mutex
	now     time.Time
	timers  []*timer
	tickers []*ticker
	changed chan struct{}
}

type timer struct {
	at time.Time
	ch chan time.Time
}

type ticker struct {
	clock    *Clock
	interval time.Duration
	next     time.Time
	ch       chan time.Time
	isActive bool
}

// NewClock は start を現在時刻とする Clock を生成します。
func NewClock(start time.Time) *Clock {
	return &Clock{now: start, changed: make(chan struct{})}
}

// Now は Advance で進めた現在時刻を返します。
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// After は Advance で d 以上進んだときに時刻を通知するチャネルを返します。
func (c *Clock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &timer{at: c.now.Add(d), ch: make(chan time.Time, 1)}
	c.timers = append(c.timers, t)
	c.notifyChanged()
	return t.ch
}

// NewTicker は Advance で d 進むたびに時刻を通知する port.Ticker を返します。
func (c *Clock) NewTicker(d time.Duration) port.Ticker {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &ticker{clock: c, interval: d, next: c.now.Add(d), ch: make(chan time.Time, 1), isActive: true}
	c.tickers = append(c.tickers, t)
	c.notifyChanged()
	return t
}

// Advance は現在時刻を d だけ進め、その間に期限が来た After と Ticker を時刻順に通知します。
// 受け手が前の通知を読んでいないチャネルへは、time.Ticker と同じく通知を捨てて待たない。
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	target := c.now.Add(d)
	for {
		at, fire, ok := c.earliestDue(target)
		if !ok {
			break
		}
		c.now = at
		fire()
	}
	c.now = target
}

// WaitForTimers は、まだ通知していない After が n 件以上になるまで待ちます。
// 待ちが ctx で打ち切られたときは ctx のエラーを返します。
func (c *Clock) WaitForTimers(ctx context.Context, n int) error {
	return c.waitFor(ctx, func() bool { return len(c.timers) >= n })
}

// WaitForTickers は、止められていない Ticker が n 件以上になるまで待ちます。
// 待ちが ctx で打ち切られたときは ctx のエラーを返します。
func (c *Clock) WaitForTickers(ctx context.Context, n int) error {
	return c.waitFor(ctx, func() bool { return c.countActiveTickers() >= n })
}

// WaitForTicksConsumed は、止められていない Ticker が通知した時刻を、受け手が全て受け取るまで待ちます。
// 待ちが ctx で打ち切られたときは ctx のエラーを返します。
func (c *Clock) WaitForTicksConsumed(ctx context.Context) error {
	for c.hasUnreadTick() {
		select {
		case <-time.After(unreadTickPollInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (c *Clock) hasUnreadTick() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.tickers {
		if t.isActive && len(t.ch) > 0 {
			return true
		}
	}
	return false
}

func (c *Clock) waitFor(ctx context.Context, isSatisfied func() bool) error {
	for {
		c.mu.Lock()
		if isSatisfied() {
			c.mu.Unlock()
			return nil
		}
		changed := c.changed
		c.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *Clock) countActiveTickers() int {
	n := 0
	for _, t := range c.tickers {
		if t.isActive {
			n++
		}
	}
	return n
}

// earliestDue は target 以前に期限が来る通知のうち最も早いものの時刻と、その通知を行う関数を返す。
func (c *Clock) earliestDue(target time.Time) (time.Time, func(), bool) {
	type due struct {
		at   time.Time
		fire func()
	}
	var dues []due
	for _, t := range c.timers {
		if !t.at.After(target) {
			dues = append(dues, due{at: t.at, fire: func() { c.fireTimer(t) }})
		}
	}
	for _, t := range c.tickers {
		if t.isActive && !t.next.After(target) {
			dues = append(dues, due{at: t.next, fire: func() { t.fire() }})
		}
	}
	if len(dues) == 0 {
		return time.Time{}, nil, false
	}
	sort.SliceStable(dues, func(i, j int) bool { return dues[i].at.Before(dues[j].at) })
	return dues[0].at, dues[0].fire, true
}

func (c *Clock) fireTimer(t *timer) {
	sendWithoutWaiting(t.ch, c.now)
	remaining := c.timers[:0]
	for _, other := range c.timers {
		if other != t {
			remaining = append(remaining, other)
		}
	}
	c.timers = remaining
}

func (c *Clock) notifyChanged() {
	close(c.changed)
	c.changed = make(chan struct{})
}

func (t *ticker) fire() {
	sendWithoutWaiting(t.ch, t.clock.now)
	t.next = t.next.Add(t.interval)
}

// C は Advance で通知された時刻を受け取るチャネルを返します。
func (t *ticker) C() <-chan time.Time {
	return t.ch
}

// Stop は以降の通知を止めます。
func (t *ticker) Stop() {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	t.isActive = false
	t.clock.notifyChanged()
}

func sendWithoutWaiting(ch chan time.Time, at time.Time) {
	select {
	case ch <- at:
	default:
	}
}
