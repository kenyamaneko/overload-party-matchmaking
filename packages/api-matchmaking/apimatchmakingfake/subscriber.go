package apimatchmakingfake

// Subscriber は受信側サービス (gateway) のテスト用 fake。
type Subscriber struct {
	broker *Broker
}

// NewSubscriber は指定 broker に紐づく Subscriber を返す。
func NewSubscriber(broker *Broker) *Subscriber {
	return &Subscriber{broker: broker}
}

// Messages は topic に発行された payload を読み取る channel を返す。
// 複数回呼ぶと独立した channel が返る (fan-out 検証用)。
func (s *Subscriber) Messages(topic string) <-chan []byte {
	return s.broker.Subscribe(topic)
}
