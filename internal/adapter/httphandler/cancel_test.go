package httphandler_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler/httphandlertest"
)

func TestCancel(t *testing.T) {
	t.Run("[マッチメイキングキュー]キューからの除外", func(t *testing.T) {
		t.Run("有効なトークンを付けて除外を要求するとき", func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				t.Run("キューに待機しているプレイヤーを除外できるとき", func(t *testing.T) {
					t.Run("応答のステータスが200になる", func(t *testing.T) {
						router := NewServerWithQueue(&FakeQueue{CancelRemoved: true})

						recorder := SendCancel(router, AuthPlayer1)

						assert.Equal(t, 200, recorder.Code)
					})

					t.Run("応答本文が空になる", func(t *testing.T) {
						router := NewServerWithQueue(&FakeQueue{CancelRemoved: true})

						recorder := SendCancel(router, AuthPlayer1)

						assert.Equal(t, 0, recorder.Body.Len())
					})
				})
			})

			t.Run("異常系", func(t *testing.T) {
				t.Run("キューに待機していないプレイヤーを除外しようとするとき、応答のステータスが、キューに待機していないことを意味する404になる", func(t *testing.T) {
					router := NewServerWithQueue(&FakeQueue{CancelRemoved: false})

					recorder := SendCancel(router, AuthPlayer1)

					assert.Equal(t, 404, recorder.Code)
				})

				t.Run("キューがエラーを返すとき、応答のステータスが、一時的に利用できないことを意味する503になる", func(t *testing.T) {
					router := NewServerWithQueue(NewFailingQueue("TST-queue-down"))

					recorder := SendCancel(router, AuthPlayer1)

					assert.Equal(t, 503, recorder.Code)
				})
			})
		})
	})
}
