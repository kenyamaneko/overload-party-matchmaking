package port

import "context"

// Event は EventBuilder が usecase に渡す publish 単位。
type Event struct {
	EventType string
	Payload   []byte
}

// MatchedPlayer は EventBuilder.BuildMatchMade への入力。
type MatchedPlayer struct {
	PlayerID string
	DeckID   int64
}

// RawEventPublisher は usecase が呼ぶ Pub/Sub 送出の低レベル interface。
type RawEventPublisher interface {
	Publish(ctx context.Context, eventType string, payload []byte) error
}

// EventBuilder は usecase が発行したいビジネスイベントを Event にシリアライズする。
type EventBuilder interface {
	BuildMatchMade(matchID string, players []MatchedPlayer) (Event, error)
}
