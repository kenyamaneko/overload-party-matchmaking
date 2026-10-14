// Package systemclock は port.Clock の実時計による実装を提供する。
package systemclock

import (
	"time"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
)

var _ port.Clock = Clock{}

// Clock は OS の時計に従う port.Clock です。
type Clock struct{}

// New は Clock を生成します。
func New() Clock {
	return Clock{}
}

// Now は現在時刻を返します。
func (Clock) Now() time.Time {
	return time.Now()
}

// After は d が経過したあとに時刻を通知するチャネルを返します。
func (Clock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// NewTicker は d ごとに時刻を通知する port.Ticker を返します。
func (Clock) NewTicker(d time.Duration) port.Ticker {
	return ticker{t: time.NewTicker(d)}
}

type ticker struct {
	t *time.Ticker
}

func (k ticker) C() <-chan time.Time {
	return k.t.C
}

func (k ticker) Stop() {
	k.t.Stop()
}
