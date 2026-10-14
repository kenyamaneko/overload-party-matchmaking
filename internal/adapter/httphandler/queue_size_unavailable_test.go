package httphandler_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler/httphandlertest"
)

func TestQueueSizeUnavailable(t *testing.T) {
	t.Run("[マッチメイキングキュー]キューサイズの取得", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("キューサイズを要求し、キューがエラーを返すとき、応答のステータスが、一時的に利用できないことを意味する503になる", func(t *testing.T) {
				router := NewServerWithQueue(NewFailingQueue("TST-queue-down"))

				recorder := SendQueueSize(router)

				assert.Equal(t, 503, recorder.Code)
			})
		})
	})
}
