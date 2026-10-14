package httphandlertest

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
)

// 各エンドポイントのパス。
const (
	PathEnqueue        = "/internal/v1/enqueue"
	PathCancel         = "/internal/v1/cancel"
	PathQueueSize      = "/internal/v1/queue-size"
	PathMatchAbandoned = "/internal/v1/match-abandoned"
	PathHealth         = "/internal/v1/health"
)

// トークンの検証の偽物が受け付けるトークンと、それが表すプレイヤー識別子。
const (
	tokenPlayer1  = "TST-TOKEN-1"
	tokenPlayer2  = "TST-TOKEN-2"
	tokenPlayer3  = "TST-TOKEN-3"
	tokenInvalid  = "TST-TOKEN-INVALID"
	PlayerID1     = "TST-PLAYER-1"
	PlayerID2     = "TST-PLAYER-2"
	PlayerID3     = "TST-PLAYER-3"
	GatewayID1    = "TST-GATEWAY-1"
	GatewayID2    = "TST-GATEWAY-2"
	defaultMatch  = "mch_TST0001"
	defaultReason = "player_not_connected"
)

// Auth は要求に認証の情報を付ける。
type Auth func(request *http.Request)

// WithToken は X-Internal-Auth ヘッダに token を付ける Auth を返す。
func WithToken(token string) Auth {
	return func(request *http.Request) { request.Header.Set(internalauth.HeaderName, token) }
}

// NoAuth は X-Internal-Auth ヘッダを付けない。
func NoAuth(*http.Request) {}

// トークンの検証の偽物が受け付けるトークンを付ける Auth。
var (
	AuthPlayer1 = WithToken(tokenPlayer1)
	AuthPlayer2 = WithToken(tokenPlayer2)
	AuthPlayer3 = WithToken(tokenPlayer3)
	AuthInvalid = WithToken(tokenInvalid)
)

var verificationByToken = map[string]func() (string, error){
	tokenPlayer1: func() (string, error) { return PlayerID1, nil },
	tokenPlayer2: func() (string, error) { return PlayerID2, nil },
	tokenPlayer3: func() (string, error) { return PlayerID3, nil },
	tokenInvalid: func() (string, error) { return "", errors.New("TST-invalid-token") },
}

// NewVerifier は tokenPlayer1〜3 を対応するプレイヤー識別子に検証し、tokenInvalid をエラーにする偽物を返す。これ以外のトークンを検証すると panic する。
func NewVerifier() *internalauth.MockVerifier {
	return &internalauth.MockVerifier{VerifyFn: func(token string) (string, error) {
		return verificationByToken[token]()
	}}
}

// NewServer は queue・circuit・abandoner を使うハンドラを NewVerifier の検証で公開するルーターを返す。
func NewServer(queue port.Queue, circuit httphandler.CircuitStater, abandoner httphandler.MatchAbandoner) http.Handler {
	gin.SetMode(gin.TestMode)
	return httphandler.NewRouter(httphandler.New(queue, circuit, abandoner), NewVerifier())
}

// NewServerWithQueue は queue だけを指定し、サーキットブレーカーを閉じた状態、マッチの破棄を成功にした NewServer を返す。
func NewServerWithQueue(queue port.Queue) http.Handler {
	return NewServer(queue, &FakeCircuit{}, &FakeAbandoner{})
}

// NewServerWithAbandoner は abandoner だけを指定し、キューを空、サーキットブレーカーを閉じた状態にした NewServer を返す。
func NewServerWithAbandoner(abandoner httphandler.MatchAbandoner) http.Handler {
	return NewServer(&FakeQueue{}, &FakeCircuit{}, abandoner)
}

// NewServerWithCircuit は circuit だけを指定し、キューを空、マッチの破棄を成功にした NewServer を返す。
func NewServerWithCircuit(circuit httphandler.CircuitStater) http.Handler {
	return NewServer(&FakeQueue{}, circuit, &FakeAbandoner{})
}

