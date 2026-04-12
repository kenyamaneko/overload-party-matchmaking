package domain

import "time"

// QueueEntry はマッチメイキングキュー内の 1 エントリを表します。
type QueueEntry struct {
	PlayerID string
	DeckID   int64
	JoinedAt time.Time
}

// MatchedPlayer はマッチ成立したプレイヤーの情報を保持します。
type MatchedPlayer struct {
	PlayerID string `json:"playerId"`
	DeckID   int64  `json:"deckId"`
}

// MatchMadeEvent はマッチ成立時に Pub/Sub へ発行されるイベントです。
type MatchMadeEvent struct {
	Type    string          `json:"type"`
	MatchID string          `json:"matchId"`
	Players []MatchedPlayer `json:"players"`
}
