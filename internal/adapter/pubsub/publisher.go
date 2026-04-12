package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/pubsub"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
)

// Publisher は Cloud Pub/Sub へマッチイベントを発行します。
type Publisher struct {
	client *pubsub.Client
	topic  *pubsub.Topic
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
	topic := client.Topic(topicID)
	ok, err := topic.Exists(ctx)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("pubsub: topic exists check: %w", err)
	}
	if !ok {
		_ = client.Close()
		return nil, fmt.Errorf("pubsub: topic %q does not exist in project %q", topicID, projectID)
	}
	return &Publisher{client: client, topic: topic}, nil
}

// PublishMatchMade はマッチ成立イベントを Pub/Sub トピックに発行します。
func (p *Publisher) PublishMatchMade(ctx context.Context, event domain.MatchMadeEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("pubsub: marshal event: %w", err)
	}
	result := p.topic.Publish(ctx, &pubsub.Message{
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

// Close は Pub/Sub トピックとクライアントを閉じます。
func (p *Publisher) Close() error {
	p.topic.Stop()
	return p.client.Close()
}
