package port

import "context"

// Event は EventBuilder が usecase に渡す publish 単位。
// EventType を SSoT とし、物理 topic への解決は RawEventPublisher 内で行う。
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
// eventType (apimatchmaking.EventType*) → 物理 topic への解決は adapter 内部で行う。
type RawEventPublisher interface {
	Publish(ctx context.Context, eventType string, payload []byte) error
}

// EventBuilder は usecase が発行したいビジネスイベントを Event に
// シリアライズする。apimatchmaking スキーマの詳細は adapter/pubsub に閉じる。
type EventBuilder interface {
	BuildMatchMade(matchID string, players []MatchedPlayer) (Event, error)
}
