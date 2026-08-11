package httphandler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/abandon"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

func init() { gin.SetMode(gin.TestMode) }

const testPlayerID = "player-1"

// newTestEngine はテスト対象の Handler をルーティングする gin.Engine を返す。
// 認証の要否は router_test.go で別途検証するため、ここでは認証済みとみなし player_id を固定で注入する。
func newTestEngine(h *httphandler.Handler) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(internalauth.PlayerIDContextKey, testPlayerID)
		c.Next()
	})
	r.POST("/internal/v1/enqueue", h.Enqueue)
	r.POST("/internal/v1/cancel", h.Cancel)
	r.POST("/internal/v1/match-abandoned", h.ReportMatchAbandoned)
	r.GET("/internal/v1/queue-size", h.RespondQueueSize)
	r.GET("/internal/v1/health", h.RespondHealth)
	return r
}

func doJSONPost(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func doEnqueue(t *testing.T, r *gin.Engine, body apimatchmaking.EnqueueRequest) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return doJSONPost(r, "/internal/v1/enqueue", string(raw))
}

func doCancel(r *gin.Engine) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/cancel", bytes.NewReader(nil))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func doReportMatchAbandoned(t *testing.T, r *gin.Engine, body apimatchmaking.MatchAbandonedRequest) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return doJSONPost(r, "/internal/v1/match-abandoned", string(raw))
}

func newHandler(q *fakeQueue, circuit *fakeCircuit) *httphandler.Handler {
	return httphandler.New(q, circuit, abandon.New(q))
}

func TestEnqueue(t *testing.T) {
	t.Run("登録受付", func(t *testing.T) {
		t.Run("JSONとして解釈できないリクエストボディで登録エンドポイントを呼び出すと、本文が不正であることを示す400になる", func(t *testing.T) {
			r := newTestEngine(newHandler(newFakeQueue(), &fakeCircuit{}))

			w := doJSONPost(r, "/internal/v1/enqueue", "{not json")

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), "request body")
		})

		t.Run("デッキIDが0のリクエストボディで登録エンドポイントを呼び出すと、デッキが指定されていないことを示す400になる", func(t *testing.T) {
			r := newTestEngine(newHandler(newFakeQueue(), &fakeCircuit{}))

			w := doEnqueue(t, r, apimatchmaking.EnqueueRequest{DeckID: 0, Name: "Alice", Level: 3, GatewayInstanceID: "gw-1"})

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), "deck_id")
		})

		t.Run("名前が空文字のリクエストボディで登録エンドポイントを呼び出すと、名前が指定されていないことを示す400になる", func(t *testing.T) {
			r := newTestEngine(newHandler(newFakeQueue(), &fakeCircuit{}))

			w := doEnqueue(t, r, apimatchmaking.EnqueueRequest{DeckID: 42, Name: "", Level: 3, GatewayInstanceID: "gw-1"})

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), "name")
		})

		t.Run("gatewayインスタンス識別子が空文字のリクエストボディで登録エンドポイントを呼び出すと、gatewayインスタンス識別子が指定されていないことを示す400になる", func(t *testing.T) {
			r := newTestEngine(newHandler(newFakeQueue(), &fakeCircuit{}))

			w := doEnqueue(t, r, apimatchmaking.EnqueueRequest{DeckID: 42, Name: "Alice", Level: 3, GatewayInstanceID: ""})

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), "gateway_instance_id")
		})

		t.Run("デッキIDが0でなく名前とgatewayインスタンス識別子がいずれも空文字でないリクエストボディで登録エンドポイントを呼び出すと、マッチメイキングキューには登録したデッキID・名前がそのまま入る", func(t *testing.T) {
			q := newFakeQueue()
			r := newTestEngine(newHandler(q, &fakeCircuit{}))

			doEnqueue(t, r, apimatchmaking.EnqueueRequest{DeckID: 42, Name: "Alice", Level: 3, GatewayInstanceID: "gw-1"})

			entry, ok := q.entries[testPlayerID]
			require.True(t, ok)
			assert.Equal(t, int64(42), entry.deckID)
			assert.Equal(t, "Alice", entry.name)
		})

		t.Run("デッキIDが0でなく名前とgatewayインスタンス識別子がいずれも空文字でないリクエストボディで登録エンドポイントを呼び出すと、202になる", func(t *testing.T) {
			r := newTestEngine(newHandler(newFakeQueue(), &fakeCircuit{}))

			w := doEnqueue(t, r, apimatchmaking.EnqueueRequest{DeckID: 42, Name: "Alice", Level: 3, GatewayInstanceID: "gw-1"})

			assert.Equal(t, http.StatusAccepted, w.Code)
		})

		t.Run("マッチメイキングキューへの登録が失敗する状態で登録エンドポイントを呼び出すと、503になる", func(t *testing.T) {
			q := newFakeQueue()
			q.enqueueErr = errors.New("enqueue failed")
			r := newTestEngine(newHandler(q, &fakeCircuit{}))

			w := doEnqueue(t, r, apimatchmaking.EnqueueRequest{DeckID: 42, Name: "Alice", Level: 3, GatewayInstanceID: "gw-1"})

			assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		})
	})
}

