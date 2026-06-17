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

// New は物理 topic 名から eventType→topic mapping を構築する。
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
	for _, publisher := range p.byEventType {
		publisher.Stop()
	}
	return p.client.Close()
}

// Publish は eventType を物理 topic に解決して送出する。未登録 eventType はエラーで返し、
// builder/publisher の設定不一致を fail-fast で検出する。
func (p *Publisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	publisher, ok := p.byEventType[eventType]
	if !ok {
		return fmt.Errorf("pubsub: unknown event type %q", eventType)
	}
	result := publisher.Publish(ctx, &gpubsub.Message{Data: payload})
	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("pubsub: publish event_type=%s: %w", eventType, err)
	}
	return nil
}
