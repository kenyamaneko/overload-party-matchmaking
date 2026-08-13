package apimatchmakingclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
	"github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking/apimatchmakingclient"
	"github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking/apimatchmakingserverfake"
)

func newTestClient(t *testing.T, baseURL string) *apimatchmakingclient.Client {
	t.Helper()
	c, err := apimatchmakingclient.New(baseURL)
	require.NoError(t, err)
	return c
}

func newUnreachableClient(t *testing.T) *apimatchmakingclient.Client {
	t.Helper()
	srv := apimatchmakingserverfake.NewServer()
	baseURL := srv.URL()
	srv.Close()
	return newTestClient(t, baseURL)
}

func extractRequestPath(t *testing.T, build func(server string) (*http.Request, error)) string {
	t.Helper()
	req, err := build("http://placeholder")
	require.NoError(t, err)
	return req.URL.Path
}

func TestClientStatusError(t *testing.T) {
	t.Run("ステータスコードに応じたエラーへの変換", func(t *testing.T) {
		statusErrorCases := []struct {
			name    string
			status  int
			wantErr error
		}{
			{name: "400を受けると、ErrBadRequestになる", status: http.StatusBadRequest, wantErr: apimatchmakingclient.ErrBadRequest},
			{name: "401を受けると、ErrUnauthorizedになる", status: http.StatusUnauthorized, wantErr: apimatchmakingclient.ErrUnauthorized},
			{name: "403を受けると、ErrForbiddenになる", status: http.StatusForbidden, wantErr: apimatchmakingclient.ErrForbidden},
			{name: "404を受けると、ErrNotFoundになる", status: http.StatusNotFound, wantErr: apimatchmakingclient.ErrNotFound},
			{name: "503を受けると、ErrServiceUnavailableになる", status: http.StatusServiceUnavailable, wantErr: apimatchmakingclient.ErrServiceUnavailable},
			{name: "500 (503以外の5xx) を受けると、ErrInternalServerになる", status: http.StatusInternalServerError, wantErr: apimatchmakingclient.ErrInternalServer},
		}
		for _, tt := range statusErrorCases {
			t.Run(tt.name, func(t *testing.T) {
				srv := apimatchmakingserverfake.NewServer()
				defer srv.Close()
				srv.QueueSizeFn = func() (int, any) { return tt.status, nil }
				c := newTestClient(t, srv.URL())

				_, err := c.GetQueueSize(context.Background())

				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			})
		}

		t.Run("418を受けると、errorになりエラーメッセージに418という文字列が含まれるが、ErrBadRequest・ErrUnauthorized・ErrForbidden・ErrNotFound・ErrServiceUnavailable・ErrInternalServerのいずれにも一致しない", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.QueueSizeFn = func() (int, any) { return http.StatusTeapot, nil }
			c := newTestClient(t, srv.URL())

			_, err := c.GetQueueSize(context.Background())

			require.Error(t, err)
			assert.Contains(t, err.Error(), "418")
			assert.NotErrorIs(t, err, apimatchmakingclient.ErrBadRequest)
			assert.NotErrorIs(t, err, apimatchmakingclient.ErrUnauthorized)
			assert.NotErrorIs(t, err, apimatchmakingclient.ErrForbidden)
			assert.NotErrorIs(t, err, apimatchmakingclient.ErrNotFound)
			assert.NotErrorIs(t, err, apimatchmakingclient.ErrServiceUnavailable)
			assert.NotErrorIs(t, err, apimatchmakingclient.ErrInternalServer)
		})
	})
}

func TestGetHealth(t *testing.T) {
	t.Run("死活監視状態の取得", func(t *testing.T) {
		t.Run("サーバが正常応答を返すと、戻り値のステータスにはok、サーキットにはclosedが返る", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.HealthFn = func() (int, any) {
				return http.StatusOK, apimatchmaking.HealthResponse{Status: "ok", Circuit: "closed"}
			}
			c := newTestClient(t, srv.URL())

			got, err := c.GetHealth(context.Background())

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, "ok", got.Status)
			assert.Equal(t, "closed", got.Circuit)
		})

		t.Run("エラー応答(401)を受けると、errorになる", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.HealthFn = func() (int, any) { return http.StatusUnauthorized, nil }
			c := newTestClient(t, srv.URL())

			_, err := c.GetHealth(context.Background())

			assert.Error(t, err)
		})

		t.Run("接続先に到達できないとき、errorになりエラーメッセージにGetHealthという文字列が含まれ、戻り値はnilになる", func(t *testing.T) {
			c := newUnreachableClient(t)

			got, err := c.GetHealth(context.Background())

			require.Error(t, err)
			assert.Contains(t, err.Error(), "GetHealth")
			assert.Nil(t, got)
		})
	})
}

func TestGetQueueSize(t *testing.T) {
	t.Run("マッチメイキング待機人数の取得", func(t *testing.T) {
		t.Run("待機人数の応答を受けると、戻り値の待機人数としてその値が返る", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.QueueSizeFn = func() (int, any) {
				return http.StatusOK, apimatchmaking.QueueSizeResponse{Size: 3}
			}
			c := newTestClient(t, srv.URL())

			got, err := c.GetQueueSize(context.Background())

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, int64(3), got.Size)
		})

		t.Run("接続先に到達できないとき、errorになりエラーメッセージにGetQueueSizeという文字列が含まれ、戻り値はnilになる", func(t *testing.T) {
			c := newUnreachableClient(t)

			got, err := c.GetQueueSize(context.Background())

			require.Error(t, err)
			assert.Contains(t, err.Error(), "GetQueueSize")
			assert.Nil(t, got)
		})
	})
}

