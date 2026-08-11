//go:build integration

package redisqueue_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPopPair(t *testing.T) {
	t.Run("ペア取り出し", func(t *testing.T) {
		t.Run("マッチメイキングキューが空の状態でペアを取り出すと、空のペアが返る", func(t *testing.T) {
			q := newQueue(t)
			pair, _, err := q.PopPair(context.Background())
			require.NoError(t, err)
			assert.Empty(t, pair)
		})

		t.Run("マッチメイキングキューが空の状態でペアを取り出しても、キューサイズは変化しない", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			before, err := q.Size(ctx)
			require.NoError(t, err)

			_, _, err = q.PopPair(ctx)
			require.NoError(t, err)

			after, err := q.Size(ctx)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})

		t.Run("マッチメイキングキューに1人いる状態でペアを取り出すと、空のペアが返る", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)

			pair, _, err := q.PopPair(ctx)
			require.NoError(t, err)
			assert.Empty(t, pair)
		})

		t.Run("マッチメイキングキューに1人いる状態でペアを取り出しても、キューサイズは変化しない", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			before, err := q.Size(ctx)
			require.NoError(t, err)

			_, _, err = q.PopPair(ctx)
			require.NoError(t, err)

			after, err := q.Size(ctx)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})

		t.Run("マッチメイキングキューに2人以上いる状態でペアを取り出すと、先に登録した2人が取り出される", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "bob", 2, "Bob", 2, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "carol", 3, "Carol", 3, "gw-1")
			require.NoError(t, err)

			pair, _, err := q.PopPair(ctx)
			require.NoError(t, err)
			require.Len(t, pair, 2)
			assert.Equal(t, "alice", pair[0].PlayerID)
			assert.Equal(t, "bob", pair[1].PlayerID)
		})

		t.Run("マッチメイキングキューに2人以上いる状態でペアを取り出すと、取り出された2人はマッチメイキングキューから取り除かれる", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "bob", 2, "Bob", 2, "gw-1")
			require.NoError(t, err)

			_, _, err = q.PopPair(ctx)
			require.NoError(t, err)

			size, err := q.Size(ctx)
			require.NoError(t, err)
			assert.Equal(t, int64(0), size)
		})

		t.Run("マッチメイキングキューに2人以上いる状態でペアを取り出すと、取り出した時点で保持していたgatewayインスタンス識別子が結果に添えられる", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "bob", 2, "Bob", 2, "gw-1")
			require.NoError(t, err)

			_, gatewayInstanceID, err := q.PopPair(ctx)
			require.NoError(t, err)
			assert.Equal(t, "gw-1", gatewayInstanceID)
		})
	})
}
