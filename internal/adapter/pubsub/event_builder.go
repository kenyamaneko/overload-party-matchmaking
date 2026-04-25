package pubsub

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

var _ port.EventBuilder = (*EventBuilder)(nil)

// EventBuilder は port.EventBuilder を実装する。
// apimatchmaking スキーマを usecase から隠し、usecase は payload を不透明な []byte として扱う。
type EventBuilder struct{}

// NewEventBuilder は EventBuilder を構築する。
func NewEventBuilder() *EventBuilder {
	return &EventBuilder{}
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
		EventType: apimatchmaking.EventTypeMatchMade,
		Payload:   payload,
	}, nil
}
