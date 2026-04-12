package redisqueue

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestQueue(t *testing.T) (*RedisQueue, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisQueue(client), mr
}

func TestEnqueueThenSize(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10))
	require.NoError(t, q.Enqueue(ctx, "p2", 20))

	n, err := q.Size(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), n)
}

func TestEnqueueIdempotentOnDeckUpdate(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10))
	require.NoError(t, q.Enqueue(ctx, "p1", 77))

	n, err := q.Size(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "re-enqueue must replace, not stack")

	entries, err := q.PopPair(ctx)
	require.NoError(t, err)
	require.Empty(t, entries, "single entry cannot form a pair")
}

func TestPopPairFIFO(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p2", 20))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p3", 30))

	pair, err := q.PopPair(ctx)
	require.NoError(t, err)
	require.Len(t, pair, 2)
	require.Equal(t, "p1", pair[0].PlayerID)
	require.Equal(t, int64(10), pair[0].DeckID)
	require.Equal(t, "p2", pair[1].PlayerID)
	require.Equal(t, int64(20), pair[1].DeckID)

	n, err := q.Size(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
}

func TestPopPairRequiresTwo(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10))
	pair, err := q.PopPair(ctx)
	require.NoError(t, err)
	require.Empty(t, pair, "single entry cannot form a pair")

	n, err := q.Size(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "non-popped entry must remain")
}

func TestCancelRemovesExactly(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10))
	require.NoError(t, q.Enqueue(ctx, "p2", 20))

	removed, err := q.Cancel(ctx, "p1")
	require.NoError(t, err)
	require.True(t, removed)

	n, err := q.Size(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)

	removed, err = q.Cancel(ctx, "p_unknown")
	require.NoError(t, err)
	require.False(t, removed)
}

func TestReenqueuePreservesOrder(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p2", 20))

	pair, err := q.PopPair(ctx)
	require.NoError(t, err)
	require.Len(t, pair, 2)

	require.NoError(t, q.Reenqueue(ctx, pair))

	again, err := q.PopPair(ctx)
	require.NoError(t, err)
	require.Len(t, again, 2)
	require.Equal(t, "p1", again[0].PlayerID)
	require.Equal(t, "p2", again[1].PlayerID)
}
