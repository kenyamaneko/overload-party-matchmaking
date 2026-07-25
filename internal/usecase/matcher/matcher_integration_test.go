//go:build integration

package matcher

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/redisqueue"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/valkeytest"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
	"github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking/apimatchmakingfake"
)

// testRedisURL は TestMain が起動した Valkey container の接続 URL。
var testRedisURL string

// TestMain はパッケージ内の結合テスト全体で共有する Valkey container を起動する。
func TestMain(m *testing.M) {
	ctx := context.Background()
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

// newRealQueue は DB をクリアした実 Valkey 接続上の RedisQueue を返す。
func newRealQueue(t *testing.T) *redisqueue.RedisQueue {
	t.Helper()
	opt, err := redis.ParseURL(testRedisURL)
	require.NoError(t, err)
	client := redis.NewClient(opt)
	ctx := context.Background()
	require.NoError(t, client.Ping(ctx).Err())
	require.NoError(t, client.FlushDB(ctx).Err())
	t.Cleanup(func() { _ = client.Close() })
	return redisqueue.NewRedisQueue(client)
}

func TestTickWithRealQueue(t *testing.T) {
	t.Run("マッチメイキング", func(t *testing.T) {
		t.Run("3人が待機しているとき、2人が組み合わされてマッチが成立し、残る1人は待機に残る", func(t *testing.T) {
			q := newRealQueue(t)
			ctx := context.Background()
			require.NoError(t, q.Enqueue(ctx, "p1", 1, "alice", 7))
			require.NoError(t, q.Enqueue(ctx, "p2", 2, "bob", 12))
			require.NoError(t, q.Enqueue(ctx, "p3", 3, "carol", 20))

			broker := apimatchmakingfake.NewBroker()
			publisher := apimatchmakingfake.NewPublisher(broker)
			m := New(q, publisher, defaultOpts())

			m.tick(ctx)

			published := publisher.Published()
			require.Len(t, published, 1)
			require.Equal(t, apimatchmaking.EventTypeMatchMade, published[0].Topic)

			var event apimatchmaking.MatchMadeEvent
			require.NoError(t, json.Unmarshal(published[0].Data, &event))
			require.NotEmpty(t, event.MatchID)
			require.ElementsMatch(t, []apimatchmaking.MatchedPlayer{
				{PlayerID: "p1", DeckID: 1, Name: "alice", Level: 7},
				{PlayerID: "p2", DeckID: 2, Name: "bob", Level: 12},
			}, event.Players)

			size, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(1), size)

			remaining, err := q.PopPair(ctx)
			require.NoError(t, err)
			require.Empty(t, remaining, "残り1人だけではマッチを組めない")
		})
	})
}

func TestTickSweepsExpiredEntries(t *testing.T) {
	t.Run("マッチメイキング", func(t *testing.T) {
		t.Run("登録から5分以上経過したエントリが残っているとき、マッチングの走査後に消え、マッチに使われない", func(t *testing.T) {
			q := newRealQueue(t)
			ctx := context.Background()
			require.NoError(t, q.Reenqueue(ctx, []domain.QueueEntry{
				{PlayerID: "stale", DeckID: 1, Name: "stale", Level: 1, JoinedAt: time.Now().Add(-6 * time.Minute)},
			}))
			require.NoError(t, q.Enqueue(ctx, "p1", 2, "p1", 1))

			broker := apimatchmakingfake.NewBroker()
			publisher := apimatchmakingfake.NewPublisher(broker)
			m := New(q, publisher, defaultOpts())

			m.tick(ctx)

			require.Empty(t, publisher.Published(), "登録から5分以上経過したエントリとの誤マッチが起きないこと")

			size, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(1), size, "掃除で消えるのは期限切れエントリのみ")

			removedStale, err := q.Cancel(ctx, "stale")
			require.NoError(t, err)
			require.False(t, removedStale, "登録から5分以上経過したエントリは掃除で既に消えている")

			removedP1, err := q.Cancel(ctx, "p1")
			require.NoError(t, err)
			require.True(t, removedP1, "登録から5分未満のエントリは掃除の対象にならず残る")
		})

		t.Run("登録から5分未満の2人が待機しているとき、掃除の影響を受けずマッチが成立する", func(t *testing.T) {
			q := newRealQueue(t)
			ctx := context.Background()
			require.NoError(t, q.Enqueue(ctx, "p1", 1, "alice", 7))
			require.NoError(t, q.Enqueue(ctx, "p2", 2, "bob", 12))

			broker := apimatchmakingfake.NewBroker()
			publisher := apimatchmakingfake.NewPublisher(broker)
			m := New(q, publisher, defaultOpts())

			m.tick(ctx)

			published := publisher.Published()
			require.Len(t, published, 1, "掃除の後も通常どおりマッチが成立すること")

			var event apimatchmaking.MatchMadeEvent
			require.NoError(t, json.Unmarshal(published[0].Data, &event))
			require.ElementsMatch(t, []apimatchmaking.MatchedPlayer{
				{PlayerID: "p1", DeckID: 1, Name: "alice", Level: 7},
				{PlayerID: "p2", DeckID: 2, Name: "bob", Level: 12},
			}, event.Players)
		})
	})
}
