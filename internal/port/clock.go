package port

import "time"

// Clock は usecase が現在時刻と待ちを得る interface です。テストが時間を進められるよう差し替え可能にする。
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
	NewTicker(d time.Duration) Ticker
}

// Ticker は Clock.NewTicker が返す、一定間隔で時刻を通知する部品です。
type Ticker interface {
	C() <-chan time.Time
	Stop()
}
