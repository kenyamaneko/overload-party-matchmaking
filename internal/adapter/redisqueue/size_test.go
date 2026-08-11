//go:build integration

package redisqueue_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSize(t *testing.T) {
	t.Run("キューサイズ取得", func(t *testing.T) {
		t.Run("マッチメイキングキューが空の状態でキューサイズを取得すると、0が返る", func(t *testing.T) {
			q := newQueue(t)
			size, err := q.Size(context.Background())
			require.NoError(t, err)
			assert.Equal(t, int64(0), size)
		})

		t.Run("マッチメイキングキューに1人いる状態でキューサイズを取得すると、1が返る", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)

			size, err := q.Size(ctx)
			require.NoError(t, err)
			assert.Equal(t, int64(1), size)
		})

		t.Run("マッチメイキングキューに2人いる状態でキューサイズを取得すると、2が返る", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "bob", 2, "Bob", 2, "gw-1")
			require.NoError(t, err)

			size, err := q.Size(ctx)
			require.NoError(t, err)
			assert.Equal(t, int64(2), size)
		})
	})
}
