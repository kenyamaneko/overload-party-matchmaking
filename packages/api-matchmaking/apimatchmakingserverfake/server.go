// Package apimatchmakingserverfake は matchmaking の HTTP 契約を実装する httptest.Server wrapper。
// 各 endpoint は Fn field (func callback) で応答を制御し、Fn=nil なら既定値を返す。
package apimatchmakingserverfake

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

// Server は matchmaking HTTP 契約を実装する httptest.Server wrapper。
type Server struct {
	srv *httptest.Server

	// EnqueueFn は POST /internal/v1/enqueue の応答を決定する (nil は 202 + 空 body)。
	EnqueueFn func(req apimatchmaking.EnqueueRequest) (status int, body any)

	// CancelFn は POST /internal/v1/cancel の応答を決定する (nil は 200 + 空 body)。
	// player_id は JWT sub から解決するため body を持たない。
	CancelFn func() (status int, body any)

	// MatchAbandonedFn は POST /internal/v1/match-abandoned の応答を決定する (nil は 204 + 空 body)。
	MatchAbandonedFn func(req apimatchmaking.MatchAbandonedRequest) (status int, body any)
}

// NewServer は起動済み Server を返す。テスト終了時に Close() すること。
func NewServer() *Server {
	s := &Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/v1/enqueue", s.handleEnqueue)
	mux.HandleFunc("POST /internal/v1/cancel", s.handleCancel)
	mux.HandleFunc("POST /internal/v1/match-abandoned", s.handleMatchAbandoned)
	s.srv = httptest.NewServer(mux)
	return s
}

// URL は httptest.Server のベース URL を返す。
func (s *Server) URL() string { return s.srv.URL }

// Close は内部 httptest.Server を閉じる。
func (s *Server) Close() { s.srv.Close() }

func (s *Server) handleEnqueue(w http.ResponseWriter, r *http.Request) {
	var req apimatchmaking.EnqueueRequest
	// body 不正でも Fn には空構造体を渡し、bad request の擬似は Fn 側で表現する。
	_ = json.NewDecoder(r.Body).Decode(&req)
	_, _ = io.Copy(io.Discard, r.Body)

	if s.EnqueueFn == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	status, body := s.EnqueueFn(req)
	writeJSON(w, status, body)
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)

	if s.CancelFn == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	status, body := s.CancelFn()
	writeJSON(w, status, body)
}

func (s *Server) handleMatchAbandoned(w http.ResponseWriter, r *http.Request) {
	var req apimatchmaking.MatchAbandonedRequest
	// body 不正でも Fn には空構造体を渡し、bad request の擬似は Fn 側で表現する。
	_ = json.NewDecoder(r.Body).Decode(&req)
	_, _ = io.Copy(io.Discard, r.Body)

	if s.MatchAbandonedFn == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	status, body := s.MatchAbandonedFn(req)
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	if body == nil {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