func TestCancel(t *testing.T) {
	t.Run("取消受付", func(t *testing.T) {
		t.Run("認証済みプレイヤーのマッチメイキングキューエントリが存在する状態で取消エンドポイントを呼び出すと、マッチメイキングキューにはそのプレイヤーがいなくなる", func(t *testing.T) {
			q := newFakeQueue()
			q.entries[testPlayerID] = queueEntry{}
			r := newTestEngine(newHandler(q, &fakeCircuit{}))

			doCancel(r)

			assert.False(t, q.contains(testPlayerID))
		})

		t.Run("認証済みプレイヤーのマッチメイキングキューエントリが存在する状態で取消エンドポイントを呼び出すと、200になる", func(t *testing.T) {
			q := newFakeQueue()
			q.entries[testPlayerID] = queueEntry{}
			r := newTestEngine(newHandler(q, &fakeCircuit{}))

			w := doCancel(r)

			assert.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("認証済みプレイヤーのマッチメイキングキューエントリが存在しない状態で取消エンドポイントを呼び出すと、404になる", func(t *testing.T) {
			r := newTestEngine(newHandler(newFakeQueue(), &fakeCircuit{}))

			w := doCancel(r)

			assert.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("マッチメイキングキューからの取り消しが失敗する状態で取消エンドポイントを呼び出すと、503になる", func(t *testing.T) {
			q := newFakeQueue()
			q.cancelErr = errors.New("cancel failed")
			r := newTestEngine(newHandler(q, &fakeCircuit{}))

			w := doCancel(r)

			assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		})
	})
}

func TestReportMatchAbandoned(t *testing.T) {
	t.Run("不成立申告受付", func(t *testing.T) {
		validRequest := func() apimatchmaking.MatchAbandonedRequest {
			return apimatchmaking.MatchAbandonedRequest{
				MatchID:   "mch_1",
				PlayerIDs: []string{"p1", "p2"},
				Reason:    apimatchmaking.MatchAbandonedRequestReasonPlayerNotConnected,
			}
		}

		t.Run("JSONとして解釈できないリクエストボディで不成立申告エンドポイントを呼び出すと、本文が不正であることを示す400になる", func(t *testing.T) {
			r := newTestEngine(newHandler(newFakeQueue(), &fakeCircuit{}))

			w := doJSONPost(r, "/internal/v1/match-abandoned", "{not json")

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), "request body")
		})

		t.Run("マッチIDが空文字のリクエストボディで不成立申告エンドポイントを呼び出すと、マッチIDが指定されていないことを示す400になる", func(t *testing.T) {
			r := newTestEngine(newHandler(newFakeQueue(), &fakeCircuit{}))
			body := validRequest()
			body.MatchID = ""

			w := doReportMatchAbandoned(t, r, body)

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), "match_id")
		})

		t.Run("プレイヤーIDの一覧の件数が不正のとき", func(t *testing.T) {
			counts := []struct {
				name      string
				playerIDs []string
			}{
				{name: "0件", playerIDs: []string{}},
				{name: "1件", playerIDs: []string{"p1"}},
				{name: "3件", playerIDs: []string{"p1", "p2", "p3"}},
			}
			for _, tt := range counts {
				t.Run("プレイヤーIDの一覧が"+tt.name+"のリクエストボディで不成立申告エンドポイントを呼び出すと、プレイヤーIDの件数が不正であることを示す400になる", func(t *testing.T) {
					r := newTestEngine(newHandler(newFakeQueue(), &fakeCircuit{}))
					body := validRequest()
					body.PlayerIDs = tt.playerIDs

					w := doReportMatchAbandoned(t, r, body)

					require.Equal(t, http.StatusBadRequest, w.Code)
					assert.Contains(t, w.Body.String(), "exactly")
				})
			}
		})

		t.Run("プレイヤーIDの一覧に空文字の要素を含むリクエストボディで不成立申告エンドポイントを呼び出すと、プレイヤーIDの一部が空であることを示す400になる", func(t *testing.T) {
			r := newTestEngine(newHandler(newFakeQueue(), &fakeCircuit{}))
			body := validRequest()
			body.PlayerIDs = []string{"p1", ""}

			w := doReportMatchAbandoned(t, r, body)

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), "empty")
		})

		t.Run("理由がplayer_not_connected以外の値のリクエストボディで不成立申告エンドポイントを呼び出すと、理由が未知の値であることを示す400になる", func(t *testing.T) {
			r := newTestEngine(newHandler(newFakeQueue(), &fakeCircuit{}))
			body := validRequest()
			body.Reason = apimatchmaking.MatchAbandonedRequestReason("unknown_reason")

			w := doReportMatchAbandoned(t, r, body)

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), "reason")
		})

		t.Run("マッチIDが設定されプレイヤーIDの一覧が空文字を含まない2件で理由がplayer_not_connectedのリクエストボディで不成立申告エンドポイントを呼び出すと、マッチメイキングキューには申告された2人がどちらもいなくなる", func(t *testing.T) {
			q := newFakeQueue()
			q.entries["p1"] = queueEntry{}
			q.entries["p2"] = queueEntry{}
			r := newTestEngine(newHandler(q, &fakeCircuit{}))

			doReportMatchAbandoned(t, r, validRequest())

			assert.False(t, q.contains("p1"))
			assert.False(t, q.contains("p2"))
		})

		t.Run("マッチIDが設定されプレイヤーIDの一覧が空文字を含まない2件で理由がplayer_not_connectedのリクエストボディで不成立申告エンドポイントを呼び出すと、204になる", func(t *testing.T) {
			q := newFakeQueue()
			q.entries["p1"] = queueEntry{}
			q.entries["p2"] = queueEntry{}
			r := newTestEngine(newHandler(q, &fakeCircuit{}))

			w := doReportMatchAbandoned(t, r, validRequest())

			assert.Equal(t, http.StatusNoContent, w.Code)
		})

		t.Run("破棄処理が失敗する状態で不成立申告エンドポイントを呼び出すと、503になる", func(t *testing.T) {
			q := newFakeQueue()
			q.cancelErr = errors.New("cancel failed")
			r := newTestEngine(newHandler(q, &fakeCircuit{}))

			w := doReportMatchAbandoned(t, r, validRequest())

			assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		})
	})
}

