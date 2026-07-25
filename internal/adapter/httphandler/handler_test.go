package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
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

// RemoveExpired は port.Queue を満たすためのスタブ。
func (q errQueue) RemoveExpired(ctx context.Context, before time.Time) (int64, error) {
	return 0, nil
}

// stubCircuit は circuit を使わないエンドポイントのテスト配線に用いる CircuitStater 実装。
type stubCircuit struct{}

// IsCircuitOpen は circuit 非依存のエンドポイントを検証できるよう health を closed に保つ。
func (c stubCircuit) IsCircuitOpen() bool { return false }

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

func TestEndpointReturns503WhenQueueFails(t *testing.T) {
	t.Run("待機列ストアが使えないとき", func(t *testing.T) {
		cases := []struct {
			name   string
			method string
			target string
			body   string
		}{
			{
				name:   "参加登録は 503 になり、エラー内容が応答に含まれる",
				method: http.MethodPost,
				target: "/internal/v1/enqueue",
				body:   `{"deck_id":3,"name":"alice","level":9}`,
			},
			{
				name:   "キャンセルは 503 になり、エラー内容が応答に含まれる",
				method: http.MethodPost,
				target: "/internal/v1/cancel",
				body:   "",
			},
			{
				name:   "待機人数の取得は 503 になり、エラー内容が応答に含まれる",
				method: http.MethodGet,
				target: "/internal/v1/queue-size",
				body:   "",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := New(errQueue{err: errors.New("redis down")}, stubCircuit{})

				rec := serve(t, h, tc.method, tc.target, tc.body)

				require.Equal(t, http.StatusServiceUnavailable, rec.Code)
				var body map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				require.Equal(t, "redis down", body["error"])
			})
		}
	})
}
