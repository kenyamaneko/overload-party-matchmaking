package apimatchmakingfake

import (
	"context"
	"sync"
)

// PublishedMessage は Publisher.Publish 1 回分の記録。
type PublishedMessage struct {
	Topic string
	Data  []byte
}

// Publisher は送信側サービス (matchmaking) のテスト用 fake。
type Publisher struct {
	broker    *Broker
	mu        sync.Mutex
	published []PublishedMessage
}

// NewPublisher は指定 broker に紐づく Publisher を返す。
func NewPublisher(broker *Broker) *Publisher {
	return &Publisher{broker: broker}
}

// Publish は data を topic に発行する。
// ctx は本 fake 内部では使わないが、本番実装 (pubsub.Publisher) と同じ interface を満たすために受ける。
func (p *Publisher) Publish(_ context.Context, topic string, data []byte) error {
	// caller 配下の buffer から独立させ、後から mutate されても記録が壊れないようにする。
	buf := append([]byte(nil), data...)
	p.mu.Lock()
	p.published = append(p.published, PublishedMessage{Topic: topic, Data: buf})
	p.mu.Unlock()
	p.broker.Publish(topic, buf)
	return nil
}

// Published は Publisher が発行したメッセージの snapshot を返す。
// slice も Data も caller 専用の新規割付で内部とは独立する。
func (p *Publisher) Published() []PublishedMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]PublishedMessage, len(p.published))
	for i, m := range p.published {
		out[i] = PublishedMessage{
			Topic: m.Topic,
			Data:  append([]byte(nil), m.Data...),
		}
	}
	return out
}
