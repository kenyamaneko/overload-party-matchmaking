// Package pubsub は matchmaking の Pub/Sub publisher。usecase から呼ばれる
// 低レベル送信層で、論理 eventType を物理 topic に解決して送出する。
//
// matchmaking が発行するサービス横断イベント:
//
//   - apimatchmaking.EventTypeMatchMade — マッチ成立時に発行。gateway が
//     subscribe して接続中プレイヤーへ WS push する。
package pubsub

import (
	"context"
	"errors"
	"fmt"

	gpubsub "cloud.google.com/go/pubsub/v2"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

var _ port.RawEventPublisher = (*Publisher)(nil)

// Publisher は port.RawEventPublisher を実装する。
type Publisher struct {
	client      *gpubsub.Client
	byEventType map[string]*gpubsub.Publisher
}

// New は物理 topic 名から eventType→topic mapping を構築する。topic 名は
// configmap / env で外から差し替えできるよう構築時に受け取る。topic は
// Terraform (modules/pubsub) で事前作成されている前提。
func New(ctx context.Context, projectID, matchMadeTopic string) (*Publisher, error) {
	if projectID == "" {
		return nil, errors.New("pubsub: projectID is empty")
	}
	if matchMadeTopic == "" {
		return nil, errors.New("pubsub: matchMadeTopic is required")
	}
	client, err := gpubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub: new pubsub client: %w", err)
	}
	return &Publisher{
		client: client,
		byEventType: map[string]*gpubsub.Publisher{
			apimatchmaking.EventTypeMatchMade: client.Publisher(matchMadeTopic),
		},
	}, nil
}

// Close は in-flight メッセージを flush し Pub/Sub client を閉じる。
func (p *Publisher) Close() error {
	for _, pub := range p.byEventType {
		pub.Stop()
	}
	return p.client.Close()
}

// Publish は未登録 eventType をエラーで返し、builder と publisher の設定不一致を
// fail-fast で検出する (Pub/Sub SDK に届く前に失敗させる)。
func (p *Publisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	pub, ok := p.byEventType[eventType]
	if !ok {
		return fmt.Errorf("pubsub: unknown event type %q", eventType)
	}
	result := pub.Publish(ctx, &gpubsub.Message{Data: payload})
	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("pubsub: publish event_type=%s: %w", eventType, err)
	}
	return nil
}
