package apimatchmakingclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
	"github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking/apimatchmakingclient"
)

type receivedRequest struct {
	method string
	path   string
}

type upstream struct {
	server   *httptest.Server
	mu       sync.Mutex
	received []receivedRequest
}

func newUpstream(t *testing.T, statuses ...int) *upstream {
	t.Helper()
	u := &upstream{}
	u.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		defer u.mu.Unlock()
		index := min(len(u.received), len(statuses)-1)
		u.received = append(u.received, receivedRequest{method: r.Method, path: r.URL.Path})
		w.WriteHeader(statuses[index])
	}))
	t.Cleanup(u.server.Close)
	return u
}

func (u *upstream) receivedRequests() []receivedRequest {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]receivedRequest(nil), u.received...)
}

func newStoppedUpstreamURL() string {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	stoppedURL := server.URL
	server.Close()
	return stoppedURL
}

func newClient(t *testing.T, baseURL string) *apimatchmakingclient.Client {
	t.Helper()
	client, err := apimatchmakingclient.New(baseURL)
	require.NoError(t, err)
	return client
}

type operation func(ctx context.Context, client *apimatchmakingclient.Client) error

func enqueuePlayer(ctx context.Context, client *apimatchmakingclient.Client) error {
	return client.EnqueuePlayer(ctx, apimatchmaking.EnqueueRequest{
		DeckID:            1,
		GatewayInstanceID: "TST-GATEWAY-1",
		Level:             1,
		Name:              "TST-PLAYER-NAME-1",
	})
}

func cancelPlayer(ctx context.Context, client *apimatchmakingclient.Client) error {
	return client.CancelPlayer(ctx)
}

func reportMatchAbandoned(ctx context.Context, client *apimatchmakingclient.Client) error {
	return client.ReportMatchAbandoned(ctx, apimatchmaking.MatchAbandonedRequest{
		MatchID:   "TST-MATCH-1",
		PlayerIDs: []string{"TST-PLAYER-1", "TST-PLAYER-2"},
		Reason:    apimatchmaking.MatchAbandonedRequestReasonPlayerNotConnected,
	})
}

func callOperation(t *testing.T, call operation, statuses ...int) (*upstream, error) {
	t.Helper()
	up := newUpstream(t, statuses...)
	return up, call(t.Context(), newClient(t, up.server.URL))
}

func TestResponseStatusClassification(t *testing.T) {
	t.Run("[マッチメイキングAPI]レスポンスステータスの分類", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("プレイヤーの追加", func(t *testing.T) {
				t.Run("ステータスが400・401・403・404・503で本文なしのとき", func(t *testing.T) {
					tests := []struct {
						name    string
						status  int
						wantErr error
					}{
						{
							name:    "ステータスが400のとき、操作は、リクエストが不正であることを示すエラーを返す",
							status:  400,
							wantErr: apimatchmakingclient.ErrBadRequest,
						},
						{
							name:    "ステータスが401のとき、操作は、認証に失敗したことを示すエラーを返す",
							status:  401,
							wantErr: apimatchmakingclient.ErrUnauthorized,
						},
						{
							name:    "ステータスが403のとき、操作は、権限が無いことを示すエラーを返す",
							status:  403,
							wantErr: apimatchmakingclient.ErrForbidden,
						},
						{
							name:    "ステータスが404のとき、操作は、対象が見つからないことを示すエラーを返す",
							status:  404,
							wantErr: apimatchmakingclient.ErrNotFound,
						},
						{
							name:    "ステータスが503のとき、操作は、サービスが利用できないことを示すエラーを返す",
							status:  503,
							wantErr: apimatchmakingclient.ErrServiceUnavailable,
						},
					}
					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							_, err := callOperation(t, enqueuePlayer, tt.status)

							assert.ErrorIs(t, err, tt.wantErr)
						})
					}
				})

				t.Run("ステータスが503以外の500以上で本文なしのとき", func(t *testing.T) {
					tests := []struct {
						name   string
						status int
					}{
						{name: "ステータスが500のとき、操作は、サーバー内部で障害が起きたことを示すエラーを返す", status: 500},
						{name: "ステータスが502のとき、操作は、サーバー内部で障害が起きたことを示すエラーを返す", status: 502},
						{name: "ステータスが504のとき、操作は、サーバー内部で障害が起きたことを示すエラーを返す", status: 504},
						{name: "ステータスが599のとき、操作は、サーバー内部で障害が起きたことを示すエラーを返す", status: 599},
					}
					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							_, err := callOperation(t, enqueuePlayer, tt.status)

							assert.ErrorIs(t, err, apimatchmakingclient.ErrInternalServer)
						})
					}
				})

				t.Run("ステータスが3xxか、400・401・403・404以外の4xxで、本文なしのとき", func(t *testing.T) {
					tests := []struct {
						name   string
						status int
						want   string
					}{
						{name: "ステータスが302のとき、操作は、想定外のレスポンスを受け取ったことを示すエラーを返す", status: 302, want: "unexpected status 302"},
						{name: "ステータスが405のとき、操作は、想定外のレスポンスを受け取ったことを示すエラーを返す", status: 405, want: "unexpected status 405"},
						{name: "ステータスが409のとき、操作は、想定外のレスポンスを受け取ったことを示すエラーを返す", status: 409, want: "unexpected status 409"},
						{name: "ステータスが429のとき、操作は、想定外のレスポンスを受け取ったことを示すエラーを返す", status: 429, want: "unexpected status 429"},
						{name: "ステータスが499のとき、操作は、想定外のレスポンスを受け取ったことを示すエラーを返す", status: 499, want: "unexpected status 499"},
					}
					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							_, err := callOperation(t, enqueuePlayer, tt.status)

							assert.ErrorContains(t, err, tt.want)
						})
					}
				})

				t.Run("ステータス503で本文なしのレスポンスで失敗したあと、続けてプレイヤーの追加を呼び出し、ステータス202のレスポンスを受け取ると、操作はエラーを返さない", func(t *testing.T) {
					up := newUpstream(t, 503, 202)
					client := newClient(t, up.server.URL)

					firstErr := enqueuePlayer(t.Context(), client)
					secondErr := enqueuePlayer(t.Context(), client)

					require.ErrorIs(t, firstErr, apimatchmakingclient.ErrServiceUnavailable)
					assert.NoError(t, secondErr)
				})
			})
		})
	})
}

