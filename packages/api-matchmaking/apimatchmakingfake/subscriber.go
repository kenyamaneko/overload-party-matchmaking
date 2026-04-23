package apimatchmakingfake

// Subscriber は受信側サービス (gateway) のテスト用 fake。Broker 経由で到着する
// payload bytes を topic 単位の channel で受け取る。typed helper や Stream は
// 内部で本 Subscriber を使用する。
type Subscriber struct {
	broker *Broker
}

// NewSubscriber は指定 broker に紐づく Subscriber を返す。
func NewSubscriber(broker *Broker) *Subscriber {
	return &Subscriber{broker: broker}
}

// Messages は topic に発行された payload を読み取る channel を返す。複数回
// 呼ぶと独立した channel が返り、fan-out 検証にも使える。
func (s *Subscriber) Messages(topic string) <-chan []byte {
	return s.broker.Subscribe(topic)
}
