package domain

import "time"

// QueueEntry はマッチメイキングキュー内の 1 エントリを表します。
// Name / Level は enqueue 時に呼び出し側 (gateway 経由) から渡される player summary snapshot。
// matchmaking 内部は account に lookup せず、与えられた値を保持して match_made event に伝搬する。
type QueueEntry struct {
	PlayerID string
	DeckID   int64
	Name     string
	Level    int64
	JoinedAt time.Time
}
