//go:build integration

package httphandler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler/httphandlertest"
)

func TestQueueSize(t *testing.T) {
	t.Run("[マッチメイキングキュー]キューサイズ", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("追加と除外の要求には有効なトークンを付けるとき", func(t *testing.T) {
				t.Run("キューが空のとき", func(t *testing.T) {
					t.Run("キューサイズ取得の応答のステータスが200になる", func(t *testing.T) {
						router := NewServerWithRealQueue(t)

						recorder := SendQueueSize(router)

						assert.Equal(t, 200, recorder.Code)
					})

					t.Run("キューサイズ取得の応答本文のsizeが0になる", func(t *testing.T) {
						router := NewServerWithRealQueue(t)

						assert.Equal(t, 0, ReadQueueSize(t, router))
					})

					t.Run("TST-PLAYER-1を1回追加したとき", func(t *testing.T) {
						t.Run("追加の応答のステータスが202になる", func(t *testing.T) {
							router := NewServerWithRealQueue(t)

							recorder := SendEnqueueFromGateway(router, AuthPlayer1, GatewayID1)

							assert.Equal(t, 202, recorder.Code)
						})

						t.Run("キューサイズ取得の応答本文のsizeが1になる", func(t *testing.T) {
							router := NewServerWithRealQueue(t)
							SendEnqueueFromGateway(router, AuthPlayer1, GatewayID1)

							assert.Equal(t, 1, ReadQueueSize(t, router))
						})
					})

					t.Run("TST-PLAYER-1とTST-PLAYER-2を同じgateway_instance_idで追加したとき", func(t *testing.T) {
						givenTwoPlayersEnqueued := func(t *testing.T) http.Handler {
							router := NewServerWithRealQueue(t)
							SendEnqueueFromGateway(router, AuthPlayer1, GatewayID1)
							SendEnqueueFromGateway(router, AuthPlayer2, GatewayID1)
							return router
						}

						t.Run("キューサイズ取得の応答本文のsizeが2になる", func(t *testing.T) {
							router := givenTwoPlayersEnqueued(t)

							assert.Equal(t, 2, ReadQueueSize(t, router))
						})

						t.Run("続けてTST-PLAYER-1をキューから除外したとき", func(t *testing.T) {
							t.Run("除外の応答のステータスが200になる", func(t *testing.T) {
								router := givenTwoPlayersEnqueued(t)

								recorder := SendCancel(router, AuthPlayer1)

								assert.Equal(t, 200, recorder.Code)
							})

							t.Run("キューサイズ取得の応答本文のsizeが1になる", func(t *testing.T) {
								router := givenTwoPlayersEnqueued(t)
								SendCancel(router, AuthPlayer1)

								assert.Equal(t, 1, ReadQueueSize(t, router))
							})
						})
					})

					t.Run("TST-PLAYER-1をTST-GATEWAY-1で追加したあとに、TST-PLAYER-2をTST-GATEWAY-2で追加したとき", func(t *testing.T) {
						givenPlayersEnqueuedFromDifferentGateways := func(t *testing.T) http.Handler {
							router := NewServerWithRealQueue(t)
							SendEnqueueFromGateway(router, AuthPlayer1, GatewayID1)
							SendEnqueueFromGateway(router, AuthPlayer2, GatewayID2)
							return router
						}

						t.Run("キューサイズ取得の応答本文のsizeが1になる", func(t *testing.T) {
							router := givenPlayersEnqueuedFromDifferentGateways(t)

							assert.Equal(t, 1, ReadQueueSize(t, router))
						})

						t.Run("TST-PLAYER-1のキューからの除外の応答のステータスが、キューに待機していないことを意味する404になる", func(t *testing.T) {
							router := givenPlayersEnqueuedFromDifferentGateways(t)

							recorder := SendCancel(router, AuthPlayer1)

							assert.Equal(t, 404, recorder.Code)
						})

						t.Run("TST-PLAYER-2のキューからの除外の応答のステータスが200になる", func(t *testing.T) {
							router := givenPlayersEnqueuedFromDifferentGateways(t)

							recorder := SendCancel(router, AuthPlayer2)

							assert.Equal(t, 200, recorder.Code)
						})
					})

					t.Run("TST-PLAYER-1・TST-PLAYER-2・TST-PLAYER-3を同じgateway_instance_idで追加したあとに、TST-PLAYER-1とTST-PLAYER-2を不成立として申告したとき", func(t *testing.T) {
						givenFirstTwoPlayersAbandoned := func(t *testing.T) (http.Handler, *httptest.ResponseRecorder) {
							router := NewServerWithRealQueue(t)
							SendEnqueueFromGateway(router, AuthPlayer1, GatewayID1)
							SendEnqueueFromGateway(router, AuthPlayer2, GatewayID1)
							SendEnqueueFromGateway(router, AuthPlayer3, GatewayID1)
							recorder := SendMatchAbandoned(router, AbandonBody(PlayerIDs(PlayerID1, PlayerID2)))
							return router, recorder
						}

						t.Run("申告の応答のステータスが204になる", func(t *testing.T) {
							_, recorder := givenFirstTwoPlayersAbandoned(t)

							assert.Equal(t, 204, recorder.Code)
						})

						t.Run("キューサイズ取得の応答本文のsizeが1になる", func(t *testing.T) {
							router, _ := givenFirstTwoPlayersAbandoned(t)

							assert.Equal(t, 1, ReadQueueSize(t, router))
						})

						t.Run("TST-PLAYER-3のキューからの除外の応答のステータスが200になる", func(t *testing.T) {
							router, _ := givenFirstTwoPlayersAbandoned(t)

							recorder := SendCancel(router, AuthPlayer3)

							assert.Equal(t, 200, recorder.Code)
						})
					})
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("キューが空のとき", func(t *testing.T) {
				t.Run("追加と除外の要求には有効なトークンを付けるとき", func(t *testing.T) {
					t.Run("TST-PLAYER-1を同じgateway_instance_idで2回続けて追加したとき", func(t *testing.T) {
						givenSamePlayerEnqueuedTwice := func(t *testing.T) (http.Handler, *httptest.ResponseRecorder) {
							router := NewServerWithRealQueue(t)
							SendEnqueueFromGateway(router, AuthPlayer1, GatewayID1)
							second := SendEnqueueFromGateway(router, AuthPlayer1, GatewayID1)
							return router, second
						}

						t.Run("2回目の追加の応答のステータスが202になる", func(t *testing.T) {
							_, second := givenSamePlayerEnqueuedTwice(t)

							assert.Equal(t, 202, second.Code)
						})

						t.Run("キューサイズ取得の応答本文のsizeが1になる", func(t *testing.T) {
							router, _ := givenSamePlayerEnqueuedTwice(t)

							assert.Equal(t, 1, ReadQueueSize(t, router))
						})
					})

					t.Run("TST-PLAYER-1とTST-PLAYER-2を同じgateway_instance_idで追加したあとに、TST-PLAYER-1とTST-PLAYER-2を不成立として同じ申告で2回続けて送ったとき", func(t *testing.T) {
						givenSameReportSentTwice := func(t *testing.T) (http.Handler, *httptest.ResponseRecorder, *httptest.ResponseRecorder) {
							router := NewServerWithRealQueue(t)
							SendEnqueueFromGateway(router, AuthPlayer1, GatewayID1)
							SendEnqueueFromGateway(router, AuthPlayer2, GatewayID1)
							body := AbandonBody(PlayerIDs(PlayerID1, PlayerID2))
							first := SendMatchAbandoned(router, body)
							second := SendMatchAbandoned(router, body)
							return router, first, second
						}

						t.Run("1回目の申告の応答のステータスが204になる", func(t *testing.T) {
							_, first, _ := givenSameReportSentTwice(t)

							assert.Equal(t, 204, first.Code)
						})

						t.Run("2回目の申告の応答のステータスが204になる", func(t *testing.T) {
							_, _, second := givenSameReportSentTwice(t)

							assert.Equal(t, 204, second.Code)
						})

						t.Run("2回目の申告のあとのキューサイズ取得の応答本文のsizeが0になる", func(t *testing.T) {
							router, _, _ := givenSameReportSentTwice(t)

							assert.Equal(t, 0, ReadQueueSize(t, router))
						})
					})

					t.Run("TST-PLAYER-1の除外を要求するとき", func(t *testing.T) {
						t.Run("応答のステータスが、キューに待機していないことを意味する404になる", func(t *testing.T) {
							router := NewServerWithRealQueue(t)

							recorder := SendCancel(router, AuthPlayer1)

							assert.Equal(t, 404, recorder.Code)
						})

						t.Run("キューサイズ取得の応答本文のsizeが0のままになる", func(t *testing.T) {
							router := NewServerWithRealQueue(t)
							SendCancel(router, AuthPlayer1)

							assert.Equal(t, 0, ReadQueueSize(t, router))
						})
					})

					t.Run("TST-PLAYER-1の除外を要求して404になったあとに、TST-PLAYER-1を追加するとき", func(t *testing.T) {
						givenEnqueuedAfterFailedCancel := func(t *testing.T) (http.Handler, *httptest.ResponseRecorder) {
							router := NewServerWithRealQueue(t)
							SendCancel(router, AuthPlayer1)
							enqueue := SendEnqueueFromGateway(router, AuthPlayer1, GatewayID1)
							return router, enqueue
						}

						t.Run("追加の応答のステータスが202になる", func(t *testing.T) {
							_, enqueue := givenEnqueuedAfterFailedCancel(t)

							assert.Equal(t, 202, enqueue.Code)
						})

						t.Run("キューサイズ取得の応答本文のsizeが1になる", func(t *testing.T) {
							router, _ := givenEnqueuedAfterFailedCancel(t)

							assert.Equal(t, 1, ReadQueueSize(t, router))
						})

						t.Run("TST-PLAYER-1のキューからの除外の応答のステータスが200になる", func(t *testing.T) {
							router, _ := givenEnqueuedAfterFailedCancel(t)

							recorder := SendCancel(router, AuthPlayer1)

							assert.Equal(t, 200, recorder.Code)
						})
					})

					t.Run("追加を要求するとき", func(t *testing.T) {
						tests := []struct {
							name string
							body string
						}{
							{"本文のdeck_idだけが0のとき、キューサイズ取得の応答本文のsizeが0のままになる", EnqueueBody(Set("deck_id", `0`))},
							{"本文のdeck_idだけが負の値の-1のとき、キューサイズ取得の応答本文のsizeが0のままになる", EnqueueBody(Set("deck_id", `-1`))},
							{"本文のdeck_idのキーだけが無いとき、キューサイズ取得の応答本文のsizeが0のままになる", EnqueueBody(Omit("deck_id"))},
							{"本文のnameだけが空文字のとき、キューサイズ取得の応答本文のsizeが0のままになる", EnqueueBody(Set("name", `""`))},
							{"本文のnameだけが空白だけのとき、キューサイズ取得の応答本文のsizeが0のままになる", EnqueueBody(Set("name", `" "`))},
							{"本文のnameのキーだけが無いとき、キューサイズ取得の応答本文のsizeが0のままになる", EnqueueBody(Omit("name"))},
							{"本文のgateway_instance_idだけが空文字のとき、キューサイズ取得の応答本文のsizeが0のままになる", EnqueueBody(Set("gateway_instance_id", `""`))},
							{"本文のgateway_instance_idだけが空白だけのとき、キューサイズ取得の応答本文のsizeが0のままになる", EnqueueBody(Set("gateway_instance_id", `" "`))},
							{"本文のgateway_instance_idのキーだけが無いとき、キューサイズ取得の応答本文のsizeが0のままになる", EnqueueBody(Omit("gateway_instance_id"))},
							{"本文がJSONとして壊れているとき、キューサイズ取得の応答本文のsizeが0のままになる", `{"deck_id":`},
							{"本文が空のとき、キューサイズ取得の応答本文のsizeが0のままになる", ``},
							{"deck_idが文字列の11のとき、キューサイズ取得の応答本文のsizeが0のままになる", EnqueueBody(Set("deck_id", `"11"`))},
						}
						for _, tt := range tests {
							t.Run(tt.name, func(t *testing.T) {
								router := NewServerWithRealQueue(t)
								SendEnqueue(router, AuthPlayer1, tt.body)

								assert.Equal(t, 0, ReadQueueSize(t, router))
							})
						}
					})
				})

				t.Run("1回目の追加の要求が失敗したあとに、2回目に有効なトークンで正しい本文の追加を要求するとき", func(t *testing.T) {
					tests := []struct {
						name string
						auth Auth
						body string
					}{
						{"1回目がdeck_idが0の本文の追加の要求だったとき", AuthPlayer1, EnqueueBody(Set("deck_id", `0`))},
						{"1回目がX-Internal-Authヘッダを付けない追加の要求だったとき", NoAuth, EnqueueBody()},
					}
					givenSecondEnqueueAfterFailure := func(t *testing.T, firstAuth Auth, firstBody string) (http.Handler, *httptest.ResponseRecorder) {
						router := NewServerWithRealQueue(t)
						SendEnqueue(router, firstAuth, firstBody)
						second := SendEnqueue(router, AuthPlayer1, EnqueueBody())
						return router, second
					}
					for _, tt := range tests {
						t.Run(tt.name+"、2回目の追加の応答のステータスが202になる", func(t *testing.T) {
							_, second := givenSecondEnqueueAfterFailure(t, tt.auth, tt.body)

							assert.Equal(t, 202, second.Code)
						})
					}
					for _, tt := range tests {
						t.Run(tt.name+"、キューサイズ取得の応答本文のsizeが1になる", func(t *testing.T) {
							router, _ := givenSecondEnqueueAfterFailure(t, tt.auth, tt.body)

							assert.Equal(t, 1, ReadQueueSize(t, router))
						})
					}
				})
			})

			t.Run("TST-PLAYER-3だけが待機しているときに、待機していないTST-PLAYER-1とTST-PLAYER-2を不成立として申告したとき", func(t *testing.T) {
				givenUnqueuedPlayersAbandoned := func(t *testing.T) (http.Handler, *httptest.ResponseRecorder) {
					router := NewServerWithRealQueue(t)
					SendEnqueueFromGateway(router, AuthPlayer3, GatewayID1)
					recorder := SendMatchAbandoned(router, AbandonBody(PlayerIDs(PlayerID1, PlayerID2)))
					return router, recorder
				}

				t.Run("申告の応答のステータスが204になる", func(t *testing.T) {
					_, recorder := givenUnqueuedPlayersAbandoned(t)

					assert.Equal(t, 204, recorder.Code)
				})

				t.Run("キューサイズ取得の応答本文のsizeが1のままになる", func(t *testing.T) {
					router, _ := givenUnqueuedPlayersAbandoned(t)

					assert.Equal(t, 1, ReadQueueSize(t, router))
				})
			})

			t.Run("TST-PLAYER-1とTST-PLAYER-2が同じgateway_instance_idで待機しているときに、不成立を申告するとき", func(t *testing.T) {
				tests := []struct {
					name string
					body string
				}{
					{"本文のmatch_idだけが空文字のとき、キューサイズ取得の応答本文のsizeが2のままになる", AbandonBody(Set("match_id", `""`))},
					{"本文のmatch_idのキーだけが無いとき、キューサイズ取得の応答本文のsizeが2のままになる", AbandonBody(Omit("match_id"))},
					{"本文のplayer_idsのキーだけが無いとき、キューサイズ取得の応答本文のsizeが2のままになる", AbandonBody(Omit("player_ids"))},
					{"本文のplayer_idsだけが空の配列のとき、キューサイズ取得の応答本文のsizeが2のままになる", AbandonBody(PlayerIDs())},
					{"本文のplayer_idsだけが1件のとき、キューサイズ取得の応答本文のsizeが2のままになる", AbandonBody(PlayerIDs(PlayerID1))},
					{"本文のplayer_idsだけが3件のとき、キューサイズ取得の応答本文のsizeが2のままになる", AbandonBody(PlayerIDs(PlayerID1, PlayerID2, PlayerID3))},
					{"本文のplayer_idsの1つ目だけが空文字のとき、キューサイズ取得の応答本文のsizeが2のままになる", AbandonBody(PlayerIDs("", PlayerID2))},
					{"本文のplayer_idsの2つ目だけが空文字のとき、キューサイズ取得の応答本文のsizeが2のままになる", AbandonBody(PlayerIDs(PlayerID1, ""))},
					{"本文のplayer_idsだけが、二つの識別子がともにTST-PLAYER-1のとき、キューサイズ取得の応答本文のsizeが2のままになる", AbandonBody(PlayerIDs(PlayerID1, PlayerID1))},
					{"本文のreasonだけが空文字のとき、キューサイズ取得の応答本文のsizeが2のままになる", AbandonBody(Set("reason", `""`))},
					{"本文のreasonのキーだけが無いとき、キューサイズ取得の応答本文のsizeが2のままになる", AbandonBody(Omit("reason"))},
					{"本文のreasonだけがunknown_reasonのとき、キューサイズ取得の応答本文のsizeが2のままになる", AbandonBody(Set("reason", `"unknown_reason"`))},
					{"本文がJSONとして壊れているとき、キューサイズ取得の応答本文のsizeが2のままになる", `{"match_id":`},
					{"本文が空のとき、キューサイズ取得の応答本文のsizeが2のままになる", ``},
					{"player_idsが文字列のTST-PLAYER-1のとき、キューサイズ取得の応答本文のsizeが2のままになる", AbandonBody(Set("player_ids", `"TST-PLAYER-1"`))},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						router := NewServerWithRealQueue(t)
						SendEnqueueFromGateway(router, AuthPlayer1, GatewayID1)
						SendEnqueueFromGateway(router, AuthPlayer2, GatewayID1)
						SendMatchAbandoned(router, tt.body)

						assert.Equal(t, 2, ReadQueueSize(t, router))
					})
				}
			})

			t.Run("TST-PLAYER-1が待機しているとき", func(t *testing.T) {
				givenPlayer1Waiting := func(t *testing.T) http.Handler {
					router := NewServerWithRealQueue(t)
					SendEnqueueFromGateway(router, AuthPlayer1, GatewayID1)
					return router
				}
				tests := []struct {
					name string
					auth Auth
				}{
					{"X-Internal-Authヘッダを付けないとき、キューサイズ取得の応答本文のsizeが1のままになる", NoAuth},
					{"検証に失敗するトークンを付けたとき、キューサイズ取得の応答本文のsizeが1のままになる", AuthInvalid},
				}

				t.Run("TST-PLAYER-2の追加を要求するとき", func(t *testing.T) {
					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							router := givenPlayer1Waiting(t)
							SendEnqueueFromGateway(router, tt.auth, GatewayID1)

							assert.Equal(t, 1, ReadQueueSize(t, router))
						})
					}
				})

				t.Run("TST-PLAYER-1の除外を要求するとき", func(t *testing.T) {
					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							router := givenPlayer1Waiting(t)
							SendCancel(router, tt.auth)

							assert.Equal(t, 1, ReadQueueSize(t, router))
						})
					}
				})
			})
		})
	})
}
