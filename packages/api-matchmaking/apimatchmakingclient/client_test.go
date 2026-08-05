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
		t.Run("202を受けたとき、エラーにならない", func(t *testing.T) {
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
				name:       "400を受けたとき、ErrBadRequestになる",
				status:     http.StatusBadRequest,
				wantTarget: apimatchmakingclient.ErrBadRequest,
			},
			{
				name:       "401を受けたとき、ErrUnauthorizedになる",
				status:     http.StatusUnauthorized,
				wantTarget: apimatchmakingclient.ErrUnauthorized,
			},
			{
				name:       "403を受けたとき、ErrForbiddenになる",
				status:     http.StatusForbidden,
				wantTarget: apimatchmakingclient.ErrForbidden,
			},
			{
				name:       "503を受けたとき、ErrServiceUnavailableになる",
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

		t.Run("接続先に到達できないとき、エラーになり操作名が伝わる", func(t *testing.T) {
			c := newTestClient(t, unreachableURL(t))
			err := c.EnqueuePlayer(context.Background(), apimatchmaking.EnqueueRequest{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "EnqueuePlayer")
		})
	})
}

func TestClient_CancelPlayer(t *testing.T) {
	t.Run("CancelPlayer", func(t *testing.T) {
		t.Run("200を受けたとき、エラーにならない", func(t *testing.T) {
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
				name:       "401を受けたとき、ErrUnauthorizedになる",
				status:     http.StatusUnauthorized,
				wantTarget: apimatchmakingclient.ErrUnauthorized,
			},
			{
				name:       "404を受けたとき、ErrNotFoundになる",
				status:     http.StatusNotFound,
				wantTarget: apimatchmakingclient.ErrNotFound,
			},
			{
				name:       "500を受けたとき、ErrInternalServerになる",
				status:     http.StatusInternalServerError,
				wantTarget: apimatchmakingclient.ErrInternalServer,
			},
			{
				name:       "503を受けたとき、ErrServiceUnavailableになる",
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

		t.Run("接続先に到達できないとき、エラーになり操作名が伝わる", func(t *testing.T) {
			c := newTestClient(t, unreachableURL(t))
			err := c.CancelPlayer(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "CancelPlayer")
		})
	})
}

func TestClient_ReportMatchAbandoned(t *testing.T) {
	t.Run("ReportMatchAbandoned", func(t *testing.T) {
		t.Run("204を受けたとき、エラーにならず、申告した内容が送信先に届く", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			var received apimatchmaking.MatchAbandonedRequest
			srv.MatchAbandonedFn = func(req apimatchmaking.MatchAbandonedRequest) (int, any) {
				received = req
				return http.StatusNoContent, nil
			}

			c := newTestClient(t, srv.URL())
			err := c.ReportMatchAbandoned(context.Background(), apimatchmaking.MatchAbandonedRequest{
				MatchID:   "TST-MATCH-1",
				PlayerIDs: []string{"TST-P1", "TST-P2"},
				Reason:    apimatchmaking.MatchAbandonedRequestReasonPlayerNotConnected,
			})

			require.NoError(t, err)
			assert.Equal(t, "TST-MATCH-1", received.MatchID)
			assert.Equal(t, []string{"TST-P1", "TST-P2"}, received.PlayerIDs)
			assert.Equal(t, apimatchmaking.MatchAbandonedRequestReasonPlayerNotConnected, received.Reason)
		})

		cases := []struct {
			name       string
			status     int
			wantTarget error
		}{
			{
				name:       "400を受けたとき、ErrBadRequestになる",
				status:     http.StatusBadRequest,
				wantTarget: apimatchmakingclient.ErrBadRequest,
			},
			{
				name:       "500を受けたとき、ErrInternalServerになる",
				status:     http.StatusInternalServerError,
				wantTarget: apimatchmakingclient.ErrInternalServer,
			},
			{
				name:       "503を受けたとき、ErrServiceUnavailableになる",
				status:     http.StatusServiceUnavailable,
				wantTarget: apimatchmakingclient.ErrServiceUnavailable,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				srv := apimatchmakingserverfake.NewServer()
				defer srv.Close()
				srv.MatchAbandonedFn = func(_ apimatchmaking.MatchAbandonedRequest) (int, any) { return tc.status, nil }

				c := newTestClient(t, srv.URL())
				err := c.ReportMatchAbandoned(context.Background(), apimatchmaking.MatchAbandonedRequest{})
				assertSentinel(t, err, tc.wantTarget)
			})
		}

		t.Run("接続先に到達できないとき、エラーになり操作名が伝わる", func(t *testing.T) {
			c := newTestClient(t, unreachableURL(t))
			err := c.ReportMatchAbandoned(context.Background(), apimatchmaking.MatchAbandonedRequest{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "ReportMatchAbandoned")
		})
	})
}

func TestClient_GetQueueSize(t *testing.T) {
	t.Run("GetQueueSize", func(t *testing.T) {
		t.Run("待機人数3の応答を受けたとき、待機人数として3が返る", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.QueueSizeFn = func() (int, any) { return http.StatusOK, apimatchmaking.QueueSizeResponse{Size: 3} }

			c := newTestClient(t, srv.URL())
			resp, err := c.GetQueueSize(context.Background())
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, int64(3), resp.Size)
		})

		t.Run("401を受けたとき、ErrUnauthorizedになる", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.QueueSizeFn = func() (int, any) { return http.StatusUnauthorized, nil }

			c := newTestClient(t, srv.URL())
			_, err := c.GetQueueSize(context.Background())
			assertSentinel(t, err, apimatchmakingclient.ErrUnauthorized)
		})

		t.Run("503を受けたとき、ErrServiceUnavailableになる", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.QueueSizeFn = func() (int, any) { return http.StatusServiceUnavailable, nil }

			c := newTestClient(t, srv.URL())
			_, err := c.GetQueueSize(context.Background())
			assertSentinel(t, err, apimatchmakingclient.ErrServiceUnavailable)
		})

		t.Run("仕様に無いstatus (418)を受けたとき、既知のエラーのいずれにもならない", func(t *testing.T) {
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

		t.Run("接続先に到達できないとき、エラーになり操作名が伝わり値は返らない", func(t *testing.T) {
			c := newTestClient(t, unreachableURL(t))
			resp, err := c.GetQueueSize(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "GetQueueSize")
			assert.Nil(t, resp)
		})
	})
}

