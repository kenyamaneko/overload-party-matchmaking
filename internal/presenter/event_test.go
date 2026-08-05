package presenter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/presenter"
)

func TestToMatchMadeWire(t *testing.T) {
	t.Run("マッチ成立イベントの変換", func(t *testing.T) {
		cases := []struct {
			name    string
			input   domain.MatchMadeEvent
			wantErr string
		}{
			{
				name: "マッチIDが空のとき、エラーになり変換結果を返さない",
				input: domain.MatchMadeEvent{
					MatchID: "",
					Players: []domain.MatchedPlayer{{PlayerID: "p1", DeckID: 1, Name: "alice", Level: 1}},
				},
				wantErr: "MatchID is empty",
			},
			{
				name: "プレイヤーがnilのとき、エラーになり変換結果を返さない",
				input: domain.MatchMadeEvent{
					MatchID: "mch_x",
					Players: nil,
				},
				wantErr: "Players is empty",
			},
			{
				name: "プレイヤーがnilでなく要素0の空列のとき、エラーになり変換結果を返さない",
				input: domain.MatchMadeEvent{
					MatchID: "mch_x",
					Players: []domain.MatchedPlayer{},
				},
				wantErr: "Players is empty",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				eventType, payload, err := presenter.ToMatchMadeWire(tc.input)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Empty(t, eventType)
				assert.Empty(t, payload)
			})
		}
	})
}
