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

// TestToMatchMadeWire は domain event を wire payload に変換した結果が
// asyncapi.yaml の仕様 (event_type, match_id, players) と一致することを検証する。
func TestToMatchMadeWire(t *testing.T) {
	tests := []struct {
		name    string
		input   domain.MatchMadeEvent
		wantErr string
	}{
		{
			name: "正常系",
			input: domain.MatchMadeEvent{
				MatchID: "mch_01H",
				Players: []domain.MatchedPlayer{
					{PlayerID: "p1", DeckID: 11},
					{PlayerID: "p2", DeckID: 22},
				},
			},
			wantErr: "",
		},
		{
			name: "MatchID 空",
			input: domain.MatchMadeEvent{
				MatchID: "",
				Players: []domain.MatchedPlayer{{PlayerID: "p1", DeckID: 1}},
			},
			wantErr: "MatchID is empty",
		},
		{
			name: "Players 空",
			input: domain.MatchMadeEvent{
				MatchID: "mch_x",
				Players: nil,
			},
			wantErr: "Players is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventType, payload, err := presenter.ToMatchMadeWire(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Empty(t, eventType)
				assert.Empty(t, payload)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, apimatchmaking.EventTypeMatchMade, eventType)

			var wire apimatchmaking.MatchMadeEvent
			require.NoError(t, json.Unmarshal(payload, &wire))
			assert.Equal(t, apimatchmaking.EventTypeMatchMade, wire.EventType)
			assert.Equal(t, tt.input.MatchID, wire.MatchID)
			require.Len(t, wire.Players, len(tt.input.Players))
			for i, p := range tt.input.Players {
				assert.Equal(t, p.PlayerID, wire.Players[i].PlayerID)
				assert.Equal(t, p.DeckID, wire.Players[i].DeckID)
			}
		})
	}
}
