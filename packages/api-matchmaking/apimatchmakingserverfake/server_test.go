package apimatchmakingserverfake_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
	"github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking/apimatchmakingserverfake"
)

// TestServer_Defaults は Fn 未設定時の各 endpoint の既定応答を検証する。
func TestServer_Defaults(t *testing.T) {
	s := apimatchmakingserverfake.NewServer()
	defer s.Close()

	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		wantStatus int
		decodeInto any
		assertBody func(t *testing.T, decoded any)
	}{
		{
			name:       "enqueue 既定 202",
			method:     http.MethodPost,
			path:       "/internal/v1/enqueue",
			body:       apimatchmaking.EnqueueRequest{PlayerID: "p1", DeckID: 1},
			wantStatus: http.StatusAccepted,
			decodeInto: nil,
			assertBody: nil,
		},
		{
			name:       "cancel 既定 200",
			method:     http.MethodPost,
			path:       "/internal/v1/cancel",
			body:       apimatchmaking.CancelRequest{PlayerID: "p1"},
			wantStatus: http.StatusOK,
			decodeInto: nil,
			assertBody: nil,
		},
		{
			name:       "queue-size 既定 size=0",
			method:     http.MethodGet,
			path:       "/internal/v1/queue-size",
			body:       nil,
			wantStatus: http.StatusOK,
			decodeInto: &apimatchmaking.QueueSizeResponse{},
			assertBody: func(t *testing.T, decoded any) {
				got := decoded.(*apimatchmaking.QueueSizeResponse)
				assert.Equal(t, int64(0), got.Size)
			},
		},
		{
			name:       "health 既定 ok/closed",
			method:     http.MethodGet,
			path:       "/internal/v1/health",
			body:       nil,
			wantStatus: http.StatusOK,
			decodeInto: &apimatchmaking.HealthResponse{},
			assertBody: func(t *testing.T, decoded any) {
				got := decoded.(*apimatchmaking.HealthResponse)
				assert.Equal(t, "ok", got.Status)
				assert.Equal(t, "closed", got.Circuit)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := doRequest(t, s.URL(), tt.method, tt.path, tt.body)
			defer resp.Body.Close()

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			if tt.decodeInto != nil {
				require.NoError(t, json.NewDecoder(resp.Body).Decode(tt.decodeInto))
				tt.assertBody(t, tt.decodeInto)
			}
		})
	}
}

// TestServer_FnOverridesResponse は Fn を設定した endpoint で fn の戻り値どおりに応答することを検証する。
func TestServer_FnOverridesResponse(t *testing.T) {
	s := apimatchmakingserverfake.NewServer()
	defer s.Close()

	var receivedEnqueue apimatchmaking.EnqueueRequest
	s.EnqueueFn = func(req apimatchmaking.EnqueueRequest) (int, any) {
		receivedEnqueue = req
		return http.StatusServiceUnavailable, map[string]string{"error": "redis down"}
	}
	s.HealthFn = func() (int, any) {
		return http.StatusServiceUnavailable, apimatchmaking.HealthResponse{Status: "degraded", Circuit: "open"}
	}

	resp := doRequest(t, s.URL(), http.MethodPost, "/internal/v1/enqueue",
		apimatchmaking.EnqueueRequest{PlayerID: "px", DeckID: 7})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, "px", receivedEnqueue.PlayerID)
	assert.Equal(t, int64(7), receivedEnqueue.DeckID)

	resp2 := doRequest(t, s.URL(), http.MethodGet, "/internal/v1/health", nil)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp2.StatusCode)
	var hr apimatchmaking.HealthResponse
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&hr))
	assert.Equal(t, "degraded", hr.Status)
	assert.Equal(t, "open", hr.Circuit)
}

func doRequest(t *testing.T, baseURL, method, path string, body any) *http.Response {
	t.Helper()
	var buf io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		buf = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, baseURL+path, buf)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}
