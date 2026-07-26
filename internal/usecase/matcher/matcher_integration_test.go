//go:build integration

package matcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/redisqueue"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/valkeytest"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
	"github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking/apimatchmakingfake"
)

// failThenSucceedPublisher は shouldFail の間だけ配信を失敗させ、それ以外は inner に委譲する。
// 配信失敗 → 実キューへの復元 → 配信成功、という一連の経路を 1 台の Publisher で駆動するためのテスト用差し替え。
type failThenSucceedPublisher struct {
	inner      *apimatchmakingfake.Publisher
	shouldFail bool
}

func (p *failThenSucceedPublisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	if p.shouldFail {
		return errors.New("publish failed (injected)")
	}
	return p.inner.Publish(ctx, eventType, payload)
}

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
			_, err := q.Enqueue(ctx, "p1", 1, "alice", 7, "g1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "p2", 2, "bob", 12, "g1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "p3", 3, "carol", 20, "g1")
			require.NoError(t, err)

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

			remaining, _, err := q.PopPair(ctx)
			require.NoError(t, err)
			require.Empty(t, remaining, "残り1人だけではマッチを組めない")
		})

		t.Run("1人だけ待機しているとき、マッチは成立せず1人が待機に残る", func(t *testing.T) {
			q := newRealQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "p1", 1, "alice", 7, "g1")
			require.NoError(t, err)

			broker := apimatchmakingfake.NewBroker()
			publisher := apimatchmakingfake.NewPublisher(broker)
			m := New(q, publisher, defaultOpts())

			m.tick(ctx)

			require.Empty(t, publisher.Published())
			size, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(1), size)
		})

		t.Run("2人のマッチの配信が失敗するとき、2人とも待機キューに戻り、次の配信成功で同じ2人のマッチが成立する", func(t *testing.T) {
			q := newRealQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "p1", 1, "alice", 7, "g1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "p2", 2, "bob", 12, "g1")
			require.NoError(t, err)

			broker := apimatchmakingfake.NewBroker()
			publisher := &failThenSucceedPublisher{inner: apimatchmakingfake.NewPublisher(broker), shouldFail: true}
			m := New(q, publisher, defaultOpts())

			m.tick(ctx)

			size, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(2), size, "配信失敗後、2人とも待機キューに戻る")

			publisher.shouldFail = false
			m.tick(ctx)

			published := publisher.inner.Published()
			require.Len(t, published, 1)
			var event apimatchmaking.MatchMadeEvent
			require.NoError(t, json.Unmarshal(published[0].Data, &event))
			require.ElementsMatch(t, []apimatchmaking.MatchedPlayer{
				{PlayerID: "p1", DeckID: 1, Name: "alice", Level: 7},
				{PlayerID: "p2", DeckID: 2, Name: "bob", Level: 12},
			}, event.Players)

			size, err = q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(0), size)
		})

		t.Run("前の gateway プロセスのエントリが残っているとき、新しい gateway から2人が登録すると、その2人でマッチが成立する", func(t *testing.T) {
			q := newRealQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "stale1", 1, "stale1", 1, "g1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "stale2", 2, "stale2", 1, "g1")
			require.NoError(t, err)

			removed, err := q.Enqueue(ctx, "p1", 3, "alice", 7, "g2")
			require.NoError(t, err)
			require.Equal(t, int64(2), removed, "別プロセスへの切り替わりとみなし以前の2件を削除する")
			_, err = q.Enqueue(ctx, "p2", 4, "bob", 12, "g2")
			require.NoError(t, err)

			broker := apimatchmakingfake.NewBroker()
			publisher := apimatchmakingfake.NewPublisher(broker)
			m := New(q, publisher, defaultOpts())

			m.tick(ctx)

			published := publisher.Published()
			require.Len(t, published, 1)

			var event apimatchmaking.MatchMadeEvent
			require.NoError(t, json.Unmarshal(published[0].Data, &event))
			require.ElementsMatch(t, []apimatchmaking.MatchedPlayer{
				{PlayerID: "p1", DeckID: 3, Name: "alice", Level: 7},
				{PlayerID: "p2", DeckID: 4, Name: "bob", Level: 12},
			}, event.Players, "リセット後に登録した2名だけでマッチが成立する")
		})
	})
}
