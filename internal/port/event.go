package port

import "context"

// RawEventPublisher は usecase が呼ぶ Pub/Sub 送出の低レベル interface。
// 引数 eventType は presenter が返す論理 event 種別、payload は marshal 済み wire bytes。
type RawEventPublisher interface {
	Publish(ctx context.Context, eventType string, payload []byte) error
}
