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

// TestEnqueueAcceptedPersistsPlayerSummary は受理された enqueue が player_summary を実 queue へ永続させる契約を検証する。
func TestEnqueueAcceptedPersistsPlayerSummary(t *testing.T) {
	q := newRealQueue(t)
	h := New(q, stubCircuit{})
	ctx := context.Background()

	rec := serve(t, h, http.MethodPost, "/internal/v1/enqueue", `{"deck_id":3,"name":"alice","level":9}`)
	require.Equal(t, http.StatusAccepted, rec.Code)

	size, err := q.Size(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), size)

	require.NoError(t, q.Enqueue(ctx, "partner", 1, "partner", 1))
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
}

// TestEnqueueRejectedLeavesQueueEmpty は入力検証で拒否された enqueue が実 queue に副作用を残さない契約を検証する。
func TestEnqueueRejectedLeavesQueueEmpty(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "malformed json",
			body: `{`,
		},
		{
			name: "deck_id omitted",
			body: `{"name":"alice","level":9}`,
		},
		{
			name: "deck_id zero",
			body: `{"deck_id":0,"name":"alice","level":9}`,
		},
		{
			name: "name omitted",
			body: `{"deck_id":3,"level":9}`,
		},
		{
			name: "name empty",
			body: `{"deck_id":3,"name":"","level":9}`,
		},
	}
	for _, tc := range cases {
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
}

// TestCancelDistinguishesRemovalFromAbsence は cancel が在籍と未在籍をステータスで区別する契約を検証する。
func TestCancelDistinguishesRemovalFromAbsence(t *testing.T) {
	cases := []struct {
		name       string
		seed       []string
		wantStatus int
	}{
		{
			name:       "enqueued player removed",
			seed:       []string{testPlayerID},
			wantStatus: http.StatusOK,
		},
		{
			name:       "absent player not found",
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
				require.NoError(t, q.Enqueue(ctx, id, 1, id, 1))
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
}

// TestQueueSizeReflectsEnqueuedCount は queue-size が実 queue の在籍数を反映する契約を検証する。
func TestQueueSizeReflectsEnqueuedCount(t *testing.T) {
	cases := []struct {
		name     string
		players  []string
		wantSize int64
	}{
		{
			name:     "empty queue",
			players:  nil,
			wantSize: 0,
		},
		{
			name:     "single player",
			players:  []string{"p1"},
			wantSize: 1,
		},
		{
			name:     "multiple players",
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
				require.NoError(t, q.Enqueue(ctx, id, 1, id, 1))
			}

			rec := serve(t, h, http.MethodGet, "/internal/v1/queue-size", "")

			require.Equal(t, http.StatusOK, rec.Code)
			var body apimatchmaking.QueueSizeResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, tc.wantSize, body.Size)
		})
	}
}

// TestHealthReportsClosedWhenCircuitClosed は circuit が健全なとき health が稼働を報告する契約を検証する。
func TestHealthReportsClosedWhenCircuitClosed(t *testing.T) {
	q := newRealQueue(t)
	m := matcher.New(q, stubPublisher{}, matcher.Options{})
	h := New(q, m)

	rec := serve(t, h, http.MethodGet, "/internal/v1/health", "")

	require.Equal(t, http.StatusOK, rec.Code)
	var body apimatchmaking.HealthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, healthStatusOK, body.Status)
	require.Equal(t, healthCircuitClosed, body.Circuit)
}

// TestHealthReportsDegradedWhenCircuitOpens は circuit open が health の劣化報告へ反映される契約を検証する。
func TestHealthReportsDegradedWhenCircuitOpens(t *testing.T) {
	q := newRealQueue(t)
	ctx := context.Background()
	require.NoError(t, q.Enqueue(ctx, "p1", 1, "p1", 1))
	require.NoError(t, q.Enqueue(ctx, "p2", 2, "p2", 1))

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
}
