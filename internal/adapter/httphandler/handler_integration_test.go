//go:build integration

package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/redisqueue"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/valkeytest"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

// testRedisURL は TestMain が起動した Valkey container の接続 URL。
var testRedisURL string

// TestMain はパッケージ内の結合テスト全体で共有する Valkey container を起動する。
func TestMain(m *testing.M) {
	ctx := context.Background()
	// 各テストは newRealQueue の FLUSHDB で分離するため、container は 1 つを共有すれば足りる。
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

// stubPublisher は port.RawEventPublisher を満たし、固定結果 (nil = 成功) を返す。
type stubPublisher struct {
	err error
}

// Publish は注入された固定結果を返す。
func (p stubPublisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	return p.err
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

func TestEnqueue(t *testing.T) {
	t.Run("enqueue エンドポイント", func(t *testing.T) {
		t.Run("受理されるとき、player_summary が実 queue に永続化される", func(t *testing.T) {
			q := newRealQueue(t)
			h := New(q, stubCircuit{})
			ctx := context.Background()

			rec := serve(t, h, http.MethodPost, "/internal/v1/enqueue", `{"deck_id":3,"name":"alice","level":9,"gateway_instance_id":"g1"}`)
			require.Equal(t, http.StatusAccepted, rec.Code)

			size, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(1), size)

			_, err = q.Enqueue(ctx, "partner", 1, "partner", 1, "g1")
			require.NoError(t, err)
			pair, err := q.PopPair(ctx)
			require.NoError(t, err)
			require.Len(t, pair, 2)

			// PopPair の取り出し順は enqueue 順と一致する保証がないため、順序非依存に永続内容を突き合わせる。
			type persistedSummary struct {
				playerID string
				deckID   int64
				name     string
				level    int64
			}
			got := make([]persistedSummary, 0, len(pair))
			for _, e := range pair {
				got = append(got, persistedSummary{e.PlayerID, e.DeckID, e.Name, e.Level})
			}
			require.ElementsMatch(t, []persistedSummary{
				{testPlayerID, 3, "alice", 9},
				{"partner", 1, "partner", 1},
			}, got)
		})

		t.Run("gateway_instance_id が直前の登録と異なるとき、以前のエントリが消え新しい登録だけが残る", func(t *testing.T) {
			q := newRealQueue(t)
			h := New(q, stubCircuit{})
			ctx := context.Background()

			_, err := q.Enqueue(ctx, "stale-partner", 1, "stale-partner", 1, "g1")
			require.NoError(t, err)

			rec := serve(t, h, http.MethodPost, "/internal/v1/enqueue", `{"deck_id":3,"name":"alice","level":9,"gateway_instance_id":"g2"}`)
			require.Equal(t, http.StatusAccepted, rec.Code)

			size, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(1), size, "別プロセスへの切り替わりとみなし以前のエントリを消して新しい登録だけが残る")

			removed, err := q.Cancel(ctx, "stale-partner")
			require.NoError(t, err)
			require.False(t, removed, "以前の識別子で登録したエントリは消えている")
		})

		rejectCases := []struct {
			name string
			body string
		}{
			{
				name: "JSON が壊れているとき、400 になり queue は空のまま",
				body: `{`,
			},
			{
				name: "deck_id を省略したとき、400 になり queue は空のまま",
				body: `{"name":"alice","level":9,"gateway_instance_id":"g1"}`,
			},
			{
				name: "deck_id が 0 のとき、400 になり queue は空のまま",
				body: `{"deck_id":0,"name":"alice","level":9,"gateway_instance_id":"g1"}`,
			},
			{
				name: "name を省略したとき、400 になり queue は空のまま",
				body: `{"deck_id":3,"level":9,"gateway_instance_id":"g1"}`,
			},
			{
				name: "name が空のとき、400 になり queue は空のまま",
				body: `{"deck_id":3,"name":"","level":9,"gateway_instance_id":"g1"}`,
			},
			{
				name: "gateway_instance_id を省略したとき、400 になり queue は空のまま",
				body: `{"deck_id":3,"name":"alice","level":9}`,
			},
			{
				name: "gateway_instance_id が空のとき、400 になり queue は空のまま",
				body: `{"deck_id":3,"name":"alice","level":9,"gateway_instance_id":""}`,
			},
		}
		for _, tc := range rejectCases {
			t.Run(tc.name, func(t *testing.T) {
				q := newRealQueue(t)
				h := New(q, stubCircuit{})

				rec := serve(t, h, http.MethodPost, "/internal/v1/enqueue", tc.body)

				require.Equal(t, http.StatusBadRequest, rec.Code)
				size, err := q.Size(context.Background())
				require.NoError(t, err)
				require.Equal(t, int64(0), size)
			})
		}
	})
}

