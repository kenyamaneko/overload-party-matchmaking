//go:build integration

package redisqueue_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCancel(t *testing.T) {
	t.Run("取消", func(t *testing.T) {
		t.Run("プレイヤーIDが空文字の状態で取消すと、errorになる", func(t *testing.T) {
			q := newQueue(t)
			_, err := q.Cancel(context.Background(), "")
			assert.Error(t, err)
		})

		t.Run("マッチメイキングキューにいるプレイヤーを取消すと、取り消し済みとしてtrueが返る", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)

			removed, err := q.Cancel(ctx, "alice")
			require.NoError(t, err)
			assert.True(t, removed)
		})

		t.Run("マッチメイキングキューにいるプレイヤーを取消すと、キューサイズが1減る", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "alice", 1, "Alice", 1, "gw-1")
			require.NoError(t, err)
			before, err := q.Size(ctx)
			require.NoError(t, err)

			_, err = q.Cancel(ctx, "alice")
			require.NoError(t, err)

			after, err := q.Size(ctx)
			require.NoError(t, err)
			assert.Equal(t, before-1, after)
		})

		t.Run("マッチメイキングキューにいないプレイヤーを取消すと、falseが返る", func(t *testing.T) {
			q := newQueue(t)
			removed, err := q.Cancel(context.Background(), "alice")
			require.NoError(t, err)
			assert.False(t, removed)
		})

		t.Run("マッチメイキングキューにいないプレイヤーを取消しても、キューサイズは変わらない", func(t *testing.T) {
			q := newQueue(t)
			ctx := context.Background()
			before, err := q.Size(ctx)
			require.NoError(t, err)

			_, err = q.Cancel(ctx, "alice")
			require.NoError(t, err)

			after, err := q.Size(ctx)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	})
}
