package pubsub

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

var _ port.EventBuilder = (*EventBuilder)(nil)

// EventBuilder はマッチ成立イベントを Event に構築する。
// event struct のスキーマ (apimatchmaking.*) を知っているのは pubsub adapter のみに
// 閉じ込め、usecase は payload を不透明な []byte として扱う。
//
// topic 名は Publisher と共通の設定値から渡す (build 時と publish 時で不一致が
// 起きないようにするため)。
type EventBuilder struct {
	matchMadeTopic string
}

// NewEventBuilder は match-made の送信先 topic 名を持つ EventBuilder を構築する。
func NewEventBuilder(matchMadeTopic string) (*EventBuilder, error) {
	if matchMadeTopic == "" {
		return nil, errors.New("pubsub: matchMadeTopic is required")
	}
	return &EventBuilder{matchMadeTopic: matchMadeTopic}, nil
}

// BuildMatchMade はマッチ成立イベントを構築する。
// gateway は payload 内 matchId をインメモリ dedup key として使う。
func (b *EventBuilder) BuildMatchMade(matchID string, players []port.MatchedPlayer) (port.Event, error) {
	if matchID == "" {
		return port.Event{}, errors.New("pubsub: matchID is empty")
	}
	if len(players) == 0 {
		return port.Event{}, errors.New("pubsub: players is empty")
	}
	apiPlayers := make([]apimatchmaking.MatchedPlayer, 0, len(players))
	for _, p := range players {
		apiPlayers = append(apiPlayers, apimatchmaking.MatchedPlayer{
			PlayerID: p.PlayerID,
			DeckID:   p.DeckID,
		})
	}
	ev := apimatchmaking.MatchMadeEvent{
		Type:    apimatchmaking.EventTypeMatchMade,
		MatchID: matchID,
		Players: apiPlayers,
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return port.Event{}, fmt.Errorf("marshal match-made: %w", err)
	}
	return port.Event{
		Topic:   b.matchMadeTopic,
		Payload: payload,
	}, nil
}