// Send は router に auth を付けた HTTP 要求を送る。
func Send(router http.Handler, method, path string, auth Auth, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	auth(request)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// SendEnqueue は auth を付けて body を追加の要求として送る。
func SendEnqueue(router http.Handler, auth Auth, body string) *httptest.ResponseRecorder {
	return Send(router, http.MethodPost, PathEnqueue, auth, body)
}

// SendEnqueueFromGateway は auth を付けて、gateway_instance_id が gatewayID の正しい本文を追加の要求として送る。
func SendEnqueueFromGateway(router http.Handler, auth Auth, gatewayID string) *httptest.ResponseRecorder {
	return SendEnqueue(router, auth, EnqueueBody(GatewayInstanceID(gatewayID)))
}

// SendCancel は auth を付けて除外の要求を送る。
func SendCancel(router http.Handler, auth Auth) *httptest.ResponseRecorder {
	return Send(router, http.MethodPost, PathCancel, auth, "")
}

// SendQueueSize は X-Internal-Auth ヘッダを付けずにキューサイズの取得を要求する。
func SendQueueSize(router http.Handler) *httptest.ResponseRecorder {
	return Send(router, http.MethodGet, PathQueueSize, NoAuth, "")
}

// SendHealth は X-Internal-Auth ヘッダを付けずにヘルスチェックを要求する。
func SendHealth(router http.Handler) *httptest.ResponseRecorder {
	return Send(router, http.MethodGet, PathHealth, NoAuth, "")
}

// SendMatchAbandoned は X-Internal-Auth ヘッダを付けずに body を不成立の申告として送る。
func SendMatchAbandoned(router http.Handler, body string) *httptest.ResponseRecorder {
	return Send(router, http.MethodPost, PathMatchAbandoned, NoAuth, body)
}

// BodyMod は JSON 本文の項目を差し替える。
type BodyMod func(fields map[string]json.RawMessage)

// Set は key の値を rawJSON に差し替える。
func Set(key, rawJSON string) BodyMod {
	return func(fields map[string]json.RawMessage) { fields[key] = json.RawMessage(rawJSON) }
}

// Omit は key を本文から取り除く。
func Omit(key string) BodyMod {
	return func(fields map[string]json.RawMessage) { delete(fields, key) }
}

// GatewayInstanceID は gateway_instance_id を id に差し替える。
func GatewayInstanceID(id string) BodyMod {
	return Set("gateway_instance_id", mustMarshal(id))
}

// PlayerIDs は player_ids を ids の配列に差し替える。
func PlayerIDs(ids ...string) BodyMod {
	return Set("player_ids", mustMarshal(ids))
}

// EnqueueBody は、すべて正しい追加の本文に mods を適用した JSON 文字列を返す。
func EnqueueBody(mods ...BodyMod) string {
	return buildBody(map[string]json.RawMessage{
		"deck_id":             json.RawMessage(`11`),
		"name":                json.RawMessage(mustMarshal("テスト太郎")),
		"level":               json.RawMessage(`7`),
		"gateway_instance_id": json.RawMessage(mustMarshal(GatewayID1)),
	}, mods)
}

// AbandonBody は、すべて正しい不成立の申告の本文に mods を適用した JSON 文字列を返す。
func AbandonBody(mods ...BodyMod) string {
	return buildBody(map[string]json.RawMessage{
		"match_id":   json.RawMessage(mustMarshal(defaultMatch)),
		"player_ids": json.RawMessage(mustMarshal([]string{PlayerID1, PlayerID2})),
		"reason":     json.RawMessage(mustMarshal(defaultReason)),
	}, mods)
}

func buildBody(fields map[string]json.RawMessage, mods []BodyMod) string {
	for _, mod := range mods {
		mod(fields)
	}
	return mustMarshal(fields)
}

func mustMarshal(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// ReadErrorMessage は応答本文の JSON オブジェクトの error 項目の文字列を返す。
func ReadErrorMessage(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Error
}

// AssertErrorResponse は応答のステータスが status で、応答本文の error の文字列が message と一致することを確かめる。
func AssertErrorResponse(t *testing.T, recorder *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	assert.Equal(t, status, recorder.Code)
	assert.Equal(t, message, ReadErrorMessage(t, recorder))
}

// ReadQueueSize はキューサイズ取得の応答が 200 であることを確かめ、応答本文の size を返す。
func ReadQueueSize(t *testing.T, router http.Handler) int {
	t.Helper()
	recorder := SendQueueSize(router)
	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Size int `json:"size"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Size
}

// ReadHealth は応答本文の status と circuit を返す。
func ReadHealth(t *testing.T, recorder *httptest.ResponseRecorder) (status, circuit string) {
	t.Helper()
	var body struct {
		Status  string `json:"status"`
		Circuit string `json:"circuit"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Status, body.Circuit
}