func TestClient_GetHealth(t *testing.T) {
	t.Run("GetHealth", func(t *testing.T) {
		t.Run("サーバが正常応答を返すとき、status=ok / circuit=closedを返す", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()

			c := newTestClient(t, srv.URL())
			resp, err := c.GetHealth(context.Background())
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, "ok", resp.Status)
			assert.Equal(t, "closed", resp.Circuit)
		})

		cases := []struct {
			name       string
			status     int
			wantTarget error
		}{
			{
				name:       "401を受けたとき、ErrUnauthorizedになる",
				status:     http.StatusUnauthorized,
				wantTarget: apimatchmakingclient.ErrUnauthorized,
			},
			{
				name:       "403を受けたとき、ErrForbiddenになる",
				status:     http.StatusForbidden,
				wantTarget: apimatchmakingclient.ErrForbidden,
			},
			{
				name:       "503を受けたとき、ErrServiceUnavailableになる",
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

		t.Run("接続先に到達できないとき、エラーになり操作名が伝わり値は返らない", func(t *testing.T) {
			c := newTestClient(t, unreachableURL(t))
			resp, err := c.GetHealth(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "GetHealth")
			assert.Nil(t, resp)
		})
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

// unreachableURL は起動直後に閉じた httptest.Server の URL を返し、接続自体の失敗 (connection refused) を発生させる。
func unreachableURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()
	return srv.URL
}

func assertSentinel(t *testing.T, gotErr, wantTarget error) {
	t.Helper()
	require.Error(t, gotErr)
	assert.ErrorIs(t, gotErr, wantTarget)
}
