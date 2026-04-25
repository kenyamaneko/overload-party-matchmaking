package domain

import "time"

// QueueEntry はマッチメイキングキュー内の 1 エントリを表します。
type QueueEntry struct {
	PlayerID string
	DeckID   int64
	JoinedAt time.Time
}
