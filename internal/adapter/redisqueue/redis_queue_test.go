package redisqueue

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// testRedisURL は docker-compose の Valkey を指す。
// DB 1 を使うのは run-local (.env.local) の DB 0 と分離するため (テストは毎回 FLUSHDB する)。
const testRedisURL = "redis://localhost:6379/1"

func newTestQueue(t *testing.T) *RedisQueue {
	t.Helper()
	opt, err := redis.ParseURL(testRedisURL)
	require.NoError(t, err)
	client := redis.NewClient(opt)
	ctx := context.Background()
	require.NoError(t, client.Ping(ctx).Err())
	require.NoError(t, client.FlushDB(ctx).Err())
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisQueue(client)
}

// TestEnqueueThenSize は複数 Enqueue 後に Size が正しいメンバー数を返すことを検証する。
func TestEnqueueThenSize(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))
	require.NoError(t, q.Enqueue(ctx, "p2", 20, "p2-name", 1))

	n, err := q.Size(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), n)
}

// TestEnqueueIdempotentOnDeckUpdate は同一 playerID で deckID を変えて再 Enqueue したとき、
// 1 件に置き換わることを検証する (同一 playerID 重複禁止)。
func TestEnqueueIdempotentOnDeckUpdate(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))
	require.NoError(t, q.Enqueue(ctx, "p1", 77, "p1-name", 1))

	n, err := q.Size(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "re-enqueue must replace, not stack")

	entries, err := q.PopPair(ctx)
	require.NoError(t, err)
	require.Empty(t, entries, "single entry cannot form a pair")
}

// TestEnqueueSameUserSameDeck は同一 (playerID, deckID) の再 Enqueue でも件数が 1 のままであることを検証する。
func TestEnqueueSameUserSameDeck(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))
	require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))

	n, err := q.Size(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "同一 (playerID, deckID) でもスタックしない")
}

// TestEnqueueSameUserUpdatesPosition は p1 → p2 → p1 の順で Enqueue したとき、最後の p1 が
// 末尾に移動することを検証する (再 Enqueue で FIFO 位置がリセットされる)。
func TestEnqueueSameUserUpdatesPosition(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p2", 20, "p2-name", 1))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p1", 11, "p1-name", 1)) // p1 を末尾に

	pair, err := q.PopPair(ctx)
	require.NoError(t, err)
	require.Len(t, pair, 2)
	require.Equal(t, "p2", pair[0].PlayerID, "再 enqueue した p1 は p2 より後ろに来る")
	require.Equal(t, "p1", pair[1].PlayerID)
	require.Equal(t, int64(11), pair[1].DeckID, "deckID も最新の値に置き換わる")
}

// TestPopPairFIFO は PopPair が joinedAt 順に先頭 2 件を pop し、3 人目が残ることを検証する。
func TestPopPairFIFO(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p2", 20, "p2-name", 1))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p3", 30, "p3-name", 1))

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

// TestPopPairRequiresTwo はキューに 1 名しかいないとき、PopPair が空で返りエントリが残ることを検証する。
func TestPopPairRequiresTwo(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))
	pair, err := q.PopPair(ctx)
	require.NoError(t, err)
	require.Empty(t, pair, "single entry cannot form a pair")

	n, err := q.Size(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "non-popped entry must remain")
}

// TestPopPairFromEmptyQueue は空キューに対する PopPair が空スライス + no error を返すことを検証する。
func TestPopPairFromEmptyQueue(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	pair, err := q.PopPair(ctx)
	require.NoError(t, err)
	require.Empty(t, pair)
}

// TestCancelRemovesExactly は Cancel が指定 playerID のエントリだけ削除し、
// 存在しない playerID では removed=false を返すことを検証する。
func TestCancelRemovesExactly(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))
	require.NoError(t, q.Enqueue(ctx, "p2", 20, "p2-name", 1))

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

// TestCancelIsIdempotent は同一 playerID に対する 2 回目の Cancel が false + no error を返すことを検証する。
func TestCancelIsIdempotent(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))

	removed, err := q.Cancel(ctx, "p1")
	require.NoError(t, err)
	require.True(t, removed)

	removed, err = q.Cancel(ctx, "p1")
	require.NoError(t, err)
	require.False(t, removed, "2 回目の Cancel は冪等に false を返す")
}

