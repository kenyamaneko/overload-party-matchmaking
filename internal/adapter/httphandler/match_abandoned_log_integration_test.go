//go:build integration

package httphandler_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler/httphandlertest"
)

func TestMatchAbandonedLog(t *testing.T) {
	t.Run("[マッチ不成立]不成立の申告のログ記録", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("TST-PLAYER-1とTST-PLAYER-2を不成立として申告したとき", func(t *testing.T) {
				abandonedLogAttrs := func(t *testing.T) map[string]string {
					logs := CaptureLogs(t)
					router := NewServerWithRealQueue(t)

					SendMatchAbandoned(router, AbandonBody(PlayerIDs(PlayerID1, PlayerID2)))

					records := logs.RecordsWithMessage("match abandoned")
					require.Len(t, records, 1)
					return records[0].Attrs
				}

				t.Run("マッチの破棄のログのmatch_idが、申告したmch_TST0001になる", func(t *testing.T) {
					attrs := abandonedLogAttrs(t)

					assert.Equal(t, "mch_TST0001", attrs["match_id"])
				})

				t.Run("マッチの破棄のログのreasonが、申告したplayer_not_connectedになる", func(t *testing.T) {
					attrs := abandonedLogAttrs(t)

					assert.Equal(t, "player_not_connected", attrs["reason"])
				})
			})
		})
	})
}
