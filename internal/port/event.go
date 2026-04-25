package port

import "context"

// Event は EventBuilder が usecase に渡す publish 単位。
//
// outbox を持たない matchmaking では EventID を保持しない (gateway 側の
// dedup key は payload 内 matchId を使う)。
type Event struct {
	Topic   string
	Payload []byte
}

// MatchedPlayer は EventBuilder.BuildMatchMade への入力。
// domain 型と独立させ、port 層が外部スキーマに直接依存しないようにする。
type MatchedPlayer struct {
	PlayerID string
	DeckID   int64
}

// RawEventPublisher は usecase が topic + payload で Cloud Pub/Sub に送出する
// 低レベルインターフェース。イベント struct の構築は adapter/pubsub の
// EventBuilder が担い、usecase は構築済み payload を流すだけ。
type RawEventPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// EventBuilder は usecase が発行したいビジネスイベントを Event に
// シリアライズする。apimatchmaking スキーマの詳細は adapter/pubsub 側に
// 閉じ込め、usecase は「何を発行したいか」だけを述語で伝える。
type EventBuilder interface {
	BuildMatchMade(matchID string, players []MatchedPlayer) (Event, error)
}