func TestNewClient(t *testing.T) {
	t.Run("[マッチメイキングAPI]クライアントの生成", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("ベースURLが空文字のとき、クライアントの生成は、ベースURLが空であることを示すエラーを返す", func(t *testing.T) {
				_, err := apimatchmakingclient.New("")

				assert.ErrorContains(t, err, "base URL is empty")
			})
		})
	})
}

func TestEnqueuePlayer(t *testing.T) {
	t.Run("[マッチメイキングキュー]プレイヤーの追加", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("接続先がステータス202のレスポンスを返すとき、操作はエラーを返さない", func(t *testing.T) {
				_, err := callOperation(t, enqueuePlayer, 202)

				assert.NoError(t, err)
			})

			t.Run("操作を呼び出したとき", func(t *testing.T) {
				t.Run("操作が接続先に送ったリクエストのメソッドがPOSTになる", func(t *testing.T) {
					up, _ := callOperation(t, enqueuePlayer, 202)

					received := up.receivedRequests()
					require.Len(t, received, 1)
					assert.Equal(t, "POST", received[0].method)
				})

				t.Run("操作が接続先に送ったリクエストのパスが/internal/v1/enqueueになる", func(t *testing.T) {
					up, _ := callOperation(t, enqueuePlayer, 202)

					received := up.receivedRequests()
					require.Len(t, received, 1)
					assert.Equal(t, "/internal/v1/enqueue", received[0].path)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("接続先が最初のリクエストにステータス503、2回目以降のリクエストにステータス202を返すとき、操作は、再送せずに、サービスが利用できないことを示すエラーを返す", func(t *testing.T) {
				_, err := callOperation(t, enqueuePlayer, 503, 202)

				assert.ErrorIs(t, err, apimatchmakingclient.ErrServiceUnavailable)
			})

			t.Run("ステータスが202以外の2xxのとき", func(t *testing.T) {
				tests := []struct {
					name   string
					status int
					want   string
				}{
					{name: "ステータスが200のとき、操作は、想定外のレスポンスを受け取ったことを示すエラーを返す", status: 200, want: "unexpected status 200"},
					{name: "ステータスが204のとき、操作は、想定外のレスポンスを受け取ったことを示すエラーを返す", status: 204, want: "unexpected status 204"},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						_, err := callOperation(t, enqueuePlayer, tt.status)

						assert.ErrorContains(t, err, tt.want)
					})
				}
			})

			t.Run("接続先が停止しているとき、操作は、接続できなかったことを示すエラーを返す", func(t *testing.T) {
				err := enqueuePlayer(t.Context(), newClient(t, newStoppedUpstreamURL()))

				var urlErr *url.Error
				assert.ErrorAs(t, err, &urlErr)
			})
		})
	})
}

