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

func TestServer(t *testing.T) {
	t.Run("サーバフェイク", func(t *testing.T) {
		t.Run("設定した Fn が status と body を上書きし、typed request を受け取る", func(t *testing.T) {
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
				apimatchmaking.EnqueueRequest{DeckID: 7})
			defer resp.Body.Close()
			assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
			assert.Equal(t, int64(7), receivedEnqueue.DeckID)

			resp2 := doRequest(t, s.URL(), http.MethodGet, "/internal/v1/health", nil)
			defer resp2.Body.Close()
			assert.Equal(t, http.StatusServiceUnavailable, resp2.StatusCode)
			var hr apimatchmaking.HealthResponse
			require.NoError(t, json.NewDecoder(resp2.Body).Decode(&hr))
			assert.Equal(t, "degraded", hr.Status)
			assert.Equal(t, "open", hr.Circuit)
		})
	})
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
