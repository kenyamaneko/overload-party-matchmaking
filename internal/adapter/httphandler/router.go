package httphandler

import "github.com/gin-gonic/gin"

// NewRouter はマッチメイキング API のルーティングを構築します。
func NewRouter(h *Handler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	internal := r.Group("/internal/v1")
	{
		internal.POST("/enqueue", h.Enqueue)
		internal.POST("/cancel", h.Cancel)
		internal.GET("/queue-size", h.QueueSize)
		internal.GET("/health", h.Health)
	}
	return r
}
