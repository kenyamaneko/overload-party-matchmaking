package presenter

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

// ToMatchMadeWire は domain event を wire payload に詰め替えて event_type と marshal 済み bytes を返す。
// 呼び出し側は戻り値の eventType と payload をそのまま port.RawEventPublisher.Publish に渡せる。
func ToMatchMadeWire(event domain.MatchMadeEvent) (eventType string, payload []byte, err error) {
	if event.MatchID == "" {
		return "", nil, errors.New("presenter: MatchMadeEvent.MatchID is empty")
	}
	if len(event.Players) == 0 {
		return "", nil, errors.New("presenter: MatchMadeEvent.Players is empty")
	}
	apiPlayers := make([]apimatchmaking.MatchedPlayer, 0, len(event.Players))
	for _, p := range event.Players {
		apiPlayers = append(apiPlayers, apimatchmaking.MatchedPlayer{
			PlayerID: p.PlayerID,
			DeckID:   p.DeckID,
			Name:     p.Name,
			Level:    p.Level,
		})
	}
	wire := apimatchmaking.MatchMadeEvent{
		EventType: apimatchmaking.EventTypeMatchMade,
		MatchID:   event.MatchID,
		Players:   apiPlayers,
	}
	payload, err = json.Marshal(wire)
	if err != nil {
		return "", nil, fmt.Errorf("marshal MatchMadeEvent: %w", err)
	}
	return apimatchmaking.EventTypeMatchMade, payload, nil
}
