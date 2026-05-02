// Package apimatchmakingfake は api-matchmaking 送信側サービス (matchmaking) が
// consumer 側サービスに提供するテスト用 fake。送信側パッケージがテストダブルを同梱することで
// consumer 側の手書き fake が契約と乖離するのを防ぐ。
package apimatchmakingfake

import "sync"

// Broker は publisher / subscriber を仲介する in-memory pub/sub。
type Broker struct {
	mu      sync.Mutex
	byTopic map[string][]chan []byte
}

// NewBroker は空の in-memory broker を返す。
func NewBroker() *Broker {
	return &Broker{byTopic: make(map[string][]chan []byte)}
}

// Publish は topic に subscribe している全 channel に data を配信する。
// channel 満杯時はブロックする (silent drop による検出漏れを避けるため)。
func (b *Broker) Publish(topic string, data []byte) {
	b.mu.Lock()
	subs := append([]chan []byte(nil), b.byTopic[topic]...)
	b.mu.Unlock()
	for _, ch := range subs {
		ch <- data
	}
}

// Subscribe は topic に対する新しい受信 channel を返す。
// 複数回呼べば複数 subscriber 扱いとなり Publish 時に fan-out される。
func (b *Broker) Subscribe(topic string) <-chan []byte {
	ch := make(chan []byte, defaultSubscribeBuffer)
	b.mu.Lock()
	b.byTopic[topic] = append(b.byTopic[topic], ch)
	b.mu.Unlock()
	return ch
}

// defaultSubscribeBuffer は Subscribe が返す channel の buffer サイズ。
// 非ゼロにすることで Publish が consumer の消費完了を待たずに進める。
const defaultSubscribeBuffer = 100
