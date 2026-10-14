package httphandler_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler/httphandlertest"
)

func TestEnqueue(t *testing.T) {
	t.Run("[マッチメイキングキュー]キューへの追加", func(t *testing.T) {
		t.Run("有効なトークンを付けて追加を要求するとき", func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				t.Run("キューがエラーを返さないとき", func(t *testing.T) {
					tests := []struct {
						name string
						body string
					}{
						{"本文がすべて正しく、キューが空のとき、応答のステータスが202になる", EnqueueBody()},
						{"本文のdeck_idが1のとき、応答のステータスが202になる", EnqueueBody(Set("deck_id", `1`))},
						{"本文のnameが1文字の「あ」のとき、応答のステータスが202になる", EnqueueBody(Set("name", `"あ"`))},
						{"本文のgateway_instance_idが1文字のgのとき、応答のステータスが202になる", EnqueueBody(Set("gateway_instance_id", `"g"`))},
					}
					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							router := NewServerWithQueue(&FakeQueue{})

							recorder := SendEnqueue(router, AuthPlayer1, tt.body)

							assert.Equal(t, 202, recorder.Code)
						})
					}

					t.Run("本文がすべて正しく、キューが空のとき、応答本文が空になる", func(t *testing.T) {
						router := NewServerWithQueue(&FakeQueue{})

						recorder := SendEnqueue(router, AuthPlayer1, EnqueueBody())

						assert.Equal(t, 0, recorder.Body.Len())
					})

					t.Run("本文のgateway_instance_idとは別のgateway_instance_idで待機中のエントリがあるとき", func(t *testing.T) {
						tests := []struct {
							name         string
							removedCount int64
						}{
							{"待機中のエントリが1件のとき、応答のステータスが202になる", 1},
							{"待機中のエントリが3件のとき、応答のステータスが202になる", 3},
						}
						for _, tt := range tests {
							t.Run(tt.name, func(t *testing.T) {
								router := NewServerWithQueue(&FakeQueue{EnqueueRemovedCount: tt.removedCount})

								recorder := SendEnqueue(router, AuthPlayer1, EnqueueBody())

								assert.Equal(t, 202, recorder.Code)
							})
						}
					})
				})
			})

			t.Run("異常系", func(t *testing.T) {
				t.Run("本文の必須項目のうち一つだけが正しくないとき", func(t *testing.T) {
					tests := []struct {
						name    string
						body    string
						message string
					}{
						{"deck_idが0のとき、応答は、deck_idが1以上の整数でなければならないことを示す400のエラーになる", EnqueueBody(Set("deck_id", `0`)), "deck_id must be a positive integer"},
						{"deck_idが負の値の-1のとき、応答は、deck_idが1以上の整数でなければならないことを示す400のエラーになる", EnqueueBody(Set("deck_id", `-1`)), "deck_id must be a positive integer"},
						{"deck_idのキーが無いとき、応答は、deck_idが1以上の整数でなければならないことを示す400のエラーになる", EnqueueBody(Omit("deck_id")), "deck_id must be a positive integer"},
						{"nameが空文字のとき、応答は、nameが必須であることを示す400のエラーになる", EnqueueBody(Set("name", `""`)), "name is required"},
						{"nameが空白だけのとき、応答は、nameが必須であることを示す400のエラーになる", EnqueueBody(Set("name", `" "`)), "name is required"},
						{"nameのキーが無いとき、応答は、nameが必須であることを示す400のエラーになる", EnqueueBody(Omit("name")), "name is required"},
						{"gateway_instance_idが空文字のとき、応答は、gateway_instance_idが必須であることを示す400のエラーになる", EnqueueBody(Set("gateway_instance_id", `""`)), "gateway_instance_id is required"},
						{"gateway_instance_idが空白だけのとき、応答は、gateway_instance_idが必須であることを示す400のエラーになる", EnqueueBody(Set("gateway_instance_id", `" "`)), "gateway_instance_id is required"},
						{"gateway_instance_idのキーが無いとき、応答は、gateway_instance_idが必須であることを示す400のエラーになる", EnqueueBody(Omit("gateway_instance_id")), "gateway_instance_id is required"},
					}
					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							router := NewServerWithQueue(&FakeQueue{})

							recorder := SendEnqueue(router, AuthPlayer1, tt.body)

							AssertErrorResponse(t, recorder, 400, tt.message)
						})
					}
				})

				t.Run("本文を読み取れないとき", func(t *testing.T) {
					tests := []struct {
						name string
						body string
					}{
						{"本文がJSONとして壊れているとき、応答は、本文が不正であることを示す400のエラーになる", `{"deck_id":`},
						{"本文が空のとき、応答は、本文が不正であることを示す400のエラーになる", ``},
						{"deck_idが文字列の11のとき、応答は、本文が不正であることを示す400のエラーになる", EnqueueBody(Set("deck_id", `"11"`))},
					}
					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							router := NewServerWithQueue(&FakeQueue{})

							recorder := SendEnqueue(router, AuthPlayer1, tt.body)

							AssertErrorResponse(t, recorder, 400, "request body is not valid")
						})
					}
				})
			})
		})
	})
}