func TestCancelPlayer(t *testing.T) {
	t.Run("[マッチメイキングキュー]プレイヤーの除外", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("接続先がステータス200のレスポンスを返すとき、操作はエラーを返さない", func(t *testing.T) {
				_, err := callOperation(t, cancelPlayer, 200)

				assert.NoError(t, err)
			})

			t.Run("操作を呼び出したとき", func(t *testing.T) {
				t.Run("操作が接続先に送ったリクエストのメソッドがPOSTになる", func(t *testing.T) {
					up, _ := callOperation(t, cancelPlayer, 200)

					received := up.receivedRequests()
					require.Len(t, received, 1)
					assert.Equal(t, "POST", received[0].method)
				})

				t.Run("操作が接続先に送ったリクエストのパスが/internal/v1/cancelになる", func(t *testing.T) {
					up, _ := callOperation(t, cancelPlayer, 200)

					received := up.receivedRequests()
					require.Len(t, received, 1)
					assert.Equal(t, "/internal/v1/cancel", received[0].path)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("接続先が最初のリクエストにステータス503、2回目以降のリクエストにステータス200を返すとき、操作は、再送せずに、サービスが利用できないことを示すエラーを返す", func(t *testing.T) {
				_, err := callOperation(t, cancelPlayer, 503, 200)

				assert.ErrorIs(t, err, apimatchmakingclient.ErrServiceUnavailable)
			})

			t.Run("ステータスが200以外の2xxのとき", func(t *testing.T) {
				tests := []struct {
					name   string
					status int
					want   string
				}{
					{name: "ステータスが202のとき、操作は、想定外のレスポンスを受け取ったことを示すエラーを返す", status: 202, want: "unexpected status 202"},
					{name: "ステータスが204のとき、操作は、想定外のレスポンスを受け取ったことを示すエラーを返す", status: 204, want: "unexpected status 204"},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						_, err := callOperation(t, cancelPlayer, tt.status)

						assert.ErrorContains(t, err, tt.want)
					})
				}
			})

			t.Run("接続先が停止しているとき、操作は、接続できなかったことを示すエラーを返す", func(t *testing.T) {
				err := cancelPlayer(t.Context(), newClient(t, newStoppedUpstreamURL()))

				var urlErr *url.Error
				assert.ErrorAs(t, err, &urlErr)
			})
		})
	})
}

func TestReportMatchAbandoned(t *testing.T) {
	t.Run("[マッチ不成立]成立したマッチの破棄の申告", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("接続先がステータス204のレスポンスを返すとき、操作はエラーを返さない", func(t *testing.T) {
				_, err := callOperation(t, reportMatchAbandoned, 204)

				assert.NoError(t, err)
			})

			t.Run("操作を呼び出したとき", func(t *testing.T) {
				t.Run("操作が接続先に送ったリクエストのメソッドがPOSTになる", func(t *testing.T) {
					up, _ := callOperation(t, reportMatchAbandoned, 204)

					received := up.receivedRequests()
					require.Len(t, received, 1)
					assert.Equal(t, "POST", received[0].method)
				})

				t.Run("操作が接続先に送ったリクエストのパスが/internal/v1/match-abandonedになる", func(t *testing.T) {
					up, _ := callOperation(t, reportMatchAbandoned, 204)

					received := up.receivedRequests()
					require.Len(t, received, 1)
					assert.Equal(t, "/internal/v1/match-abandoned", received[0].path)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("接続先が最初のリクエストにステータス503、2回目以降のリクエストにステータス204を返すとき、操作は、再送せずに、サービスが利用できないことを示すエラーを返す", func(t *testing.T) {
				_, err := callOperation(t, reportMatchAbandoned, 503, 204)

				assert.ErrorIs(t, err, apimatchmakingclient.ErrServiceUnavailable)
			})

			t.Run("ステータスが204以外の2xxのとき", func(t *testing.T) {
				tests := []struct {
					name   string
					status int
					want   string
				}{
					{name: "ステータスが200のとき、操作は、想定外のレスポンスを受け取ったことを示すエラーを返す", status: 200, want: "unexpected status 200"},
					{name: "ステータスが202のとき、操作は、想定外のレスポンスを受け取ったことを示すエラーを返す", status: 202, want: "unexpected status 202"},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						_, err := callOperation(t, reportMatchAbandoned, tt.status)

						assert.ErrorContains(t, err, tt.want)
					})
				}
			})

			t.Run("接続先が停止しているとき、操作は、接続できなかったことを示すエラーを返す", func(t *testing.T) {
				err := reportMatchAbandoned(t.Context(), newClient(t, newStoppedUpstreamURL()))

				var urlErr *url.Error
				assert.ErrorAs(t, err, &urlErr)
			})
		})
	})
}
