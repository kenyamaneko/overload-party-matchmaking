// Package apimatchmakingfake は api-matchmaking の送信側サービス (matchmaking)
// が consumer 側サービスに提供するテスト用 fake。送信側パッケージがテスト
// ダブルを同梱することで、consumer 側の手書き fake が契約と乖離するのを防ぐ。
//
// 基本構成: Broker + Publisher + Subscriber + Stream + typed helper。
//   - Broker: topic 名をキーとした in-memory pub/sub 基盤 (低レベル API)
//   - Publisher: 送信側サービス (matchmaking) の自テストが発行アサーションに使う fake
//   - Subscriber: 受信側サービス (gateway) のテストで消費検証に使う fake
//   - Stream: port.MessageStream 相当の observable stream (consumer 側 adapter 不要)
//   - typed helper: TopicMatchmakingEvents / MatchMadeEvent に対応する型付き API
package apimatchmakingfake

import "sync"

// Broker は apimatchmakingfake 内の publisher / subscriber を仲介する topic
// ベースの in-memory pub/sub 実装。テスト毎に NewBroker で新規生成する想定で、
// 状態はプロセス内揮発的で永続化・再送・配信保証は行わない。
type Broker struct {
	mu      sync.Mutex
	byTopic map[string][]chan []byte
}

// NewBroker は空の in-memory broker を返す。
func NewBroker() *Broker {
	return &Broker{byTopic: make(map[string][]chan []byte)}
}

// Publish は topic に subscribe している全 channel に data を配信する。
// channel 満杯時はブロックする — テストがメッセージをドレインし忘れた場合に
// ハングが観測できるようにする意図的な選択 (silent drop だとテスト漏れが隠れる)。
func (b *Broker) Publish(topic string, data []byte) {
	b.mu.Lock()
	subs := append([]chan []byte(nil), b.byTopic[topic]...)
	b.mu.Unlock()
	for _, ch := range subs {
		ch <- data
	}
}

// Subscribe は topic に対する新しい受信 channel を返す。buffer サイズは 100。
// 複数回呼べば複数 subscriber 扱いとなり、Publish 時に fan-out される。
func (b *Broker) Subscribe(topic string) <-chan []byte {
	ch := make(chan []byte, defaultSubscribeBuffer)
	b.mu.Lock()
	b.byTopic[topic] = append(b.byTopic[topic], ch)
	b.mu.Unlock()
	return ch
}

// defaultSubscribeBuffer は Subscribe が返す channel の buffer サイズ。
// buffer を非ゼロにすることで、Publish が handler 経由の消費完了を待たずに
// 送信側へ戻れる (テストでの publish→assert ペアを同期的に書くため)。
const defaultSubscribeBuffer = 100
