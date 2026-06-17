package apimatchmakingfake

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

// matchMadeChannel は asyncapi.yaml の MatchMade channel address。物理 topic 名ではなく、
// fake broker のキーとして使う論理 channel。
const matchMadeChannel = "match-made"

// PublishMatchMade は MatchMade 論理チャネルへ MatchMadeEvent を 1 件発行する。
// EventType は EventTypeMatchMade に固定し、MatchID 未設定時は自動付与する。
func PublishMatchMade(ctx context.Context, p *Publisher, event apimatchmaking.MatchMadeEvent) error {
	event = fillMatchMadeDefaults(event)
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal MatchMadeEvent: %w", err)
	}
	return p.Publish(ctx, matchMadeChannel, data)
}

// MatchMadeExpecter は MatchMade 論理チャネルに subscribe 済みの待受器。
type MatchMadeExpecter struct {
	ch <-chan []byte
}

// ExpectMatchMade は MatchMade 論理チャネルに即時 subscribe し Expecter を返す。
// publish より前に呼び出す必要がある。
func ExpectMatchMade(s *Subscriber) *MatchMadeExpecter {
	return &MatchMadeExpecter{ch: s.Messages(matchMadeChannel)}
}

// Wait は publish された最初の MatchMadeEvent を timeout 付きで取り出す。
// timeout 超過や decode 失敗は zero 値 + error を返す。
func (e *MatchMadeExpecter) Wait(timeout time.Duration) (apimatchmaking.MatchMadeEvent, error) {
	var zero apimatchmaking.MatchMadeEvent
	select {
	case data, ok := <-e.ch:
		if !ok {
			return zero, fmt.Errorf("channel closed for %q before receiving message", matchMadeChannel)
		}
		var v apimatchmaking.MatchMadeEvent
		if err := json.Unmarshal(data, &v); err != nil {
			return zero, fmt.Errorf("unmarshal %q payload: %w", matchMadeChannel, err)
		}
		return v, nil
	case <-time.After(timeout):
		return zero, fmt.Errorf("timeout waiting for %q after %s", matchMadeChannel, timeout)
	}
}

// fillMatchMadeDefaults は MatchMadeEvent の定型フィールドを補完する。
func fillMatchMadeDefaults(event apimatchmaking.MatchMadeEvent) apimatchmaking.MatchMadeEvent {
	event.EventType = apimatchmaking.EventTypeMatchMade
	if event.MatchID == "" {
		event.MatchID = newMatchID()
	}
	return event
}
