package httphandler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

// Health 応答の status / circuit 値。openapi.yaml の例示と一致させる。
const (
	healthStatusOK       = "ok"
	healthStatusDegraded = "degraded"
	healthCircuitClosed  = "closed"
	healthCircuitOpen    = "open"
)

// CircuitStater はサーキットブレーカーの状態を公開するインタフェースです。
type CircuitStater interface {
	IsCircuitOpen() bool
}

// Handler はマッチメイキング HTTP ハンドラを提供します。
type Handler struct {
	queue   port.Queue
	circuit CircuitStater
}

// New は Handler を生成します。
func New(queue port.Queue, circuit CircuitStater) *Handler {
	return &Handler{queue: queue, circuit: circuit}
}

// Enqueue はプレイヤーをマッチメイキングキューに追加します。
// player_id は VerifyInternalAuth が JWT sub から context に注入したものを利用する。
// name / level は呼び出し側 (gateway 経由) が /me で取得した player summary snapshot で、
// matchmaking は account を呼ばずそのまま queue entry に保存し match_made event に同梱する。
func (h *Handler) Enqueue(c *gin.Context) {
	var req apimatchmaking.EnqueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.DeckID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deck_id is required"})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	playerID := c.GetString(internalauth.PlayerIDContextKey)
	if err := h.queue.Enqueue(c.Request.Context(), playerID, req.DeckID, req.Name, req.Level); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusAccepted)
}

// Cancel はプレイヤーのマッチメイキング待機をキャンセルします。
// player_id は VerifyInternalAuth が JWT sub から context に注入したものを利用する。
func (h *Handler) Cancel(c *gin.Context) {
	playerID := c.GetString(internalauth.PlayerIDContextKey)
	removed, err := h.queue.Cancel(c.Request.Context(), playerID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	if !removed {
		c.Status(http.StatusNotFound)
		return
	}
	c.Status(http.StatusOK)
}

// RespondQueueSize は現在のキュー内プレイヤー数を返します。
func (h *Handler) RespondQueueSize(c *gin.Context) {
	n, err := h.queue.Size(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, apimatchmaking.QueueSizeResponse{Size: n})
}

// RespondHealth はサーキットブレーカーの状態を含むヘルスチェック結果を返します。
// k8s liveness/readiness probe の対象でもあるため、circuit open 時は 503 を返す。
func (h *Handler) RespondHealth(c *gin.Context) {
	isOpen := h.circuit != nil && h.circuit.IsCircuitOpen()
	if isOpen {
		c.JSON(http.StatusServiceUnavailable, apimatchmaking.HealthResponse{
			Status:  healthStatusDegraded,
			Circuit: healthCircuitOpen,
		})
		return
	}
	c.JSON(http.StatusOK, apimatchmaking.HealthResponse{
		Status:  healthStatusOK,
		Circuit: healthCircuitClosed,
	})
}
