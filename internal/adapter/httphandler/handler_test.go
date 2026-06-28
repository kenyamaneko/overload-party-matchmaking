package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

// testPlayerID は VerifyInternalAuth が context に注入した player_id を模した固定値。
const testPlayerID = "player-123"

// enqueueCall は fakeQueue が記録する 1 回の Enqueue 呼び出し。
type enqueueCall struct {
	playerID string
	deckID   int64
	name     string
	level    int64
}

// fakeQueue は port.Queue を実装し、呼び出し記録と返り値を制御するテストダブル。
type fakeQueue struct {
	enqueueErr   error
	enqueueCalls []enqueueCall

	cancelIsRemoved bool
	cancelErr       error
	cancelCalls     []string

	sizeValue int64
	sizeErr   error
}

// Enqueue は port.Queue.Enqueue のテストダブル。
func (f *fakeQueue) Enqueue(ctx context.Context, playerID string, deckID int64, name string, level int64) error {
	f.enqueueCalls = append(f.enqueueCalls, enqueueCall{playerID: playerID, deckID: deckID, name: name, level: level})
	return f.enqueueErr
}

// Cancel は port.Queue.Cancel のテストダブル。
func (f *fakeQueue) Cancel(ctx context.Context, playerID string) (bool, error) {
	f.cancelCalls = append(f.cancelCalls, playerID)
	return f.cancelIsRemoved, f.cancelErr
}

// Size は port.Queue.Size のテストダブル。
func (f *fakeQueue) Size(ctx context.Context) (int64, error) { return f.sizeValue, f.sizeErr }

// PopPair は port.Queue を満たすためのスタブ。
func (f *fakeQueue) PopPair(ctx context.Context) ([]domain.QueueEntry, error) { return nil, nil }

// Reenqueue は port.Queue を満たすためのスタブ。
func (f *fakeQueue) Reenqueue(ctx context.Context, entries []domain.QueueEntry) error { return nil }

// fakeCircuit は CircuitStater を実装し、open 状態を固定で返すテストダブル。
type fakeCircuit struct {
	isOpen bool
}

// IsCircuitOpen は CircuitStater.IsCircuitOpen のテストダブル。
func (f fakeCircuit) IsCircuitOpen() bool { return f.isOpen }

// serve は player_id を注入したエンジンにリクエストを通し、レスポンスレコーダを返す。
func serve(t *testing.T, h *Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// VerifyInternalAuth が context に player_id を入れる挙動を模す。
	r.Use(func(c *gin.Context) { c.Set(internalauth.PlayerIDContextKey, testPlayerID) })
	r.POST("/internal/v1/enqueue", h.Enqueue)
	r.POST("/internal/v1/cancel", h.Cancel)
	r.GET("/internal/v1/queue-size", h.RespondQueueSize)
	r.GET("/internal/v1/health", h.RespondHealth)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// TestEnqueue は enqueue の受理・入力検証・queue 失敗の応答契約を固定する。
func TestEnqueue(t *testing.T) {
	const validBody = `{"deck_id":3,"name":"alice","level":9}`
	forwarded := []enqueueCall{{playerID: testPlayerID, deckID: 3, name: "alice", level: 9}}
	cases := []struct {
		name       string
		body       string
		enqueueErr error
		wantStatus int
		wantCalls  []enqueueCall
	}{
		{name: "accepted forwards player summary", body: validBody, wantStatus: http.StatusAccepted, wantCalls: forwarded},
		// 欠落と空 / ゼロ値は presence/absence の区別を値差に潰さないよう別ケースに保つ。
		{name: "malformed json", body: `{`, wantStatus: http.StatusBadRequest},
		{name: "deck_id omitted", body: `{"name":"alice","level":9}`, wantStatus: http.StatusBadRequest},
		{name: "deck_id zero", body: `{"deck_id":0,"name":"alice","level":9}`, wantStatus: http.StatusBadRequest},
		{name: "name omitted", body: `{"deck_id":3,"level":9}`, wantStatus: http.StatusBadRequest},
		{name: "name empty", body: `{"deck_id":3,"name":"","level":9}`, wantStatus: http.StatusBadRequest},
		{name: "queue error", body: validBody, enqueueErr: errors.New("redis down"), wantStatus: http.StatusServiceUnavailable, wantCalls: forwarded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeQueue{enqueueErr: tc.enqueueErr}
			h := New(q, fakeCircuit{})

			rec := serve(t, h, http.MethodPost, "/internal/v1/enqueue", tc.body)

			require.Equal(t, tc.wantStatus, rec.Code)
			require.Equal(t, tc.wantCalls, q.enqueueCalls)
		})
	}
}

