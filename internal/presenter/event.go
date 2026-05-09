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
func ToMatchMadeWire(ev domain.MatchMadeEvent) (eventType string, payload []byte, err error) {
	if ev.MatchID == "" {
		return "", nil, errors.New("presenter: MatchMadeEvent.MatchID is empty")
	}
	if len(ev.Players) == 0 {
		return "", nil, errors.New("presenter: MatchMadeEvent.Players is empty")
	}
	// asyncapi-codegen-tools が $ref-in-items を未サポートのため Players は
	// []map[string]interface{} 型 (kenyamaneko/overload-party-common#48)。
	// JSON wire は asyncapi.yaml のスキーマ通り {player_id, deck_id} を満たす。
	apiPlayers := make([]map[string]interface{}, 0, len(ev.Players))
	for _, p := range ev.Players {
		apiPlayers = append(apiPlayers, map[string]interface{}{
			"player_id": p.PlayerID,
			"deck_id":   p.DeckID,
		})
	}
	wire := apimatchmaking.MatchMadeEvent{
		EventType: apimatchmaking.EventTypeMatchMade,
		MatchID:   ev.MatchID,
		Players:   apiPlayers,
	}
	payload, err = json.Marshal(wire)
	if err != nil {
		return "", nil, fmt.Errorf("marshal MatchMadeEvent: %w", err)
	}
	return apimatchmaking.EventTypeMatchMade, payload, nil
}