func TestEnqueuePlayer(t *testing.T) {
	t.Run("マッチメイキングキューへの登録", func(t *testing.T) {
		validRequest := apimatchmaking.EnqueueRequest{
			DeckID:            1,
			Name:              "Alice",
			Level:             1,
			GatewayInstanceID: "gw-1",
		}

		t.Run("202を受けると、errorにならない", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.EnqueueFn = func(_ apimatchmaking.EnqueueRequest) (int, any) { return http.StatusAccepted, nil }
			c := newTestClient(t, srv.URL())

			err := c.EnqueuePlayer(context.Background(), validRequest)

			assert.NoError(t, err)
		})

		t.Run("エラー応答(400)を受けると、errorになる", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.EnqueueFn = func(_ apimatchmaking.EnqueueRequest) (int, any) { return http.StatusBadRequest, nil }
			c := newTestClient(t, srv.URL())

			err := c.EnqueuePlayer(context.Background(), validRequest)

			assert.Error(t, err)
		})

		t.Run("接続先に到達できないとき、errorになりエラーメッセージにEnqueuePlayerという文字列が含まれる", func(t *testing.T) {
			c := newUnreachableClient(t)

			err := c.EnqueuePlayer(context.Background(), validRequest)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "EnqueuePlayer")
		})
	})
}

func TestCancelPlayer(t *testing.T) {
	t.Run("マッチメイキングキューからの取消", func(t *testing.T) {
		t.Run("200を受けると、errorにならない", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.CancelFn = func() (int, any) { return http.StatusOK, nil }
			c := newTestClient(t, srv.URL())

			err := c.CancelPlayer(context.Background())

			assert.NoError(t, err)
		})

		t.Run("エラー応答(404)を受けると、errorになる", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.CancelFn = func() (int, any) { return http.StatusNotFound, nil }
			c := newTestClient(t, srv.URL())

			err := c.CancelPlayer(context.Background())

			assert.Error(t, err)
		})

		t.Run("接続先に到達できないとき、errorになりエラーメッセージにCancelPlayerという文字列が含まれる", func(t *testing.T) {
			c := newUnreachableClient(t)

			err := c.CancelPlayer(context.Background())

			require.Error(t, err)
			assert.Contains(t, err.Error(), "CancelPlayer")
		})
	})
}

func TestReportMatchAbandoned(t *testing.T) {
	t.Run("マッチ不成立の申告", func(t *testing.T) {
		validRequest := apimatchmaking.MatchAbandonedRequest{
			MatchID:   "mch_1",
			PlayerIDs: []string{"p1", "p2"},
			Reason:    apimatchmaking.MatchAbandonedRequestReasonPlayerNotConnected,
		}

		t.Run("204を受けると、errorにならない", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.MatchAbandonedFn = func(_ apimatchmaking.MatchAbandonedRequest) (int, any) { return http.StatusNoContent, nil }
			c := newTestClient(t, srv.URL())

			err := c.ReportMatchAbandoned(context.Background(), validRequest)

			assert.NoError(t, err)
		})

		t.Run("エラー応答(400)を受けると、errorになる", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.MatchAbandonedFn = func(_ apimatchmaking.MatchAbandonedRequest) (int, any) { return http.StatusBadRequest, nil }
			c := newTestClient(t, srv.URL())

			err := c.ReportMatchAbandoned(context.Background(), validRequest)

			assert.Error(t, err)
		})

		t.Run("接続先に到達できないとき、errorになりエラーメッセージにReportMatchAbandonedという文字列が含まれる", func(t *testing.T) {
			c := newUnreachableClient(t)

			err := c.ReportMatchAbandoned(context.Background(), validRequest)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "ReportMatchAbandoned")
		})
	})
}

func TestWithRequestEditorFn(t *testing.T) {
	t.Run("リクエストエディタの適用", func(t *testing.T) {
		t.Run("ヘッダを付与する処理を設定すると、サーバが受信した全リクエストにそのヘッダが付与されている", func(t *testing.T) {
			healthPath := extractRequestPath(t, apimatchmaking.NewGetHealthRequest)
			queueSizePath := extractRequestPath(t, apimatchmaking.NewGetQueueSizeRequest)

			var mu sync.Mutex
			var receivedHeaders []http.Header
			spy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				receivedHeaders = append(receivedHeaders, r.Header.Clone())
				mu.Unlock()

				switch r.URL.Path {
				case healthPath:
					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(apimatchmaking.HealthResponse{Status: "ok", Circuit: "closed"}); err != nil {
						t.Errorf("failed to encode health response: %v", err)
					}
				case queueSizePath:
					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(apimatchmaking.QueueSizeResponse{Size: 0}); err != nil {
						t.Errorf("failed to encode queue size response: %v", err)
					}
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer spy.Close()

			c, err := apimatchmakingclient.New(
				spy.URL,
				apimatchmakingclient.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
					req.Header.Set("X-Test-Header", "injected")
					return nil
				}),
			)
			require.NoError(t, err)

			_, err = c.GetHealth(context.Background())
			require.NoError(t, err)
			_, err = c.GetQueueSize(context.Background())
			require.NoError(t, err)

			mu.Lock()
			defer mu.Unlock()
			require.Len(t, receivedHeaders, 2)
			for _, h := range receivedHeaders {
				assert.Equal(t, "injected", h.Get("X-Test-Header"))
			}
		})
	})
}
