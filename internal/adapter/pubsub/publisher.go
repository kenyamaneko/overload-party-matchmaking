// Package pubsub は matchmaking サービスの Cloud Pub/Sub publisher を提供する。
//
// matchmaking は 1 種のサービス横断イベントを発行する:
//
//   - `match_made` — マッチ成立時に発行。gateway が subscribe して接続中
//     プレイヤーへ WS push する。
//
// Publisher は usecase (matcher) から topic + payload 指定で呼ばれる低レベル
// 送信層。ビジネス要件に応じたイベント struct の構築は EventBuilder が担う。
package pubsub

import (
	"context"
	"errors"
	"fmt"

	gpubsub "cloud.google.com/go/pubsub/v2"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
)

var _ port.RawEventPublisher = (*Publisher)(nil)

// Publisher は matchmaking が publish する全 topic をまとめる。起動時に一度だけ
// 構築し、Close で in-flight メッセージを flush して gRPC client を解放する。
// port.RawEventPublisher を実装する。
//
// byTopic は map 構造で将来の topic 追加を許容する。現状 matchmaking の
// publish topic は `match_made` 1 本のみだが、shop / scenario の publisher 層
// と構造を揃え、O(1) で dispatch できる形を維持するために map を保持する。
type Publisher struct {
	client  *gpubsub.Client
	byTopic map[string]*gpubsub.Publisher
}

// NewPublisher は指定 project + topic 名に紐づく Publisher を構築する。
// topic は Terraform で事前作成されている前提。
func NewPublisher(ctx context.Context, projectID, matchMadeTopic string) (*Publisher, error) {
	if projectID == "" {
		return nil, errors.New("pubsub: projectID is empty")
	}
	if matchMadeTopic == "" {
		return nil, errors.New("pubsub: matchMadeTopic is empty")
	}
	client, err := gpubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub: new client: %w", err)
	}
	return &Publisher{
		client: client,
		byTopic: map[string]*gpubsub.Publisher{
			matchMadeTopic: client.Publisher(matchMadeTopic),
		},
	}, nil
}

// Publish は topic 名で指定された Cloud Pub/Sub topic に payload をそのまま送る。
// 未登録 topic はエラーを返す (builder と publisher の topic 設定不一致を
// fail-fast で検出するため)。
func (p *Publisher) Publish(ctx context.Context, topic string, payload []byte) error {
	pub, ok := p.byTopic[topic]
	if !ok {
		return fmt.Errorf("pubsub: unknown topic %q", topic)
	}
	result := pub.Publish(ctx, &gpubsub.Message{Data: payload})
	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("pubsub: publish to %s: %w", topic, err)
	}
	return nil
}

// Close は in-flight メッセージを flush し Cloud Pub/Sub client を閉じる。
func (p *Publisher) Close() error {
	for _, pub := range p.byTopic {
		pub.Stop()
	}
	return p.client.Close()
}
