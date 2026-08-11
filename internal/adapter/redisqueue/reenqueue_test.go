//go:build integration

package redisqueue_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
)

func TestReenqueue(t *testing.T) {
	t.Run("書き戻し", func(t *testing.T) {
		t.Run("空のエントリ一覧を渡して書き戻すと、trueが返る", func(t *testing.T) {
			q := newQueue(t)
			ok, err := q.Reenqueue(context.Background(), []domain.QueueEntry{}, "gw-1")
			require.NoError(t, err)
			assert.True(t, ok)
		})

		t.Run("空のエントリ一覧を渡して書き戻しても、マッチメイキングキューの中身は変化しない", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			before, err := q.Size(ctx)
			require.NoError(t, err)

			_, err = q.Reenqueue(ctx, []domain.QueueEntry{}, "gw-1")
			require.NoError(t, err)

			after, err := q.Size(ctx)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})

		t.Run("ペア取り出しで取り出した時点のgatewayインスタンス識別子と現在の保持値が一致する状態で書き戻すと、trueが返る", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "bob", 2, "Bob", 2, "gw-1")
			require.NoError(t, err)
			pair, gatewayInstanceID, err := q.PopPair(ctx)
			require.NoError(t, err)

			ok, err := q.Reenqueue(ctx, pair, gatewayInstanceID)
			require.NoError(t, err)
			assert.True(t, ok)
		})

		t.Run("ペア取り出しで取り出した時点のgatewayインスタンス識別子と現在の保持値が一致する状態で書き戻すと、エントリが元のFIFO順序を保ったままマッチメイキングキューに戻る", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "bob", 2, "Bob", 2, "gw-1")
			require.NoError(t, err)
			pair, gatewayInstanceID, err := q.PopPair(ctx)
			require.NoError(t, err)

			_, err = q.Reenqueue(ctx, pair, gatewayInstanceID)
			require.NoError(t, err)

			size, err := q.Size(ctx)
			require.NoError(t, err)
			assert.Equal(t, int64(2), size)
		})

		t.Run("書き戻したペアを再度ペアを取り出すと、同じ2人が同じ順序で取り出される", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "bob", 2, "Bob", 2, "gw-1")
			require.NoError(t, err)
			pair, gatewayInstanceID, err := q.PopPair(ctx)
			require.NoError(t, err)
			_, err = q.Reenqueue(ctx, pair, gatewayInstanceID)
			require.NoError(t, err)

			gotPair, _, err := q.PopPair(ctx)
			require.NoError(t, err)
			require.Len(t, gotPair, 2)
			assert.Equal(t, "alice", gotPair[0].PlayerID)
			assert.Equal(t, "bob", gotPair[1].PlayerID)
		})

		t.Run("ペア取り出しで取り出した時点のgatewayインスタンス識別子が現在の保持値と一致しない状態で書き戻すと、falseが返る", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "bob", 2, "Bob", 2, "gw-1")
			require.NoError(t, err)
			pair, gatewayInstanceID, err := q.PopPair(ctx)
			require.NoError(t, err)

			_, err = q.Enqueue(ctx, "carol", 3, "Carol", 3, "gw-2")
			require.NoError(t, err)

			ok, err := q.Reenqueue(ctx, pair, gatewayInstanceID)
			require.NoError(t, err)
			assert.False(t, ok)
		})

		t.Run("ペア取り出しで取り出した時点のgatewayインスタンス識別子が現在の保持値と一致しない状態で書き戻すと、エントリはマッチメイキングキューに戻らない", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "bob", 2, "Bob", 2, "gw-1")
			require.NoError(t, err)
			pair, gatewayInstanceID, err := q.PopPair(ctx)
			require.NoError(t, err)

			_, err = q.Enqueue(ctx, "carol", 3, "Carol", 3, "gw-2")
			require.NoError(t, err)

			_, err = q.Reenqueue(ctx, pair, gatewayInstanceID)
			require.NoError(t, err)

			size, err := q.Size(ctx)
			require.NoError(t, err)
			assert.Equal(t, int64(1), size)
		})
	})
}
