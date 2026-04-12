package httphandler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
)

// CircuitStater はサーキットブレーカーの状態を公開するインタフェースです。
type CircuitStater interface {
	CircuitOpen() bool
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

type enqueueRequest struct {
	PlayerID string `json:"playerId" binding:"required"`
	DeckID   int64  `json:"deckId" binding:"required"`
}

type cancelRequest struct {
	PlayerID string `json:"playerId" binding:"required"`
}

// Enqueue はプレイヤーをマッチメイキングキューに追加します。
func (h *Handler) Enqueue(c *gin.Context) {
	var req enqueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.queue.Enqueue(c.Request.Context(), req.PlayerID, req.DeckID); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusAccepted)
}

// Cancel はプレイヤーのマッチメイキング待機をキャンセルします。
func (h *Handler) Cancel(c *gin.Context) {
	var req cancelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	removed, err := h.queue.Cancel(c.Request.Context(), req.PlayerID)
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

// QueueSize は現在のキュー内プレイヤー数を返します。
func (h *Handler) QueueSize(c *gin.Context) {
	n, err := h.queue.Size(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"size": n})
}

// Health はサーキットブレーカーの状態を含むヘルスチェック結果を返します。
func (h *Handler) Health(c *gin.Context) {
	open := h.circuit != nil && h.circuit.CircuitOpen()
	if open {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "degraded",
			"circuit": "open",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"circuit": "closed",
	})
}
