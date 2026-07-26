//go:build integration

package redisqueue

import (
	"context"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/valkeytest"
)

// testRedisURL は TestMain が起動した Valkey container の接続 URL。
var testRedisURL string

// TestMain はパッケージ内の結合テスト全体で共有する Valkey container を起動する。
func TestMain(m *testing.M) {
	ctx := context.Background()
	// 各テストは newTestQueue の FLUSHDB で分離するため、container は 1 つを共有すれば足りる。
	vk, err := valkeytest.New(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "valkeytest start: %v\n", err)
		os.Exit(1)
	}
	testRedisURL = vk.URL()
	code := m.Run()
	if err := vk.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "valkeytest terminate: %v\n", err)
	}
	os.Exit(code)
}

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

func TestEnqueue(t *testing.T) {
	t.Run("キューへの追加", func(t *testing.T) {
		t.Run("2 名を追加したとき、待機人数が 2 になる", func(t *testing.T) {
			q := newTestQueue(t)
			ctx := context.Background()

			require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))
			require.NoError(t, q.Enqueue(ctx, "p2", 20, "p2-name", 1))

			n, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(2), n)
		})

		t.Run("同一プレイヤーがデッキを変えて再追加したとき、1 件に置き換わる", func(t *testing.T) {
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
		})

		t.Run("同一プレイヤーが同一デッキで再追加したとき、待機人数は 1 のまま", func(t *testing.T) {
			q := newTestQueue(t)
			ctx := context.Background()

			require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))
			require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))

			n, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(1), n, "同一 (playerID, deckID) でもスタックしない")
		})

		t.Run("p1 → p2 → p1 の順で追加したとき、再追加した p1 が待機列の末尾へ移動しデッキも最新になる", func(t *testing.T) {
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
		})

		// name / level は enqueue 時の snapshot として queue entry に保持され、PopPair でそのまま復元される。
		// matchmaking は account を呼ばず信頼するため、空 name も含めて保持する。
		summaryCases := []struct {
			name      string
			playerID  string
			deckID    int64
			inputName string
			level     int64
		}{
			{
				name:      "通常の名前のとき、名前とレベルが復元される",
				playerID:  "p1",
				deckID:    10,
				inputName: "alice",
				level:     7,
			},
			{
				name:      "区切り文字 : を含む名前のとき、壊れず復元される",
				playerID:  "p2",
				deckID:    20,
				inputName: "name:with:colons",
				level:     12,
			},
			{
				name:      "名前が空のとき、空のまま復元される",
				playerID:  "p3",
				deckID:    30,
				inputName: "",
				level:     0,
			},
			{
				name:      "deckID と level が int64 の最大値のとき、そのまま復元される",
				playerID:  "p4",
				deckID:    math.MaxInt64,
				inputName: "p4-name",
				level:     math.MaxInt64,
			},
			{
				name:      "level が負値 -1 のとき、そのまま復元される",
				playerID:  "p5",
				deckID:    40,
				inputName: "p5-name",
				level:     -1,
			},
		}
		for _, tc := range summaryCases {
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
	})
}

func TestPopPair(t *testing.T) {
	t.Run("先頭2件の取り出し", func(t *testing.T) {
		t.Run("3 名が待機するとき、参加時刻順に先頭 2 件を取り出し 3 人目が残る", func(t *testing.T) {
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
		})

		t.Run("1 名しかいないとき、ペアにならず待機に残る", func(t *testing.T) {
			q := newTestQueue(t)
			ctx := context.Background()

			require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))
			pair, err := q.PopPair(ctx)
			require.NoError(t, err)
			require.Empty(t, pair, "single entry cannot form a pair")

			n, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(1), n, "non-popped entry must remain")
		})

		t.Run("キューが空のとき、ペアは取れずエラーにならない", func(t *testing.T) {
			q := newTestQueue(t)
			ctx := context.Background()

			pair, err := q.PopPair(ctx)
			require.NoError(t, err)
			require.Empty(t, pair)
		})

		t.Run("p1 が追加後すぐキャンセルで抜けるとき、後続の p2・p3 が正しくペアになる", func(t *testing.T) {
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
		})

		t.Run("複数ラウンドで連続して取り出すとき、待機順が保たれ余りが次ラウンド先頭になる", func(t *testing.T) {
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
		})
	})
}

func TestCancel(t *testing.T) {
	t.Run("キャンセル", func(t *testing.T) {
		t.Run("指定したプレイヤーだけ削除し、存在しないプレイヤーは削除されない", func(t *testing.T) {
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
		})

		t.Run("同じプレイヤーを 2 回キャンセルしたとき、2 回目は何も削除されない", func(t *testing.T) {
			q := newTestQueue(t)
			ctx := context.Background()

			require.NoError(t, q.Enqueue(ctx, "p1", 10, "p1-name", 1))

			removed, err := q.Cancel(ctx, "p1")
			require.NoError(t, err)
			require.True(t, removed)

			removed, err = q.Cancel(ctx, "p1")
			require.NoError(t, err)
			require.False(t, removed, "2 回目の Cancel は冪等に false を返す")
		})

		t.Run("キャンセル後に同じプレイヤーを再追加したとき、ゴーストが残らずマッチに乗る", func(t *testing.T) {
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
		})
	})
}

func TestReenqueue(t *testing.T) {
	// Reenqueue は publish 失敗時に pop 済みペアを戻す経路で、元の FIFO 順序を保つ必要がある。
	t.Run("キューへの戻し", func(t *testing.T) {
		t.Run("取り出したペアを戻したとき、元の順序で戻され再取り出し時も同じ順序で取れる", func(t *testing.T) {
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
		})

		t.Run("戻す対象が無い (nil) とき、エラーなくキューは変わらない", func(t *testing.T) {
			q := newTestQueue(t)
			ctx := context.Background()

			require.NoError(t, q.Reenqueue(ctx, nil))

			n, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(0), n)
		})

		t.Run("戻す対象が空列のとき、エラーなくキューは変わらない", func(t *testing.T) {
			q := newTestQueue(t)
			ctx := context.Background()

			require.NoError(t, q.Reenqueue(ctx, []domain.QueueEntry{}))

			n, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(0), n)
		})
	})
}