// TestCancel は削除結果に応じてステータスを分け、いずれの分岐でも context の player_id を
// queue に渡すことを検証する。
func TestCancel(t *testing.T) {
	cases := []struct {
		name       string
		isRemoved  bool
		queueErr   error
		wantStatus int
	}{
		{name: "removed", isRemoved: true, queueErr: nil, wantStatus: http.StatusOK},
		{name: "not found", isRemoved: false, queueErr: nil, wantStatus: http.StatusNotFound},
		{name: "queue error", isRemoved: false, queueErr: errors.New("redis down"), wantStatus: http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeQueue{cancelIsRemoved: tc.isRemoved, cancelErr: tc.queueErr}
			h := New(q, fakeCircuit{})

			rec := serve(t, h, http.MethodPost, "/internal/v1/cancel", "")

			require.Equal(t, tc.wantStatus, rec.Code)
			require.Equal(t, []string{testPlayerID}, q.cancelCalls)
		})
	}
}

// TestQueueSize はキュー件数の応答と queue 失敗時の応答契約を固定する。
func TestQueueSize(t *testing.T) {
	cases := []struct {
		name       string
		size       int64
		sizeErr    error
		wantStatus int
		wantSize   int64
	}{
		{name: "empty queue", size: 0, wantStatus: http.StatusOK, wantSize: 0},
		{name: "non-empty queue", size: 42, wantStatus: http.StatusOK, wantSize: 42},
		{name: "queue error", sizeErr: errors.New("redis down"), wantStatus: http.StatusServiceUnavailable, wantSize: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeQueue{sizeValue: tc.size, sizeErr: tc.sizeErr}
			h := New(q, fakeCircuit{})

			rec := serve(t, h, http.MethodGet, "/internal/v1/queue-size", "")

			require.Equal(t, tc.wantStatus, rec.Code)
			var body apimatchmaking.QueueSizeResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, tc.wantSize, body.Size)
		})
	}
}

// TestHealth は circuit の open / closed / 未設定に応じて status code と body の
// status・circuit 値が決まることを検証する。
func TestHealth(t *testing.T) {
	cases := []struct {
		name        string
		circuit     CircuitStater
		wantStatus  int
		wantHealth  string
		wantCircuit string
	}{
		{name: "circuit open", circuit: fakeCircuit{isOpen: true}, wantStatus: http.StatusServiceUnavailable, wantHealth: healthStatusDegraded, wantCircuit: healthCircuitOpen},
		{name: "circuit closed", circuit: fakeCircuit{isOpen: false}, wantStatus: http.StatusOK, wantHealth: healthStatusOK, wantCircuit: healthCircuitClosed},
		{name: "circuit nil", circuit: nil, wantStatus: http.StatusOK, wantHealth: healthStatusOK, wantCircuit: healthCircuitClosed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := New(&fakeQueue{}, tc.circuit)

			rec := serve(t, h, http.MethodGet, "/internal/v1/health", "")

			require.Equal(t, tc.wantStatus, rec.Code)
			var body apimatchmaking.HealthResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, tc.wantHealth, body.Status)
			require.Equal(t, tc.wantCircuit, body.Circuit)
		})
	}
}
