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

// testPlayerID は VerifyInternalAuth が context に注入する player_id を模した固定値。
const testPlayerID = "player-123"

// errQueue は全操作で固定エラーを返す port.Queue 実装。
// 健全な Valkey では起こらない「redis down」分岐 (503) の到達にのみ使う。
type errQueue struct {
	err error
}

// Enqueue は注入された固定エラーを返す。
func (q errQueue) Enqueue(ctx context.Context, playerID string, deckID int64, name string, level int64) error {
	return q.err
}

// Cancel は注入された固定エラーを返す。
func (q errQueue) Cancel(ctx context.Context, playerID string) (bool, error) { return false, q.err }

// Size は注入された固定エラーを返す。
func (q errQueue) Size(ctx context.Context) (int64, error) { return 0, q.err }

// PopPair は port.Queue を満たすためのスタブ。
func (q errQueue) PopPair(ctx context.Context) ([]domain.QueueEntry, error) { return nil, nil }

// Reenqueue は port.Queue を満たすためのスタブ。
func (q errQueue) Reenqueue(ctx context.Context, entries []domain.QueueEntry) error { return nil }

// serve は player_id を注入したエンジンにリクエストを通し、レスポンスレコーダを返す。
func serve(t *testing.T, h *Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
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

// TestEndpointReturns503WhenQueueFails は queue 障害時に各エンドポイントが利用不可を返す契約を検証する。
func TestEndpointReturns503WhenQueueFails(t *testing.T) {
	cases := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{
			name:   "enqueue",
			method: http.MethodPost,
			target: "/internal/v1/enqueue",
			body:   `{"deck_id":3,"name":"alice","level":9}`,
		},
		{
			name:   "cancel",
			method: http.MethodPost,
			target: "/internal/v1/cancel",
			body:   "",
		},
		{
			name:   "queue-size",
			method: http.MethodGet,
			target: "/internal/v1/queue-size",
			body:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := New(errQueue{err: errors.New("redis down")}, nil)

			rec := serve(t, h, tc.method, tc.target, tc.body)

			require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		})
	}
}

// TestHealthWithoutCircuitReportsClosed は circuit 未配線時に health が稼働を報告する契約を検証する。
func TestHealthWithoutCircuitReportsClosed(t *testing.T) {
	h := New(errQueue{}, nil)

	rec := serve(t, h, http.MethodGet, "/internal/v1/health", "")

	require.Equal(t, http.StatusOK, rec.Code)
	var body apimatchmaking.HealthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, healthStatusOK, body.Status)
	require.Equal(t, healthCircuitClosed, body.Circuit)
}
