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

// fakeRouterVerifier は router テスト用の internalauth.Verifier 最小 fake。
type fakeRouterVerifier struct {
	playerID string
	err      error
}

// Verify は token を実検証せず、auth middleware の通過 / 拒否だけを制御する。
func (f fakeRouterVerifier) Verify(string) (string, error) {
	return f.playerID, f.err
}

// okQueue は全操作が成功する port.Queue 実装。router の auth 配線検証で
// handler の成功応答まで到達させるために使う。
type okQueue struct{}

// Enqueue は Valkey なしで enqueue の成功応答経路を通すためのスタブ。
func (okQueue) Enqueue(context.Context, string, int64, string, int64) error { return nil }

// Cancel は Valkey なしで cancel の成功応答経路を通すためのスタブ。
func (okQueue) Cancel(context.Context, string) (bool, error) { return true, nil }

// Size は Valkey なしで queue-size の成功応答経路を通すためのスタブ。
func (okQueue) Size(context.Context) (int64, error) { return 0, nil }

// PopPair は port.Queue を満たすためのスタブ。
func (okQueue) PopPair(context.Context) ([]domain.QueueEntry, error) { return nil, nil }

// Reenqueue は port.Queue を満たすためのスタブ。
func (okQueue) Reenqueue(context.Context, []domain.QueueEntry) error { return nil }

// TestNewRouter_ReadRoutesAreAuthFree は queue-size / health が auth header なしで
// handler の成功応答まで到達することを確かめる。
func TestNewRouter_ReadRoutesAreAuthFree(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{
			name: "queue-size は auth-free でキュー数を返し 200",
			path: "/internal/v1/queue-size",
		},
		{
			name: "health は auth-free で稼働状態を返し 200",
			path: "/internal/v1/health",
		},
	}

	r := NewRouter(New(okQueue{}, stubCircuit{}), fakeRouterVerifier{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

// TestNewRouter_PlayerRoutesRequireInternalAuth は enqueue / cancel が auth header
// 欠落で 401 を返し handler に到達しないことを確かめる。
func TestNewRouter_PlayerRoutesRequireInternalAuth(t *testing.T) {
	r := NewRouter(New(okQueue{}, stubCircuit{}), fakeRouterVerifier{playerID: testPlayerID})

	cases := []struct {
		name string
		path string
	}{
		{
			name: "POST /enqueue は auth header 欠落で 401",
			path: "/internal/v1/enqueue",
		},
		{
			name: "POST /cancel は auth header 欠落で 401",
			path: "/internal/v1/cancel",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, tc.path, nil))
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

// TestNewRouter_PlayerRouteRejectsVerifierError は verifier が error を返すと 401 を
// 返し handler に到達しないことを確かめる。
func TestNewRouter_PlayerRouteRejectsVerifierError(t *testing.T) {
	r := NewRouter(New(okQueue{}, stubCircuit{}), fakeRouterVerifier{err: errors.New("invalid token")})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/cancel", nil)
	req.Header.Set(internalauth.HeaderName, "any.token")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestNewRouter_PlayerRouteWithValidTokenReachesHandler は verifier を通過した
// リクエストが handler の成功応答まで到達することを確かめる。
func TestNewRouter_PlayerRouteWithValidTokenReachesHandler(t *testing.T) {
	r := NewRouter(New(okQueue{}, stubCircuit{}), fakeRouterVerifier{playerID: testPlayerID})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/enqueue",
		strings.NewReader(`{"deck_id":1,"name":"TST-NAME","level":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(internalauth.HeaderName, "any.token")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusAccepted, w.Code)
}
