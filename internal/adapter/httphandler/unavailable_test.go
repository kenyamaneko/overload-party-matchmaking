package httphandler_test

import (
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler/httphandlertest"
)

func TestUnavailable(t *testing.T) {
	t.Run("[マッチメイキングAPI]503応答", func(t *testing.T) {
		t.Run("有効なトークンを付け、本文がすべて正しい追加を要求するとき", func(t *testing.T) {
			t.Run("異常系", func(t *testing.T) {
				t.Run("キューがエラーTST-queue-downを返すとき", func(t *testing.T) {
					t.Run("応答は、一時的に利用できないことを示す503のエラーになる", func(t *testing.T) {
						router := NewServerWithQueue(NewFailingQueue("TST-queue-down"))

						recorder := SendEnqueue(router, AuthPlayer1, EnqueueBody())

						AssertErrorResponse(t, recorder, 503, "matchmaking is temporarily unavailable")
					})

					t.Run("Errorレベルのログが1件だけ出力される", func(t *testing.T) {
						logs := CaptureLogs(t)
						router := NewServerWithQueue(NewFailingQueue("TST-queue-down"))

						SendEnqueue(router, AuthPlayer1, EnqueueBody())

						assert.Len(t, logs.RecordsAt(slog.LevelError), 1)
					})

					t.Run("ErrorレベルのログのerrorがエラーのメッセージのTST-queue-downになる", func(t *testing.T) {
						logs := CaptureLogs(t)
						router := NewServerWithQueue(NewFailingQueue("TST-queue-down"))

						SendEnqueue(router, AuthPlayer1, EnqueueBody())

						records := logs.RecordsAt(slog.LevelError)
						require.Len(t, records, 1)
						assert.Equal(t, "TST-queue-down", records[0].Attrs["error"])
					})
				})

				t.Run("同じ追加の要求を2回続けて送り、1回目はキューがエラーを返し、2回目はキューがエラーを返さないとき", func(t *testing.T) {
					secondRequestAfterRecovery := func() *httptest.ResponseRecorder {
						queue := NewFailingQueue("TST-queue-down")
						router := NewServerWithQueue(queue)
						SendEnqueue(router, AuthPlayer1, EnqueueBody())
						queue.EnqueueErr = nil
						return SendEnqueue(router, AuthPlayer1, EnqueueBody())
					}

					t.Run("2回目の応答のステータスが202になる", func(t *testing.T) {
						recorder := secondRequestAfterRecovery()

						assert.Equal(t, 202, recorder.Code)
					})

					t.Run("2回目の応答本文が空になる", func(t *testing.T) {
						recorder := secondRequestAfterRecovery()

						assert.Equal(t, 0, recorder.Body.Len())
					})
				})
			})
		})
	})
}
