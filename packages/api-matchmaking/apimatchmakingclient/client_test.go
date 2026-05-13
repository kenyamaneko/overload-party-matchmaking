package apimatchmakingclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
	"github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking/apimatchmakingclient"
	"github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking/apimatchmakingserverfake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 以下の TestClient_<Endpoint>_StatusMapping 群は、SDK の固有責務である
// 「OpenAPI spec で宣言された 4xx/5xx status を sentinel error に変換する」契約を
// endpoint ごとに検証する。

func TestClient_EnqueuePlayer_StatusMapping(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		wantTarget error
	}{
		{
			name:       "400 を受けたとき ErrBadRequest",
			status:     http.StatusBadRequest,
			wantTarget: apimatchmakingclient.ErrBadRequest,
		},
		{
			name:       "401 を受けたとき ErrUnauthorized",
			status:     http.StatusUnauthorized,
			wantTarget: apimatchmakingclient.ErrUnauthorized,
		},
		{
			name:       "503 を受けたとき ErrServiceUnavailable",
			status:     http.StatusServiceUnavailable,
			wantTarget: apimatchmakingclient.ErrServiceUnavailable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.EnqueueFn = func(_ apimatchmaking.EnqueueRequest) (int, any) { return tc.status, nil }

			c := newTestClient(t, srv.URL())
			// status mapping 検証のため request body の内容は無関係 (server fake は body を見ず tc.status を返す)。
			err := c.EnqueuePlayer(context.Background(), apimatchmaking.EnqueueRequest{})
			assertSentinel(t, err, tc.wantTarget)
		})
	}
}

func TestClient_CancelPlayer_StatusMapping(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		wantTarget error
	}{
		{
			name:       "401 を受けたとき ErrUnauthorized",
			status:     http.StatusUnauthorized,
			wantTarget: apimatchmakingclient.ErrUnauthorized,
		},
		{
			name:       "404 を受けたとき ErrNotFound",
			status:     http.StatusNotFound,
			wantTarget: apimatchmakingclient.ErrNotFound,
		},
		{
			name:       "503 を受けたとき ErrServiceUnavailable",
			status:     http.StatusServiceUnavailable,
			wantTarget: apimatchmakingclient.ErrServiceUnavailable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.CancelFn = func() (int, any) { return tc.status, nil }

			c := newTestClient(t, srv.URL())
			err := c.CancelPlayer(context.Background())
			assertSentinel(t, err, tc.wantTarget)
		})
	}
}

func TestClient_GetQueueSize_StatusMapping(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		wantTarget error
	}{
		{
			name:       "503 を受けたとき ErrServiceUnavailable",
			status:     http.StatusServiceUnavailable,
			wantTarget: apimatchmakingclient.ErrServiceUnavailable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.QueueSizeFn = func() (int, any) { return tc.status, nil }

			c := newTestClient(t, srv.URL())
			_, err := c.GetQueueSize(context.Background())
			assertSentinel(t, err, tc.wantTarget)
		})
	}
}

func TestClient_GetHealth_StatusMapping(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		wantTarget error
	}{
		{
			name:       "503 を受けたとき ErrServiceUnavailable",
			status:     http.StatusServiceUnavailable,
			wantTarget: apimatchmakingclient.ErrServiceUnavailable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.HealthFn = func() (int, any) { return tc.status, nil }

			c := newTestClient(t, srv.URL())
			_, err := c.GetHealth(context.Background())
			assertSentinel(t, err, tc.wantTarget)
		})
	}
}

// TestClient_RequestEditor は Option pattern の契約 (WithRequestEditorFn で渡した
// editor が全リクエストに適用される) を検証する。X-Internal-Auth header 注入の
// 接続点として SDK が機能することを担保する。
func TestClient_RequestEditor(t *testing.T) {
	var gotHeader string
	spy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Internal-Auth")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"queue_size":0}`))
	}))
	defer spy.Close()

	c, err := apimatchmakingclient.New(spy.URL,
		apimatchmakingclient.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("X-Internal-Auth", "test-token")
			return nil
		}),
	)
	require.NoError(t, err)

	_, err = c.GetQueueSize(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-token", gotHeader)
}

func newTestClient(t *testing.T, baseURL string) *apimatchmakingclient.Client {
	t.Helper()
	c, err := apimatchmakingclient.New(baseURL)
	require.NoError(t, err)
	return c
}

func assertSentinel(t *testing.T, gotErr, wantTarget error) {
	t.Helper()
	require.Error(t, gotErr)
	assert.ErrorIs(t, gotErr, wantTarget)
}
