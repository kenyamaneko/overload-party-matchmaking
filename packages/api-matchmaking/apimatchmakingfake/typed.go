package apimatchmakingfake

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

// PublishMatchMade は TopicMatchmakingEvents へ MatchMadeEvent を 1 件発行する。
// Type は EventTypeMatchMade に固定し、MatchID 未設定時は自動付与する。
func PublishMatchMade(ctx context.Context, p *Publisher, ev apimatchmaking.MatchMadeEvent) error {
	ev = fillMatchMadeDefaults(ev)
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal MatchMadeEvent: %w", err)
	}
	return p.Publish(ctx, apimatchmaking.TopicMatchmakingEvents, data)
}

// MatchMadeExpecter は TopicMatchmakingEvents に subscribe 済みの待受器。
type MatchMadeExpecter struct {
	ch <-chan []byte
}

// ExpectMatchMade は TopicMatchmakingEvents に即時 subscribe し Expecter を返す。
// publish より前に呼び出す必要がある。
func ExpectMatchMade(s *Subscriber) *MatchMadeExpecter {
	return &MatchMadeExpecter{ch: s.Messages(apimatchmaking.TopicMatchmakingEvents)}
}

// Wait は publish された最初の MatchMadeEvent を timeout 付きで取り出す。
// timeout 超過や decode 失敗は zero 値 + error を返す。
func (e *MatchMadeExpecter) Wait(timeout time.Duration) (apimatchmaking.MatchMadeEvent, error) {
	var zero apimatchmaking.MatchMadeEvent
	select {
	case data, ok := <-e.ch:
		if !ok {
			return zero, fmt.Errorf("channel closed for topic %q before receiving message",
				apimatchmaking.TopicMatchmakingEvents)
		}
		var v apimatchmaking.MatchMadeEvent
		if err := json.Unmarshal(data, &v); err != nil {
			return zero, fmt.Errorf("unmarshal %q payload: %w",
				apimatchmaking.TopicMatchmakingEvents, err)
		}
		return v, nil
	case <-time.After(timeout):
		return zero, fmt.Errorf("timeout waiting for %q after %s",
			apimatchmaking.TopicMatchmakingEvents, timeout)
	}
}

// fillMatchMadeDefaults は MatchMadeEvent の定型フィールドを補完する。
func fillMatchMadeDefaults(ev apimatchmaking.MatchMadeEvent) apimatchmaking.MatchMadeEvent {
	ev.Type = apimatchmaking.EventTypeMatchMade
	if ev.MatchID == "" {
		ev.MatchID = newMatchID()
	}
	return ev
}
