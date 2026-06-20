package httphandler

import (
	"github.com/gin-gonic/gin"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
)

// NewRouter はマッチメイキング API のルーティングを構築します。
// player-scoped (enqueue / cancel) は X-Internal-Auth (HS256 JWT) を要求し、
// queue-size / health は ClusterIP 内部からの運用 / k8s probe 用に認証なしで公開する。
func NewRouter(h *Handler, authVerifier internalauth.Verifier) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	internal := r.Group("/internal/v1")
	{
		internal.GET("/queue-size", h.RespondQueueSize)
		internal.GET("/health", h.RespondHealth)

		authed := internal.Group("")
		authed.Use(internalauth.VerifyInternalAuth(authVerifier))
		{
			authed.POST("/enqueue", h.Enqueue)
			authed.POST("/cancel", h.Cancel)
		}
	}
	return r
}
