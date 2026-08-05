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
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/abandon"
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

// runMatcherUntil はマッチングループを起動し、isReady が満たされたところで停止して完了を待つ。
func runMatcherUntil(t *testing.T, m *matcher.Matcher, isReady func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()
	require.Eventually(t, isReady, 2*time.Second, 5*time.Millisecond)
	cancel()
	<-done
}

func TestEnqueue(t *testing.T) {
	t.Run("enqueueエンドポイント", func(t *testing.T) {
		t.Run("受理されるとき、player_summaryが実queueに永続化される", func(t *testing.T) {
			q := newRealQueue(t)
			h := New(q, stubCircuit{}, abandon.New(q))
			ctx := context.Background()

			rec := serve(t, h, http.MethodPost, "/internal/v1/enqueue", `{"deck_id":3,"name":"alice","level":9,"gateway_instance_id":"g1"}`)
			require.Equal(t, http.StatusAccepted, rec.Code)

			size, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(1), size)

			_, err = q.Enqueue(ctx, "partner", 1, "partner", 1, "g1")
			require.NoError(t, err)
			pair, _, err := q.PopPair(ctx)
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

		t.Run("gateway_instance_idが直前の登録と異なるとき、以前のエントリが消え新しい登録だけが残る", func(t *testing.T) {
			q := newRealQueue(t)
			h := New(q, stubCircuit{}, abandon.New(q))
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
			name      string
			body      string
			wantError string
		}{
			{
				name:      "JSONが壊れているとき、400になり本文を読み取れなかったと応答から分かり、queueは空のまま",
				body:      `{`,
				wantError: "request body is not valid",
			},
			{
				name:      "deck_idを省略したとき、400になりdeck_idが必要だと応答から分かり、queueは空のまま",
				body:      `{"name":"alice","level":9,"gateway_instance_id":"g1"}`,
				wantError: "deck_id is required",
			},
			{
				name:      "deck_idが0のとき、400になりdeck_idが必要だと応答から分かり、queueは空のまま",
				body:      `{"deck_id":0,"name":"alice","level":9,"gateway_instance_id":"g1"}`,
				wantError: "deck_id is required",
			},
			{
				name:      "nameを省略したとき、400になりnameが必要だと応答から分かり、queueは空のまま",
				body:      `{"deck_id":3,"level":9,"gateway_instance_id":"g1"}`,
				wantError: "name is required",
			},
			{
				name:      "nameが空のとき、400になりnameが必要だと応答から分かり、queueは空のまま",
				body:      `{"deck_id":3,"name":"","level":9,"gateway_instance_id":"g1"}`,
				wantError: "name is required",
			},
			{
				name:      "gateway_instance_idを省略したとき、400になりgateway_instance_idが必要だと応答から分かり、queueは空のまま",
				body:      `{"deck_id":3,"name":"alice","level":9}`,
				wantError: "gateway_instance_id is required",
			},
			{
				name:      "gateway_instance_idが空のとき、400になりgateway_instance_idが必要だと応答から分かり、queueは空のまま",
				body:      `{"deck_id":3,"name":"alice","level":9,"gateway_instance_id":""}`,
				wantError: "gateway_instance_id is required",
			},
		}
		for _, tc := range rejectCases {
			t.Run(tc.name, func(t *testing.T) {
				q := newRealQueue(t)
				h := New(q, stubCircuit{}, abandon.New(q))

				rec := serve(t, h, http.MethodPost, "/internal/v1/enqueue", tc.body)

				require.Equal(t, http.StatusBadRequest, rec.Code)
				var body map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				require.Equal(t, tc.wantError, body["error"])
				size, err := q.Size(context.Background())
				require.NoError(t, err)
				require.Equal(t, int64(0), size)
			})
		}
	})
}

func TestCancel(t *testing.T) {
	t.Run("cancelエンドポイント", func(t *testing.T) {
		cases := []struct {
			name       string
			seed       []string
			wantStatus int
		}{
			{
				name:       "在籍プレイヤーがcancelするとき、200になりqueueから削除される",
				seed:       []string{testPlayerID},
				wantStatus: http.StatusOK,
			},
			{
				name:       "未在籍プレイヤーがcancelするとき、404になる",
				seed:       nil,
				wantStatus: http.StatusNotFound,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				q := newRealQueue(t)
				h := New(q, stubCircuit{}, abandon.New(q))
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

func TestReportMatchAbandoned(t *testing.T) {
	t.Run("マッチ不成立の申告", func(t *testing.T) {
		t.Run("成立して待機から消えたペアが申告されるとき、204になり待機は空のままになる", func(t *testing.T) {
			q := newRealQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "p1", 1, "alice", 7, "g1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "p2", 2, "bob", 12, "g1")
			require.NoError(t, err)

			m := matcher.New(q, stubPublisher{}, matcher.Options{
				Interval:     time.Millisecond,
				DrainTimeout: time.Second,
			})
			h := New(q, m, abandon.New(q))
			runMatcherUntil(t, m, func() bool {
				size, err := q.Size(ctx)
				return err == nil && size == 0
			})

			rec := serve(t, h, http.MethodPost, "/internal/v1/match-abandoned",
				`{"match_id":"TST-MATCH-1","player_ids":["p1","p2"],"reason":"player_not_connected"}`)

			require.Equal(t, http.StatusNoContent, rec.Code)
			size, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(0), size)
		})

		t.Run("配信に失敗して待機へ戻ったペアが申告されるとき、204になり2人とも待機から消える", func(t *testing.T) {
			q := newRealQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "p1", 1, "alice", 7, "g1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "p2", 2, "bob", 12, "g1")
			require.NoError(t, err)

			m := matcher.New(q, stubPublisher{err: errors.New("pubsub down")}, matcher.Options{
				Interval:         time.Millisecond,
				CircuitThreshold: 1,
				CircuitCooldown:  time.Hour,
				DrainTimeout:     time.Second,
			})
			h := New(q, m, abandon.New(q))
			runMatcherUntil(t, m, m.IsCircuitOpen)

			size, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(2), size, "配信に失敗したペアは待機に戻っている")

			rec := serve(t, h, http.MethodPost, "/internal/v1/match-abandoned",
				`{"match_id":"TST-MATCH-1","player_ids":["p1","p2"],"reason":"player_not_connected"}`)

			require.Equal(t, http.StatusNoContent, rec.Code)
			size, err = q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(0), size)
		})

		t.Run("同じ申告が二度届くとき、二度目も204になり、無関係の待機プレイヤーは待機に残る", func(t *testing.T) {
			q := newRealQueue(t)
			ctx := context.Background()
			_, err := q.Enqueue(ctx, "p1", 1, "alice", 7, "g1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "p2", 2, "bob", 12, "g1")
			require.NoError(t, err)
			_, err = q.Enqueue(ctx, "p3", 3, "carol", 20, "g1")
			require.NoError(t, err)
			h := New(q, stubCircuit{}, abandon.New(q))

			body := `{"match_id":"TST-MATCH-1","player_ids":["p1","p2"],"reason":"player_not_connected"}`
			first := serve(t, h, http.MethodPost, "/internal/v1/match-abandoned", body)
			require.Equal(t, http.StatusNoContent, first.Code)

			second := serve(t, h, http.MethodPost, "/internal/v1/match-abandoned", body)

			require.Equal(t, http.StatusNoContent, second.Code)
			size, err := q.Size(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(1), size)
			removed, err := q.Cancel(ctx, "p3")
			require.NoError(t, err)
			require.True(t, removed, "申告に含まれないプレイヤーは待機に残る")
		})

		rejectCases := []struct {
			name      string
			body      string
			wantError string
		}{
			{
				name:      "JSONが壊れているとき、400になり、読み取れなかったことが応答から分かる",
				body:      `{`,
				wantError: "request body is not valid",
			},
			{
				name:      "match_idが空のとき、400になり、match_idが必要だと応答から分かる",
				body:      `{"match_id":"","player_ids":["p1","p2"],"reason":"player_not_connected"}`,
				wantError: "match_id is required",
			},
			{
				name:      "player_idsが1件のとき、400になり、2件必要だと応答から分かる",
				body:      `{"match_id":"TST-MATCH-1","player_ids":["p1"],"reason":"player_not_connected"}`,
				wantError: "player_ids must contain exactly 2 ids",
			},
			{
				name:      "player_idsが3件のとき、400になり、2件必要だと応答から分かる",
				body:      `{"match_id":"TST-MATCH-1","player_ids":["p1","p2","p3"],"reason":"player_not_connected"}`,
				wantError: "player_ids must contain exactly 2 ids",
			},
			{
				name:      "player_idsに空のidが含まれるとき、400になり、空のidが理由だと応答から分かる",
				body:      `{"match_id":"TST-MATCH-1","player_ids":["p1",""],"reason":"player_not_connected"}`,
				wantError: "player_ids must not contain an empty id",
			},
			{
				name:      "reasonが契約にない値のとき、400になり、その値が未知だと応答から分かる",
				body:      `{"match_id":"TST-MATCH-1","player_ids":["p1","p2"],"reason":"TST-UNKNOWN-REASON"}`,
				wantError: `reason "TST-UNKNOWN-REASON" is not a known value`,
			},
			{
				name:      "reasonを省略したとき、400になり、空の理由が未知だと応答から分かる",
				body:      `{"match_id":"TST-MATCH-1","player_ids":["p1","p2"]}`,
				wantError: `reason "" is not a known value`,
			},
		}
		for _, tc := range rejectCases {
			t.Run(tc.name, func(t *testing.T) {
				q := newRealQueue(t)
				ctx := context.Background()
				_, err := q.Enqueue(ctx, "p1", 1, "alice", 7, "g1")
				require.NoError(t, err)
				_, err = q.Enqueue(ctx, "p2", 2, "bob", 12, "g1")
				require.NoError(t, err)
				h := New(q, stubCircuit{}, abandon.New(q))

				rec := serve(t, h, http.MethodPost, "/internal/v1/match-abandoned", tc.body)

				require.Equal(t, http.StatusBadRequest, rec.Code)
				var body map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				require.Contains(t, body["error"], tc.wantError)
				size, err := q.Size(ctx)
				require.NoError(t, err)
				require.Equal(t, int64(2), size, "申告が拒否されたとき、2人は待機に残る")
			})
		}
	})
}

func TestQueueSize(t *testing.T) {
	t.Run("queue-sizeエンドポイント", func(t *testing.T) {
		cases := []struct {
			name     string
			players  []string
			wantSize int64
		}{
			{
				name:     "0名のとき、sizeは0になる",
				players:  nil,
				wantSize: 0,
			},
			{
				name:     "1名のとき、sizeは1になる",
				players:  []string{"p1"},
				wantSize: 1,
			},
			{
				name:     "複数名 (3名)のとき、sizeは3になる",
				players:  []string{"p1", "p2", "p3"},
				wantSize: 3,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				q := newRealQueue(t)
				h := New(q, stubCircuit{}, abandon.New(q))
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
	t.Run("healthエンドポイント", func(t *testing.T) {
		t.Run("circuitがclosedのとき、200でstatus=ok / circuit=closedを返す", func(t *testing.T) {
			q := newRealQueue(t)
			m := matcher.New(q, stubPublisher{}, matcher.Options{})
			h := New(q, m, abandon.New(q))

			rec := serve(t, h, http.MethodGet, "/internal/v1/health", "")

			require.Equal(t, http.StatusOK, rec.Code)
			var body apimatchmaking.HealthResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, healthStatusOK, body.Status)
			require.Equal(t, healthCircuitClosed, body.Circuit)
		})

		t.Run("circuitがopenのとき、503でstatus=degraded / circuit=openを返す", func(t *testing.T) {
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
			h := New(q, m, abandon.New(q))
			runMatcherUntil(t, m, m.IsCircuitOpen)

			rec := serve(t, h, http.MethodGet, "/internal/v1/health", "")

			require.Equal(t, http.StatusServiceUnavailable, rec.Code)
			var body apimatchmaking.HealthResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, healthStatusDegraded, body.Status)
			require.Equal(t, healthCircuitOpen, body.Circuit)
		})
	})
}
