package httphandler_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler/httphandlertest"
)

func TestReportMatchAbandoned(t *testing.T) {
	t.Run("[マッチ不成立]不成立の申告", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("X-Internal-Authヘッダを付けずに申告し、マッチの破棄が成功するとき", func(t *testing.T) {
				tests := []struct {
					name string
					body string
				}{
					{"本文がすべて正しいとき、応答のステータスが204になる", AbandonBody()},
					{"本文のmatch_idが1文字のgのとき、応答のステータスが204になる", AbandonBody(Set("match_id", `"g"`))},
					{"本文のplayer_idsの二つの識別子がともに1文字のaとbのとき、応答のステータスが204になる", AbandonBody(PlayerIDs("a", "b"))},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						router := NewServerWithAbandoner(&FakeAbandoner{})

						recorder := SendMatchAbandoned(router, tt.body)

						assert.Equal(t, 204, recorder.Code)
					})
				}

				t.Run("本文がすべて正しいとき、応答本文が空になる", func(t *testing.T) {
					router := NewServerWithAbandoner(&FakeAbandoner{})

					recorder := SendMatchAbandoned(router, AbandonBody())

					assert.Equal(t, 0, recorder.Body.Len())
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("本文のmatch_id・player_ids・reasonのうち一つだけが正しくないとき", func(t *testing.T) {
				tests := []struct {
					name    string
					body    string
					message string
				}{
					{"match_idが空文字のとき、応答は、match_idが必須であることを示す400のエラーになる", AbandonBody(Set("match_id", `""`)), "match_id is required"},
					{"match_idのキーが無いとき、応答は、match_idが必須であることを示す400のエラーになる", AbandonBody(Omit("match_id")), "match_id is required"},
					{"player_idsのキーが無いとき、応答は、player_idsがちょうど2件でなければならないことを示す400のエラーになる", AbandonBody(Omit("player_ids")), "player_ids must contain exactly 2 ids"},
					{"player_idsが空の配列のとき、応答は、player_idsがちょうど2件でなければならないことを示す400のエラーになる", AbandonBody(PlayerIDs()), "player_ids must contain exactly 2 ids"},
					{"player_idsが1件のとき、応答は、player_idsがちょうど2件でなければならないことを示す400のエラーになる", AbandonBody(PlayerIDs(PlayerID1)), "player_ids must contain exactly 2 ids"},
					{"player_idsが3件のとき、応答は、player_idsがちょうど2件でなければならないことを示す400のエラーになる", AbandonBody(PlayerIDs(PlayerID1, PlayerID2, PlayerID3)), "player_ids must contain exactly 2 ids"},
					{"player_idsの1つ目が空文字のとき、応答は、player_idsに空の識別子を含められないことを示す400のエラーになる", AbandonBody(PlayerIDs("", PlayerID2)), "player_ids must not contain an empty id"},
					{"player_idsの2つ目が空文字のとき、応答は、player_idsに空の識別子を含められないことを示す400のエラーになる", AbandonBody(PlayerIDs(PlayerID1, "")), "player_ids must not contain an empty id"},
					{"player_idsの二つの識別子がともにTST-PLAYER-1のとき、応答は、player_idsに同じ識別子を含められないことを示す400のエラーになる", AbandonBody(PlayerIDs(PlayerID1, PlayerID1)), "player_ids must not contain a duplicate id"},
					{"reasonが空文字のとき、応答は、送られたreasonの値が未知であることを示す400のエラーになる", AbandonBody(Set("reason", `""`)), `reason "" is not a known value`},
					{"reasonのキーが無いとき、応答は、送られたreasonの値が未知であることを示す400のエラーになる", AbandonBody(Omit("reason")), `reason "" is not a known value`},
					{"reasonがunknown_reasonのとき、応答は、送られたreasonの値が未知であることを示す400のエラーになる", AbandonBody(Set("reason", `"unknown_reason"`)), `reason "unknown_reason" is not a known value`},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						router := NewServerWithAbandoner(&FakeAbandoner{})

						recorder := SendMatchAbandoned(router, tt.body)

						AssertErrorResponse(t, recorder, 400, tt.message)
					})
				}
			})

			t.Run("本文を読み取れないとき", func(t *testing.T) {
				tests := []struct {
					name string
					body string
				}{
					{"本文がJSONとして壊れているとき、応答は、本文が不正であることを示す400のエラーになる", `{"match_id":`},
					{"本文が空のとき、応答は、本文が不正であることを示す400のエラーになる", ``},
					{"player_idsが文字列のTST-PLAYER-1のとき、応答は、本文が不正であることを示す400のエラーになる", AbandonBody(Set("player_ids", `"TST-PLAYER-1"`))},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						router := NewServerWithAbandoner(&FakeAbandoner{})

						recorder := SendMatchAbandoned(router, tt.body)

						AssertErrorResponse(t, recorder, 400, "request body is not valid")
					})
				}
			})

			t.Run("本文がすべて正しく、マッチの破棄がエラーになるとき、応答のステータスが、一時的に利用できないことを意味する503になる", func(t *testing.T) {
				router := NewServerWithAbandoner(&FakeAbandoner{Err: errors.New("TST-abandon-down")})

				recorder := SendMatchAbandoned(router, AbandonBody())

				assert.Equal(t, 503, recorder.Code)
			})
		})
	})
}
