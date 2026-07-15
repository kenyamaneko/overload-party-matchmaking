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

func TestClient_EnqueuePlayer(t *testing.T) {
	t.Run("EnqueuePlayer", func(t *testing.T) {
		t.Run("202 を受けたとき、エラーにならない", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.EnqueueFn = func(_ apimatchmaking.EnqueueRequest) (int, any) { return http.StatusAccepted, nil }

			c := newTestClient(t, srv.URL())
			err := c.EnqueuePlayer(context.Background(), apimatchmaking.EnqueueRequest{})
			require.NoError(t, err)
		})

		cases := []struct {
			name       string
			status     int
			wantTarget error
		}{
			{
				name:       "400 を受けたとき、ErrBadRequest になる",
				status:     http.StatusBadRequest,
				wantTarget: apimatchmakingclient.ErrBadRequest,
			},
			{
				name:       "401 を受けたとき、ErrUnauthorized になる",
				status:     http.StatusUnauthorized,
				wantTarget: apimatchmakingclient.ErrUnauthorized,
			},
			{
				name:       "403 を受けたとき、ErrForbidden になる",
				status:     http.StatusForbidden,
				wantTarget: apimatchmakingclient.ErrForbidden,
			},
			{
				name:       "503 を受けたとき、ErrServiceUnavailable になる",
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
				err := c.EnqueuePlayer(context.Background(), apimatchmaking.EnqueueRequest{})
				assertSentinel(t, err, tc.wantTarget)
			})
		}
	})
}

func TestClient_CancelPlayer(t *testing.T) {
	t.Run("CancelPlayer", func(t *testing.T) {
		t.Run("200 を受けたとき、エラーにならない", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.CancelFn = func() (int, any) { return http.StatusOK, nil }

			c := newTestClient(t, srv.URL())
			err := c.CancelPlayer(context.Background())
			require.NoError(t, err)
		})

		cases := []struct {
			name       string
			status     int
			wantTarget error
		}{
			{
				name:       "401 を受けたとき、ErrUnauthorized になる",
				status:     http.StatusUnauthorized,
				wantTarget: apimatchmakingclient.ErrUnauthorized,
			},
			{
				name:       "404 を受けたとき、ErrNotFound になる",
				status:     http.StatusNotFound,
				wantTarget: apimatchmakingclient.ErrNotFound,
			},
			{
				name:       "500 を受けたとき、ErrInternalServer になる",
				status:     http.StatusInternalServerError,
				wantTarget: apimatchmakingclient.ErrInternalServer,
			},
			{
				name:       "503 を受けたとき、ErrServiceUnavailable になる",
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
	})
}

func TestClient_GetQueueSize(t *testing.T) {
	t.Run("GetQueueSize", func(t *testing.T) {
		t.Run("401 を受けたとき、ErrUnauthorized になる", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.QueueSizeFn = func() (int, any) { return http.StatusUnauthorized, nil }

			c := newTestClient(t, srv.URL())
			_, err := c.GetQueueSize(context.Background())
			assertSentinel(t, err, apimatchmakingclient.ErrUnauthorized)
		})

		t.Run("503 を受けたとき、ErrServiceUnavailable になる", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.QueueSizeFn = func() (int, any) { return http.StatusServiceUnavailable, nil }

			c := newTestClient(t, srv.URL())
			_, err := c.GetQueueSize(context.Background())
			assertSentinel(t, err, apimatchmakingclient.ErrServiceUnavailable)
		})

		t.Run("仕様に無い status (418) を受けたとき、既知のエラーのいずれにもならない", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.QueueSizeFn = func() (int, any) { return http.StatusTeapot, nil }

			c := newTestClient(t, srv.URL())
			_, err := c.GetQueueSize(context.Background())

			require.Error(t, err)
			assert.NotErrorIs(t, err, apimatchmakingclient.ErrBadRequest)
			assert.NotErrorIs(t, err, apimatchmakingclient.ErrUnauthorized)
			assert.NotErrorIs(t, err, apimatchmakingclient.ErrForbidden)
			assert.NotErrorIs(t, err, apimatchmakingclient.ErrNotFound)
			assert.NotErrorIs(t, err, apimatchmakingclient.ErrServiceUnavailable)
			assert.NotErrorIs(t, err, apimatchmakingclient.ErrInternalServer)
		})
	})
}

func TestClient_GetHealth(t *testing.T) {
	t.Run("GetHealth", func(t *testing.T) {
		cases := []struct {
			name       string
			status     int
			wantTarget error
		}{
			{
				name:       "401 を受けたとき、ErrUnauthorized になる",
				status:     http.StatusUnauthorized,
				wantTarget: apimatchmakingclient.ErrUnauthorized,
			},
			{
				name:       "403 を受けたとき、ErrForbidden になる",
				status:     http.StatusForbidden,
				wantTarget: apimatchmakingclient.ErrForbidden,
			},
			{
				name:       "503 を受けたとき、ErrServiceUnavailable になる",
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
	})
}

func TestClient_RequestEditor(t *testing.T) {
	t.Run("リクエストエディタの適用", func(t *testing.T) {
		t.Run("設定したヘッダが送信先の全リクエストに付与される", func(t *testing.T) {
			// X-Internal-Auth header 注入の接続点として SDK が機能することを担保する。
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
		})
	})
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
