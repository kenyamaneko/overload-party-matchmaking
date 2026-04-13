package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/pubsub/v2"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
)

// Publisher は Cloud Pub/Sub へマッチイベントを発行します。
type Publisher struct {
	client *pubsub.Client
	pub    *pubsub.Publisher
}

// NewPublisher は Pub/Sub クライアントを初期化し Publisher を生成します。
func NewPublisher(ctx context.Context, projectID, topicID string) (*Publisher, error) {
	if projectID == "" {
		return nil, fmt.Errorf("pubsub: projectID is empty")
	}
	if topicID == "" {
		return nil, fmt.Errorf("pubsub: topicID is empty")
	}
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub: new client: %w", err)
	}
	return &Publisher{client: client, pub: client.Publisher(topicID)}, nil
}

// PublishMatchMade はマッチ成立イベントを Pub/Sub トピックに発行します。
func (p *Publisher) PublishMatchMade(ctx context.Context, event domain.MatchMadeEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("pubsub: marshal event: %w", err)
	}
	result := p.pub.Publish(ctx, &pubsub.Message{
		Data: payload,
		Attributes: map[string]string{
			"type":    event.Type,
			"matchId": event.MatchID, // gateway 側の dedup key
		},
	})
	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("pubsub: publish: %w", err)
	}
	return nil
}

// Close は Publisher を停止して Pub/Sub クライアントを閉じます。
func (p *Publisher) Close() error {
	p.pub.Stop()
	return p.client.Close()
}
