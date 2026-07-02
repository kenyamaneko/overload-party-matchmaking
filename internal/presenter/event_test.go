package presenter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/presenter"
)

// TestToMatchMadeWireRejectsInvalidEvent は不正な domain event (MatchID 空 / Players 空) を
// ToMatchMadeWire がエラーとして拒否し、wire payload を返さないことを検証する。
func TestToMatchMadeWireRejectsInvalidEvent(t *testing.T) {
	tests := []struct {
		name    string
		input   domain.MatchMadeEvent
		wantErr string
	}{
		{
			name: "MatchID 空",
			input: domain.MatchMadeEvent{
				MatchID: "",
				Players: []domain.MatchedPlayer{{PlayerID: "p1", DeckID: 1, Name: "alice", Level: 1}},
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
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, eventType)
			assert.Empty(t, payload)
		})
	}
}
