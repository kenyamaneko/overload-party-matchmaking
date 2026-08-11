package presenter_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/presenter"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

func TestToMatchMadeWire(t *testing.T) {
	t.Run("マッチ成立イベントの変換", func(t *testing.T) {
		newEvent := func() domain.MatchMadeEvent {
			return domain.MatchMadeEvent{
				MatchID: "mch_test1",
				Players: []domain.MatchedPlayer{
					{PlayerID: "player-1", DeckID: 11, Name: "Alice", Level: 3},
					{PlayerID: "player-2", DeckID: 22, Name: "Bob", Level: 5},
				},
			}
		}

		t.Run("マッチIDが設定され、プレイヤーが2人いるマッチ成立イベントを変換すると、変換結果のイベント種別はmatch_madeになる", func(t *testing.T) {
			eventType, _, err := presenter.ToMatchMadeWire(newEvent())
			require.NoError(t, err)
			assert.Equal(t, apimatchmaking.EventTypeMatchMade, eventType)
		})

		t.Run("マッチIDが設定され、プレイヤーが2人いるマッチ成立イベントを変換すると、変換結果のpayloadにはマッチIDと各プレイヤーのプレイヤーID・デッキID・名前・レベルが含まれる", func(t *testing.T) {
			_, payload, err := presenter.ToMatchMadeWire(newEvent())
			require.NoError(t, err)

			var wire apimatchmaking.MatchMadeEvent
			require.NoError(t, json.Unmarshal(payload, &wire))

			assert.Equal(t, "mch_test1", wire.MatchID)
			assert.Equal(t, []apimatchmaking.MatchedPlayer{
				{PlayerID: "player-1", DeckID: 11, Name: "Alice", Level: 3},
				{PlayerID: "player-2", DeckID: 22, Name: "Bob", Level: 5},
			}, wire.Players)
		})
	})
}