func TestRespondQueueSize(t *testing.T) {
	t.Run("キューサイズ取得応答", func(t *testing.T) {
		t.Run("マッチメイキングキューへの問い合わせが成功する状態でキューサイズ取得エンドポイントを呼び出すと、応答本文のsizeに現在のキューサイズが入る", func(t *testing.T) {
			q := newFakeQueue()
			q.sizeVal = 3
			r := newTestEngine(newHandler(q, &fakeCircuit{}))

			req := httptest.NewRequest(http.MethodGet, "/internal/v1/queue-size", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			var resp apimatchmaking.QueueSizeResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, int64(3), resp.Size)
		})

		t.Run("マッチメイキングキューへの問い合わせが失敗する状態でキューサイズ取得エンドポイントを呼び出すと、503になる", func(t *testing.T) {
			q := newFakeQueue()
			q.sizeErr = errors.New("size failed")
			r := newTestEngine(newHandler(q, &fakeCircuit{}))

			req := httptest.NewRequest(http.MethodGet, "/internal/v1/queue-size", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		})
	})
}

func TestRespondHealth(t *testing.T) {
	t.Run("ヘルスチェック応答", func(t *testing.T) {
		t.Run("サーキットブレーカーが閉じている状態でhealthエンドポイントを呼び出すと、200になり応答本文はstatus:ok・circuit:closedになる", func(t *testing.T) {
			r := newTestEngine(newHandler(newFakeQueue(), &fakeCircuit{isOpen: false}))

			req := httptest.NewRequest(http.MethodGet, "/internal/v1/health", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			var resp apimatchmaking.HealthResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, "ok", resp.Status)
			assert.Equal(t, "closed", resp.Circuit)
		})

		t.Run("サーキットブレーカーが開いている状態でhealthエンドポイントを呼び出すと、503になり応答本文はstatus:degraded・circuit:openになる", func(t *testing.T) {
			r := newTestEngine(newHandler(newFakeQueue(), &fakeCircuit{isOpen: true}))

			req := httptest.NewRequest(http.MethodGet, "/internal/v1/health", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusServiceUnavailable, w.Code)
			var resp apimatchmaking.HealthResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, "degraded", resp.Status)
			assert.Equal(t, "open", resp.Circuit)
		})
	})
}
