package httphandler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
)

// okQueue は全操作が成功する port.Queue 実装。router の auth 配線検証で
// handler の成功応答まで到達させるために使う。
type okQueue struct{}

// Enqueue は Valkey なしで enqueue の成功応答経路を通すためのスタブ。
func (okQueue) Enqueue(context.Context, string, int64, string, int64, string) (int64, error) {
	return 0, nil
}

// Cancel は Valkey なしで cancel の成功応答経路を通すためのスタブ。
func (okQueue) Cancel(context.Context, string) (bool, error) { return true, nil }

// Size は Valkey なしで queue-size の成功応答経路を通すためのスタブ。
func (okQueue) Size(context.Context) (int64, error) { return 0, nil }

// PopPair は port.Queue を満たすためのスタブ。
func (okQueue) PopPair(context.Context) ([]domain.QueueEntry, string, error) { return nil, "", nil }

// Reenqueue は port.Queue を満たすためのスタブ。
func (okQueue) Reenqueue(context.Context, []domain.QueueEntry, string) (bool, error) {
	return true, nil
}

func TestNewRouter(t *testing.T) {
	t.Run("auth 配線", func(t *testing.T) {
		readRouteCases := []struct {
			name string
			path string
		}{
			{name: "queue-size は auth-free で 200 を返す", path: "/internal/v1/queue-size"},
			{name: "health は auth-free で 200 を返す", path: "/internal/v1/health"},
		}
		for _, tc := range readRouteCases {
			t.Run(tc.name, func(t *testing.T) {
				// VerifyFn 未設定: auth-free ルートが verifier に到達しないことの検出を兼ねる
				r := NewRouter(New(okQueue{}, stubCircuit{}), &internalauth.MockVerifier{})
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
				assert.Equal(t, http.StatusOK, w.Code)
			})
		}

		missingHeaderCases := []struct {
			name string
			path string
		}{
			{name: "POST /enqueue は auth header 欠落で 401 になる", path: "/internal/v1/enqueue"},
			{name: "POST /cancel は auth header 欠落で 401 になる", path: "/internal/v1/cancel"},
		}
		for _, tc := range missingHeaderCases {
			t.Run(tc.name, func(t *testing.T) {
				// VerifyFn 未設定: header 欠落時は middleware が verifier に到達しないことの検出を兼ねる
				r := NewRouter(New(okQueue{}, stubCircuit{}), &internalauth.MockVerifier{})
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, tc.path, nil))
				assert.Equal(t, http.StatusUnauthorized, w.Code)
			})
		}

		t.Run("player ルートは verifier がエラーを返すとき、401 になる", func(t *testing.T) {
			r := NewRouter(New(okQueue{}, stubCircuit{}), &internalauth.MockVerifier{
				VerifyFn: func(string) (string, error) { return "", errors.New("invalid token") },
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/internal/v1/cancel", nil)
			req.Header.Set(internalauth.HeaderName, "any.token")
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("player ルートは有効なトークンのとき、handler の成功応答まで到達する", func(t *testing.T) {
			r := NewRouter(New(okQueue{}, stubCircuit{}), &internalauth.MockVerifier{
				VerifyFn: func(string) (string, error) { return testPlayerID, nil },
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/internal/v1/enqueue",
				strings.NewReader(`{"deck_id":1,"name":"TST-NAME","level":1,"gateway_instance_id":"TST-GATEWAY-1"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(internalauth.HeaderName, "any.token")
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusAccepted, w.Code)
		})
	})
}
