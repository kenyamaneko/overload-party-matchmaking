package httphandler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/abandon"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

// newTestServer は NewRouter を実サーバとして起動する。認証配線 (X-Internal-Auth の
// 要否) の検証が目的で、検証ロジック自体 (internalauth.Verifier) は対象としないため
// MockVerifier を渡す。
func newTestServer(t *testing.T, verifier internalauth.Verifier) *httptest.Server {
	t.Helper()
	q := newFakeQueue()
	h := httphandler.New(q, &fakeCircuit{}, abandon.New(q))
	r := httphandler.NewRouter(h, verifier)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func doRequest(t *testing.T, method, url, token string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set(internalauth.HeaderName, token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}

func TestRouterAuth(t *testing.T) {
	t.Run("認証の要否", func(t *testing.T) {
		t.Run("X-Internal-Authヘッダを付けずに登録エンドポイントを呼び出すと、401になり応答本文にヘッダが必須であることを示すメッセージが入る", func(t *testing.T) {
			verifier := &internalauth.MockVerifier{}
			srv := newTestServer(t, verifier)

			resp := doRequest(t, http.MethodPost, srv.URL+"/internal/v1/enqueue", "", []byte(`{}`))

			require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			assert.Contains(t, readBody(t, resp), "required")
		})

		t.Run("X-Internal-Authヘッダを付けずに取消エンドポイントを呼び出すと、401になる", func(t *testing.T) {
			verifier := &internalauth.MockVerifier{}
			srv := newTestServer(t, verifier)

			resp := doRequest(t, http.MethodPost, srv.URL+"/internal/v1/cancel", "", nil)

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		})

		t.Run("X-Internal-Authヘッダの値が検証できないトークンの状態で登録エンドポイントを呼び出すと、401になり応答本文にトークンが無効であることを示すメッセージが入る", func(t *testing.T) {
			verifier := &internalauth.MockVerifier{
				VerifyFn: func(string) (string, error) { return "", errors.New("invalid signature") },
			}
			srv := newTestServer(t, verifier)

			resp := doRequest(t, http.MethodPost, srv.URL+"/internal/v1/enqueue", "bad-token", []byte(`{}`))

			require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			assert.Contains(t, readBody(t, resp), "invalid")
		})

		t.Run("X-Internal-Authヘッダを付けずにキューサイズ取得エンドポイントを呼び出すと、200になる", func(t *testing.T) {
			verifier := &internalauth.MockVerifier{}
			srv := newTestServer(t, verifier)

			resp := doRequest(t, http.MethodGet, srv.URL+"/internal/v1/queue-size", "", nil)

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})

		t.Run("X-Internal-Authヘッダを付けずにhealthエンドポイントを呼び出すと、200になる", func(t *testing.T) {
			verifier := &internalauth.MockVerifier{}
			srv := newTestServer(t, verifier)

			resp := doRequest(t, http.MethodGet, srv.URL+"/internal/v1/health", "", nil)

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})

		t.Run("X-Internal-Authヘッダを付けずに、有効な内容で不成立申告エンドポイントを呼び出すと、204になる", func(t *testing.T) {
			verifier := &internalauth.MockVerifier{}
			srv := newTestServer(t, verifier)
			body, err := json.Marshal(apimatchmaking.MatchAbandonedRequest{
				MatchID:   "mch_1",
				PlayerIDs: []string{"p1", "p2"},
				Reason:    apimatchmaking.MatchAbandonedRequestReasonPlayerNotConnected,
			})
			require.NoError(t, err)

			resp := doRequest(t, http.MethodPost, srv.URL+"/internal/v1/match-abandoned", "", body)

			assert.Equal(t, http.StatusNoContent, resp.StatusCode)
		})
	})
}