// TestCancelThenReenqueue は Cancel 後の同 playerID を再度 Enqueue してマッチングに乗れることを検証する
// (ゴーストエントリが残らないこと)。
func TestCancelThenReenqueue(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))
	removed, err := q.Cancel(ctx, "p1")
	require.NoError(t, err)
	require.True(t, removed)

	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p1", 99, "p1-name", 1)) // 復帰
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p2", 20, "p2-name", 1))

	pair, err := q.PopPair(ctx)
	require.NoError(t, err)
	require.Len(t, pair, 2)
	require.Equal(t, "p1", pair[0].PlayerID, "復帰した p1 がマッチに乗る")
	require.Equal(t, int64(99), pair[0].DeckID)
	require.Equal(t, "p2", pair[1].PlayerID)
}

// TestReenqueuePreservesOrder は pop したペアを Reenqueue したとき、元の score で戻されて
// 再 pop 時も同じ順序で取れることを検証する (publish 失敗時の FIFO 保持)。
func TestReenqueuePreservesOrder(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p2", 20, "p2-name", 1))

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

// TestPopPairAfterCancelPairsLaterEntrants は p1 が enqueue 後すぐ Cancel で抜け、
// 後から来た p2・p3 が正しくペアリングされることを検証する (Cancel がゴースト残留を起こさない)。
func TestPopPairAfterCancelPairsLaterEntrants(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))
	time.Sleep(2 * time.Millisecond)

	removed, err := q.Cancel(ctx, "p1")
	require.NoError(t, err)
	require.True(t, removed)

	n, err := q.Size(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), n, "Cancel 後はキューが空になっていること")

	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p2", 20, "p2-name", 1))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p3", 30, "p3-name", 1))

	pair, err := q.PopPair(ctx)
	require.NoError(t, err)
	require.Len(t, pair, 2)
	require.Equal(t, "p2", pair[0].PlayerID, "Cancel した p1 が混入せず、p2 が先頭になる")
	require.Equal(t, int64(20), pair[0].DeckID)
	require.Equal(t, "p3", pair[1].PlayerID)
	require.Equal(t, int64(30), pair[1].DeckID)

	n, err = q.Size(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), n, "ペア成立後はキューが空")
}

// TestEnqueuePreservesPlayerSummary は enqueue で渡した name / level が PopPair の戻り値で
// そのまま取得できることを検証する (queue entry に snapshot を保持する仕様の担保)。
// name には区切り文字 `:` を含むケースも検証し、encoding が壊れないことを確認する。
func TestEnqueuePreservesPlayerSummary(t *testing.T) {
	cases := []struct {
		name      string
		playerID  string
		deckID    int64
		inputName string
		level     int64
	}{
		{
			name:      "通常の name",
			playerID:  "p1",
			deckID:    10,
			inputName: "alice",
			level:     7,
		},
		{
			name:      "区切り文字 `:` を含む name",
			playerID:  "p2",
			deckID:    20,
			inputName: "name:with:colons",
			level:     12,
		},
		{
			name:      "空 name (matchmaking は account に依存せず信頼する仕様の前提)",
			playerID:  "p3",
			deckID:    30,
			inputName: "",
			level:     0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := newTestQueue(t)
			ctx := context.Background()

			require.NoError(t, q.Enqueue(ctx, tc.playerID, tc.deckID, tc.inputName, tc.level))
			time.Sleep(2 * time.Millisecond)
			require.NoError(t, q.Enqueue(ctx, "other", 99, "other", 1))

			pair, err := q.PopPair(ctx)
			require.NoError(t, err)
			require.Len(t, pair, 2)
			require.Equal(t, tc.playerID, pair[0].PlayerID)
			require.Equal(t, tc.deckID, pair[0].DeckID)
			require.Equal(t, tc.inputName, pair[0].Name)
			require.Equal(t, tc.level, pair[0].Level)
		})
	}
}

// TestPopPairAcrossMultipleRounds は複数ラウンドにわたる Enqueue → PopPair の連続で、
// FIFO 順序が保たれたまま次々とペアが pop されることを検証する。
func TestPopPairAcrossMultipleRounds(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p2", 20, "p2-name", 1))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p3", 30, "p3-name", 1))

	pair1, err := q.PopPair(ctx)
	require.NoError(t, err)
	require.Len(t, pair1, 2)
	require.Equal(t, "p1", pair1[0].PlayerID)
	require.Equal(t, "p2", pair1[1].PlayerID)

	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p4", 40, "p4-name", 1))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, "p5", 50, "p5-name", 1))

	pair2, err := q.PopPair(ctx)
	require.NoError(t, err)
	require.Len(t, pair2, 2)
	require.Equal(t, "p3", pair2[0].PlayerID, "p3 は 1 ラウンド目の余りで 2 ラウンド目の先頭")
	require.Equal(t, "p4", pair2[1].PlayerID)

	n, err := q.Size(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "p5 のみ残る")
}
