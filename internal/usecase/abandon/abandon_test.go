package abandon_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/abandon"
)

func TestAbandon(t *testing.T) {
	t.Run("不成立申告時の破棄", func(t *testing.T) {
		t.Run("申告されたプレイヤーID全員の取り消しに成功する状態で破棄を実行すると、戻り値はnilになる", func(t *testing.T) {
			q := newFakeQueue("player-1", "player-2")
			a := abandon.New(q)

			err := a.Abandon(context.Background(), "mch_1", []string{"player-1", "player-2"}, "player_not_connected")

			assert.NoError(t, err)
		})

		t.Run("申告されたプレイヤーIDのいずれかの取り消しが失敗する状態で破棄を実行すると、errorが返る", func(t *testing.T) {
			q := newFakeQueue("player-1", "player-2")
			q.cancelErrByPlayerID["player-1"] = errors.New("cancel failed")
			a := abandon.New(q)

			err := a.Abandon(context.Background(), "mch_1", []string{"player-1", "player-2"}, "player_not_connected")

			assert.Error(t, err)
		})

		t.Run("申告されたプレイヤーIDが複数あり先頭のプレイヤーIDの取り消しが失敗する状態で破棄を実行すると、マッチメイキングキューには後続のプレイヤーIDがまだ残っている", func(t *testing.T) {
			q := newFakeQueue("player-1", "player-2")
			q.cancelErrByPlayerID["player-1"] = errors.New("cancel failed")
			a := abandon.New(q)

			_ = a.Abandon(context.Background(), "mch_1", []string{"player-1", "player-2"}, "player_not_connected")

			assert.True(t, q.contains("player-2"))
		})
	})
}
