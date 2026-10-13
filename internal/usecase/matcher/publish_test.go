package matcher_test

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher/matchertest"
)

func publishOnePair(t *testing.T, startedAt time.Time) *scenario {
	t.Helper()
	s := newScenario(t, defaultOptions(), startedAt)
	s.queue.AddPops(pairOfPlayerAAndPlayerB())
	s.start(t)
	s.tick(t)
	s.waitForPublications(t, 1)
	s.settle(t)
	return s
}

func TestMatchMadeEventPublication(t *testing.T) {
	t.Run("[マッチ成立]match_madeイベントの発行", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("ペアが取り出されるとき、定期処理の繰り返しを開始すると", func(t *testing.T) {
				t.Run("テスト用の発行先に記録された発行のイベントの種別はmatch_madeになる", func(t *testing.T) {
					s := publishOnePair(t, scenarioStartedAt)

					assert.Equal(t, "match_made", s.publisher.Publications()[0].EventType)
				})

				t.Run("テスト用の発行先に記録された発行の本文のplayersは2件になる", func(t *testing.T) {
					s := publishOnePair(t, scenarioStartedAt)

					assert.Len(t, s.matchMadeEvent(t, 0).Players, 2)
				})

				t.Run("テスト用の発行先に記録された発行の本文のmatch_idはmch_に続く26文字のULIDになる", func(t *testing.T) {
					s := publishOnePair(t, scenarioStartedAt)

					assert.Regexp(t, `^mch_[0-9A-HJKMNP-TV-Z]{26}$`, s.matchMadeEvent(t, 0).MatchID)
				})

				t.Run("現在時刻が2026-01-01T00:00:00Zのとき、テスト用の発行先に記録された発行の本文のmatch_idのULIDの時刻部分は2026-01-01T00:00:00Zになる", func(t *testing.T) {
					s := publishOnePair(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Hour))

					matchID := ulid.MustParse(strings.TrimPrefix(s.matchMadeEvent(t, 0).MatchID, "mch_"))
					assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UnixMilli(int64(matchID.Time())).UTC())
				})

				t.Run("1人目がplayer-a・deck_idが101・nameがalice・levelが7、2人目がplayer-b・deck_idが202・nameがbob・levelが12のとき", func(t *testing.T) {
					tests := []struct {
						name   string
						actual func(event apimatchmaking.MatchMadeEvent) any
						want   any
					}{
						{"テスト用の発行先に記録された発行の本文の1人目のplayer_idはplayer-aになる", func(e apimatchmaking.MatchMadeEvent) any { return e.Players[0].PlayerID }, "player-a"},
						{"テスト用の発行先に記録された発行の本文の1人目のdeck_idは101になる", func(e apimatchmaking.MatchMadeEvent) any { return e.Players[0].DeckID }, int64(101)},
						{"テスト用の発行先に記録された発行の本文の1人目のnameはaliceになる", func(e apimatchmaking.MatchMadeEvent) any { return e.Players[0].Name }, "alice"},
						{"テスト用の発行先に記録された発行の本文の1人目のlevelは7になる", func(e apimatchmaking.MatchMadeEvent) any { return e.Players[0].Level }, int64(7)},
						{"テスト用の発行先に記録された発行の本文の2人目のplayer_idはplayer-bになる", func(e apimatchmaking.MatchMadeEvent) any { return e.Players[1].PlayerID }, "player-b"},
						{"テスト用の発行先に記録された発行の本文の2人目のdeck_idは202になる", func(e apimatchmaking.MatchMadeEvent) any { return e.Players[1].DeckID }, int64(202)},
						{"テスト用の発行先に記録された発行の本文の2人目のnameはbobになる", func(e apimatchmaking.MatchMadeEvent) any { return e.Players[1].Name }, "bob"},
						{"テスト用の発行先に記録された発行の本文の2人目のlevelは12になる", func(e apimatchmaking.MatchMadeEvent) any { return e.Players[1].Level }, int64(12)},
					}
					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							s := publishOnePair(t, scenarioStartedAt)

							assert.Equal(t, tt.want, tt.actual(s.matchMadeEvent(t, 0)))
						})
					}
				})
			})

			t.Run("2組のペアが続けて取り出されるとき、定期処理の繰り返しを開始すると、1件目のテスト用の発行先に記録された発行の本文のmatch_idと、2件目のテスト用の発行先に記録された発行の本文のmatch_idは異なる", func(t *testing.T) {
				s := newDefaultScenario(t)
				s.start(t)
				s.queue.AddPops(pairOfPlayers("player-a", "player-b"))
				s.tick(t)
				s.queue.AddPops(pairOfPlayers("player-c", "player-d"))
				s.tick(t)
				s.waitForPublications(t, 2)

				assert.NotEqual(t, s.matchMadeEvent(t, 0).MatchID, s.matchMadeEvent(t, 1).MatchID)
			})

			t.Run("定期処理の繰り返しを開始すると", func(t *testing.T) {
				publishedTests := []struct {
					name             string
					emptyTicksBefore int
				}{
					{"ペアが取り出されるとき、テスト用の発行先に記録された発行は1件になる", 0},
					{"取り出されたペアが無い定期処理のあとに、ペアが取り出されるとき、テスト用の発行先に記録された発行は1件になる", 1},
				}
				for _, tt := range publishedTests {
					t.Run(tt.name, func(t *testing.T) {
						s := newDefaultScenario(t)
						s.start(t)
						for range tt.emptyTicksBefore {
							s.tick(t)
						}
						s.queue.AddPops(pairOfPlayerAAndPlayerB())
						s.tick(t)
						s.waitForPublications(t, 1)
						s.settle(t)

						assert.Len(t, s.publisher.Publications(), 1)
					})
				}

				notReenqueuedTests := []struct {
					name string
					pops []matchertest.PopStep
				}{
					{"ペアが取り出され、発行が成功するとき、テスト用のキューに記録された書き戻しの要求は0件になる", []matchertest.PopStep{pairOfPlayerAAndPlayerB()}},
					{"取り出されたペアが無いとき、テスト用のキューに記録された書き戻しの要求は0件になる", nil},
				}
				for _, tt := range notReenqueuedTests {
					t.Run(tt.name, func(t *testing.T) {
						s := newDefaultScenario(t)
						s.queue.AddPops(tt.pops...)
						s.start(t)
						s.tick(t)
						s.waitForPops(t, 1)
						s.settle(t)

						assert.Empty(t, s.queue.ReenqueueRequests())
					})
				}

				t.Run("取り出されたペアが無いとき、テスト用の発行先に記録された発行は0件になる", func(t *testing.T) {
					s := newDefaultScenario(t)
					s.start(t)
					s.tick(t)
					s.waitForPops(t, 1)
					s.settle(t)

					assert.Empty(t, s.publisher.Publications())
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("取り出しがエラーを返すとき、定期処理の繰り返しを開始すると", func(t *testing.T) {
				startWithFailingPop := func(t *testing.T) *scenario {
					t.Helper()
					s := newDefaultScenario(t)
					s.queue.AddPops(matchertest.PopFails(errPopFailed))
					s.start(t)
					s.tick(t)
					s.waitForPops(t, 1)
					s.settle(t)
					return s
				}

				t.Run("テスト用の発行先に記録された発行は0件になる", func(t *testing.T) {
					s := startWithFailingPop(t)

					assert.Empty(t, s.publisher.Publications())
				})

				t.Run("テスト用のキューに記録された書き戻しの要求は0件になる", func(t *testing.T) {
					s := startWithFailingPop(t)

					assert.Empty(t, s.queue.ReenqueueRequests())
				})

				t.Run("取り出しの失敗を知らせるErrorログが1件出力される", func(t *testing.T) {
					s := startWithFailingPop(t)

					assert.Len(t, s.logs.Entries(slog.LevelError, "matcher pop pair failed"), 1)
				})

				t.Run("取り出しの失敗を知らせるログのエラーの内容に、取り出しが返したエラーのメッセージが含まれる", func(t *testing.T) {
					s := startWithFailingPop(t)

					entries := s.logs.Entries(slog.LevelError, "matcher pop pair failed")
					require.Len(t, entries, 1)
					assert.Contains(t, entries[0].Text("error"), "redis unavailable while popping")
				})
			})

			t.Run("取り出しがエラーを返した定期処理のあとに、ペアが取り出されるとき、定期処理の繰り返しを開始すると、テスト用の発行先に記録された発行は1件になる", func(t *testing.T) {
				s := newDefaultScenario(t)
				s.start(t)
				s.queue.AddPops(matchertest.PopFails(errPopFailed))
				s.tick(t)
				s.queue.AddPops(pairOfPlayerAAndPlayerB())
				s.tick(t)
				s.waitForPublications(t, 1)
				s.settle(t)

				assert.Len(t, s.publisher.Publications(), 1)
			})
		})
	})
}
