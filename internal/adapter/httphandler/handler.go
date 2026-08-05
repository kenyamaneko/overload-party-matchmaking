package httphandler

import (
	"context"
	"fmt"
	"log/slog"
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

// 1 マッチのプレイヤー数。openapi.yaml の player_ids の minItems / maxItems と一致させる。
const matchPlayerCount = 2

// CircuitStater はサーキットブレーカーの状態を公開するインタフェースです。
type CircuitStater interface {
	IsCircuitOpen() bool
}

// MatchAbandoner は不成立が申告されたマッチの破棄を抽象化するインタフェースです。
type MatchAbandoner interface {
	Abandon(ctx context.Context, matchID string, playerIDs []string, reason string) error
}

// Handler はマッチメイキング HTTP ハンドラを提供します。
type Handler struct {
	queue     port.Queue
	circuit   CircuitStater
	abandoner MatchAbandoner
}

// New は Handler を生成します。
func New(queue port.Queue, circuit CircuitStater, abandoner MatchAbandoner) *Handler {
	return &Handler{queue: queue, circuit: circuit, abandoner: abandoner}
}

// Enqueue はプレイヤーをマッチメイキングキューに追加します。
// player_id は VerifyInternalAuth が JWT sub から context に注入したものを利用する。
// name / level は呼び出し側 (gateway 経由) が /me で取得した player summary snapshot で、
// matchmaking は account を呼ばずそのまま queue entry に保存し match_made event に同梱する。
// gateway_instance_id が直前の Enqueue と異なる場合、queue がリセットされたことを Warn ログに残す。
func (h *Handler) Enqueue(c *gin.Context) {
	var req apimatchmaking.EnqueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidRequestBodyMessage})
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
	if req.GatewayInstanceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateway_instance_id is required"})
		return
	}
	playerID := c.GetString(internalauth.PlayerIDContextKey)
	removed, err := h.queue.Enqueue(c.Request.Context(), playerID, req.DeckID, req.Name, req.Level, req.GatewayInstanceID)
	if err != nil {
		respondUnavailable(c, err)
		return
	}
	if removed > 0 {
		slog.Warn("matchmaking queue reset for new gateway instance",
			"gateway_instance_id", req.GatewayInstanceID, "removed_entries", removed)
	}
	c.Status(http.StatusAccepted)
}

// Cancel はプレイヤーのマッチメイキング待機をキャンセルします。
// player_id は VerifyInternalAuth が JWT sub から context に注入したものを利用する。
func (h *Handler) Cancel(c *gin.Context) {
	playerID := c.GetString(internalauth.PlayerIDContextKey)
	removed, err := h.queue.Cancel(c.Request.Context(), playerID)
	if err != nil {
		respondUnavailable(c, err)
		return
	}
	if !removed {
		c.Status(http.StatusNotFound)
		return
	}
	c.Status(http.StatusOK)
}

// ReportMatchAbandoned は成立したマッチの不成立の申告を受け、ペアを破棄します。
func (h *Handler) ReportMatchAbandoned(c *gin.Context) {
	var req apimatchmaking.MatchAbandonedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidRequestBodyMessage})
		return
	}
	if req.MatchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "match_id is required"})
		return
	}
	if len(req.PlayerIDs) != matchPlayerCount {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("player_ids must contain exactly %d ids", matchPlayerCount),
		})
		return
	}
	for _, id := range req.PlayerIDs {
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "player_ids must not contain an empty id"})
			return
		}
	}
	if !req.Reason.Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("reason %q is not a known value", req.Reason)})
		return
	}
	if err := h.abandoner.Abandon(c.Request.Context(), req.MatchID, req.PlayerIDs, string(req.Reason)); err != nil {
		respondUnavailable(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// RespondQueueSize は現在のキュー内プレイヤー数を返します。
func (h *Handler) RespondQueueSize(c *gin.Context) {
	n, err := h.queue.Size(c.Request.Context())
	if err != nil {
		respondUnavailable(c, err)
		return
	}
	c.JSON(http.StatusOK, apimatchmaking.QueueSizeResponse{Size: n})
}

// RespondHealth はサーキットブレーカーの状態を含むヘルスチェック結果を返します。
// k8s liveness/readiness probe の対象でもあるため、circuit open 時は 503 を返す。
func (h *Handler) RespondHealth(c *gin.Context) {
	isOpen := h.circuit.IsCircuitOpen()
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
