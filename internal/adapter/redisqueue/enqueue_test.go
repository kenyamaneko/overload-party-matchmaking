//go:build integration

package redisqueue_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
)

func TestEnqueue(t *testing.T) {
	t.Run("プレイヤー登録", func(t *testing.T) {
		t.Run("プレイヤーIDが空文字の状態で登録すると、errorになる", func(t *testing.T) {
			q := newQueue(t)
			_, err := q.Enqueue(context.Background(), "", 1, "Alice", 1, "gw-1")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "playerID")
		})

		t.Run("gatewayインスタンス識別子が空文字の状態で登録すると、errorになる", func(t *testing.T) {
			q := newQueue(t)
			_, err := q.Enqueue(context.Background(), "alice", 1, "Alice", 1, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "gatewayInstanceID")
		})

		t.Run("未登録のプレイヤーを登録すると、キューサイズが1増える", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			before, err := q.Size(ctx)
			require.NoError(t, err)

			_, err = q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)

			after, err := q.Size(ctx)
			require.NoError(t, err)
			assert.Equal(t, before+1, after)
		})

		t.Run("同じプレイヤーがgatewayインスタンス識別子を変えずに2回登録すると、キューサイズは1のままになる", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)

			_, err = q.Enqueue(ctx, "alice", 99, "AliceV2", 9, "gw-1")
			require.NoError(t, err)

			size, err := q.Size(ctx)
			require.NoError(t, err)
			assert.Equal(t, int64(1), size)
		})

		t.Run("同じプレイヤーがgatewayインスタンス識別子を変えずに2回登録すると、ペアを取り出したときに読み出せる内容は2回目の登録内容(デッキID・名前・レベル)になる", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "alice", 99, "AliceV2", 9, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "bob", 2, "Bob", 2, "gw-1")
			require.NoError(t, err)

			pair, _, err := q.PopPair(ctx)
			require.NoError(t, err)
			require.Len(t, pair, 2)

			alice := findByPlayerID(t, pair, "alice")
			assert.Equal(t, int64(99), alice.DeckID)
			assert.Equal(t, "AliceV2", alice.Name)
			assert.Equal(t, int64(9), alice.Level)
		})

		t.Run("名前に:を含むプレイヤーを登録してペアを取り出すと、名前がそのまま返る", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Ali:ce", 1, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "bob", 2, "Bob", 2, "gw-1")
			require.NoError(t, err)

			pair, _, err := q.PopPair(ctx)
			require.NoError(t, err)
			require.Len(t, pair, 2)

			alice := findByPlayerID(t, pair, "alice")
			assert.Equal(t, "Ali:ce", alice.Name)
		})

		t.Run("直前の登録と異なるgatewayインスタンス識別子で登録すると、それまで待機していた全員がマッチメイキングキューから消える", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "bob", 2, "Bob", 2, "gw-1")
			require.NoError(t, err)

			_, err = q.Enqueue(ctx, "carol", 3, "Carol", 3, "gw-2")
			require.NoError(t, err)

			size, err := q.Size(ctx)
			require.NoError(t, err)
			assert.Equal(t, int64(1), size)
		})

		t.Run("直前の登録と異なるgatewayインスタンス識別子で登録すると、削除件数が戻り値に入る", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "bob", 2, "Bob", 2, "gw-1")
			require.NoError(t, err)

			removed, err := q.Enqueue(ctx, "carol", 3, "Carol", 3, "gw-2")
			require.NoError(t, err)
			assert.Equal(t, int64(2), removed)
		})

		t.Run("gatewayインスタンス識別子を一度も登録していない状態で最初の登録をすると、削除件数は0が返る", func(t *testing.T) {
			q := newQueue(t)
			removed, err := q.Enqueue(context.Background(), "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			assert.Equal(t, int64(0), removed)
		})

		t.Run("プレイヤーIDが空文字でエラーになったあと、有効なプレイヤーIDで登録すると成功し、キューサイズが1増える", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "", 1, "Alice", 1, "gw-1")
			require.Error(t, err)

			before, err := q.Size(ctx)
			require.NoError(t, err)

			_, err = q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)

			after, err := q.Size(ctx)
			require.NoError(t, err)
			assert.Equal(t, before+1, after)
		})
	})
}

func findByPlayerID(t *testing.T, entries []domain.QueueEntry, playerID string) domain.QueueEntry {
	t.Helper()
	for _, e := range entries {
		if e.PlayerID == playerID {
			return e
		}
	}
	t.Fatalf("player %q not found in %v", playerID, entries)
	return domain.QueueEntry{}
}
