package httphandler_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler/httphandlertest"
)

func TestInternalAuth(t *testing.T) {
	t.Run("[マッチメイキングAPI]X-Internal-Authによる認証", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("X-Internal-Authヘッダを付けずにキューサイズを取得し、キューがエラーを返さないとき、応答のステータスが200になる", func(t *testing.T) {
				router := NewServerWithQueue(&FakeQueue{})

				recorder := SendQueueSize(router)

				assert.Equal(t, 200, recorder.Code)
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("キューへの追加を要求するとき", func(t *testing.T) {
				tests := []struct {
					name string
					auth Auth
				}{
					{"X-Internal-Authヘッダを付けないとき、応答のステータスが、認証に失敗したことを意味する401になる", NoAuth},
					{"検証に失敗するトークンを付けたとき、応答のステータスが、認証に失敗したことを意味する401になる", AuthInvalid},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						router := NewServerWithQueue(&FakeQueue{})

						recorder := SendEnqueue(router, tt.auth, EnqueueBody())

						assert.Equal(t, 401, recorder.Code)
					})
				}
			})

			t.Run("キューからの除外を要求するとき", func(t *testing.T) {
				tests := []struct {
					name string
					auth Auth
				}{
					{"X-Internal-Authヘッダを付けないとき、応答のステータスが、認証に失敗したことを意味する401になる", NoAuth},
					{"検証に失敗するトークンを付けたとき、応答のステータスが、認証に失敗したことを意味する401になる", AuthInvalid},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						router := NewServerWithQueue(&FakeQueue{CancelRemoved: true})

						recorder := SendCancel(router, tt.auth)

						assert.Equal(t, 401, recorder.Code)
					})
				}
			})
		})
	})
}