func TestCancel(t *testing.T) {
	t.Run("cancel エンドポイント", func(t *testing.T) {
		cases := []struct {
			name       string
			seed       []string
			wantStatus int
		}{
			{
				name:       "在籍プレイヤーが cancel するとき、200 になり queue から削除される",
				seed:       []string{testPlayerID},
				wantStatus: http.StatusOK,
			},
			{
				name:       "未在籍プレイヤーが cancel するとき、404 になる",
				seed:       nil,
				wantStatus: http.StatusNotFound,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				q := newRealQueue(t)
				h := New(q, stubCircuit{})
				ctx := context.Background()
				for _, id := range tc.seed {
					_, err := q.Enqueue(ctx, id, 1, id, 1, "g1")
					require.NoError(t, err)
				}

				preSize, err := q.Size(ctx)
				require.NoError(t, err)
				require.Equal(t, int64(len(tc.seed)), preSize)

				rec := serve(t, h, http.MethodPost, "/internal/v1/cancel", "")

				require.Equal(t, tc.wantStatus, rec.Code)
				postSize, err := q.Size(ctx)
				require.NoError(t, err)
				require.Equal(t, int64(0), postSize)
			})
		}
	})
}

func TestQueueSize(t *testing.T) {
	t.Run("queue-size エンドポイント", func(t *testing.T) {
		cases := []struct {
			name     string
			players  []string
			wantSize int64
		}{
			{
				name:     "0 名のとき、size は 0 になる",
				players:  nil,
				wantSize: 0,
			},
			{
				name:     "1 名のとき、size は 1 になる",
				players:  []string{"p1"},
				wantSize: 1,
			},
			{
				name:     "複数名 (3 名) のとき、size は 3 になる",
				players:  []string{"p1", "p2", "p3"},
				wantSize: 3,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				q := newRealQueue(t)
				h := New(q, stubCircuit{})
				ctx := context.Background()
				for _, id := range tc.players {
					_, err := q.Enqueue(ctx, id, 1, id, 1, "g1")
					require.NoError(t, err)
				}

				rec := serve(t, h, http.MethodGet, "/internal/v1/queue-size", "")

				require.Equal(t, http.StatusOK, rec.Code)
				var body apimatchmaking.QueueSizeResponse
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				require.Equal(t, tc.wantSize, body.Size)
			})
		}
	})
}

func TestHealth(t *testing.T) {
	t.Run("health エンドポイント", func(t *testing.T) {
		t.Run("circuit が closed のとき、200 で status=ok / circuit=closed を返す", func(t *testing.T) {
			q := newRealQueue(t)
			m := matcher.New(q, stubPublisher{}, matcher.Options{})
			h := New(q, m)

			rec := serve(t, h, http.MethodGet, "/internal/v1/health", "")

			require.Equal(t, http.StatusOK, rec.Code)
			var body apimatchmaking.HealthResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, healthStatusOK, body.Status)
			require.Equal(t, healthCircuitClosed, body.Circuit)
		})

		t.Run("circuit が open のとき、503 で status=degraded / circuit=open を返す", func(t *testing.T) {
			q := newRealQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "p1", 1, "p1", 1, "g1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "p2", 2, "p2", 1, "g1")
			require.NoError(t, err)

			m := matcher.New(q, stubPublisher{err: errors.New("pubsub down")}, matcher.Options{
				Interval:         time.Millisecond,
				CircuitThreshold: 3,
				CircuitCooldown:  time.Hour,
				DrainTimeout:     time.Second,
			})
			h := New(q, m)

			runCtx, cancel := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() {
				m.Run(runCtx)
				close(done)
			}()
			t.Cleanup(func() {
				cancel()
				<-done
			})

			require.Eventually(t, m.IsCircuitOpen, 2*time.Second, 5*time.Millisecond)

			rec := serve(t, h, http.MethodGet, "/internal/v1/health", "")

			require.Equal(t, http.StatusServiceUnavailable, rec.Code)
			var body apimatchmaking.HealthResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, healthStatusDegraded, body.Status)
			require.Equal(t, healthCircuitOpen, body.Circuit)
		})
	})
}
