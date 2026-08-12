package apimatchmakingclient_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
	"github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking/apimatchmakingclient"
	"github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking/apimatchmakingserverfake"
)

type recordingDoer struct {
	client  *http.Client
	headers []http.Header
}

func (d *recordingDoer) Do(req *http.Request) (*http.Response, error) {
	d.headers = append(d.headers, req.Header.Clone())
	return d.client.Do(req)
}

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

		t.Run("仕様に定義のないステータスコード (例: 418) を受けると、errorになるがErrBadRequest・ErrUnauthorized・ErrForbidden・ErrNotFound・ErrServiceUnavailable・ErrInternalServerのいずれにも一致しない", func(t *testing.T) {
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

func TestGetHealth(t *testing.T) {
	t.Run("死活監視状態の取得", func(t *testing.T) {
		t.Run("サーバがstatus:ok・circuit:closedの正常応答を返すと、戻り値にその内容がそのまま入る", func(t *testing.T) {
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

		t.Run("接続先に到達できないとき、errorになりエラーメッセージに操作名GetHealthが含まれ、戻り値はnilになる", func(t *testing.T) {
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
		t.Run("待機人数の応答を受けると、戻り値の待機人数にその値がそのまま入る", func(t *testing.T) {
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

		t.Run("接続先に到達できないとき、errorになりエラーメッセージに操作名GetQueueSizeが含まれ、戻り値はnilになる", func(t *testing.T) {
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

		t.Run("接続先に到達できないとき、errorになりエラーメッセージに操作名EnqueuePlayerが含まれる", func(t *testing.T) {
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

		t.Run("接続先に到達できないとき、errorになりエラーメッセージに操作名CancelPlayerが含まれる", func(t *testing.T) {
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

		t.Run("接続先に到達できないとき、errorになりエラーメッセージに操作名ReportMatchAbandonedが含まれる", func(t *testing.T) {
			c := newUnreachableClient(t)

			err := c.ReportMatchAbandoned(context.Background(), validRequest)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "ReportMatchAbandoned")
		})
	})
}

func TestWithRequestEditorFn(t *testing.T) {
	t.Run("リクエストエディタの適用", func(t *testing.T) {
		t.Run("ヘッダを付与する処理を設定すると、送信される全リクエストにそのヘッダが付与される", func(t *testing.T) {
			srv := apimatchmakingserverfake.NewServer()
			defer srv.Close()
			srv.HealthFn = func() (int, any) {
				return http.StatusOK, apimatchmaking.HealthResponse{Status: "ok", Circuit: "closed"}
			}
			srv.QueueSizeFn = func() (int, any) {
				return http.StatusOK, apimatchmaking.QueueSizeResponse{Size: 0}
			}
			recorder := &recordingDoer{client: &http.Client{}}

			c, err := apimatchmakingclient.New(
				srv.URL(),
				apimatchmakingclient.WithHTTPClient(recorder),
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

			require.Len(t, recorder.headers, 2)
			for _, h := range recorder.headers {
				assert.Equal(t, "injected", h.Get("X-Test-Header"))
			}
		})
	})
}
