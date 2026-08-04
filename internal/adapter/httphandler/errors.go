package httphandler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// unavailableMessage は依存先の障害で処理できなかったときに 503 応答本文へ返す文言。
// 呼び出し側が対処できない原因を外部へ出さないため、実際のエラーではなく固定の文言を返す。
const unavailableMessage = "matchmaking is temporarily unavailable"

// invalidRequestBodyMessage はリクエストボディを読み取れなかったときに 400 応答本文へ返す文言。
// 解析ライブラリ由来の文字列を外部へ出さないため、自前の文言に置き換える。
const invalidRequestBodyMessage = "request body is not valid"

// respondUnavailable は依存先の障害を 503 で返し、原因を構造化ログに記録します。
func respondUnavailable(c *gin.Context, err error) {
	slog.Error("matchmaking dependency unavailable",
		"path", c.Request.URL.Path,
		"method", c.Request.Method,
		"error", err,
	)
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": unavailableMessage})
}
