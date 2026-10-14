package matcher_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher/matchertest"
)

func addThreePairs(s *scenario) {
	s.queue.AddPops(
		pairOfPlayers("player-a", "player-b"),
		pairOfPlayers("player-c", "player-d"),
		pairOfPlayers("player-e", "player-f"),
	)
}

func TestMatcherLoop(t *testing.T) {
	t.Run("[マッチ成立]定期処理の繰り返し実行", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("キューにplayer-aとplayer-b、player-cとplayer-d、player-eとplayer-fの3組のペアがあるとき、定期処理の繰り返しを開始すると", func(t *testing.T) {
				startWithThreePairs := func(t *testing.T) *scenario {
					t.Helper()
					s := newDefaultScenario(t)
					addThreePairs(s)
					s.start(t)
					for range 3 {
						s.tick(t)
					}
					s.waitForPublications(t, 3)
					s.settle(t)
					return s
				}

				t.Run("テスト用の発行先に記録された発行は3件になる", func(t *testing.T) {
					s := startWithThreePairs(t)

					assert.Len(t, s.publisher.Publications(), 3)
				})

				tests := []struct {
					name  string
					index int
					want  []string
				}{
					{"1件目のテスト用の発行先に記録された発行の本文のplayersは、player-a、player-bの順になる", 0, []string{"player-a", "player-b"}},
					{"2件目のテスト用の発行先に記録された発行の本文のplayersは、player-c、player-dの順になる", 1, []string{"player-c", "player-d"}},
					{"3件目のテスト用の発行先に記録された発行の本文のplayersは、player-e、player-fの順になる", 2, []string{"player-e", "player-f"}},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						s := startWithThreePairs(t)

						assert.Equal(t, tt.want, playerIDsOf(s.matchMadeEvent(t, tt.index)))
					})
				}
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("キューにplayer-aとplayer-b、player-cとplayer-d、player-eとplayer-fの3組のペアがあり、1組目の発行が完了しないとき、定期処理の繰り返しを開始すると", func(t *testing.T) {
				startWithBlockedFirstPublication := func(t *testing.T) (*scenario, *matchertest.Gate) {
					t.Helper()
					gate := matchertest.NewGate()
					s := newDefaultScenario(t)
					addThreePairs(s)
					s.publisher.AddSteps(matchertest.PublishBlockedUntil(gate))
					s.start(t)
					t.Cleanup(gate.Open)
					s.tick(t)
					s.waitForPublications(t, 1)
					return s, gate
				}

				t.Run("定期処理の実行間隔が5回経過しても、キューに残るペアは2組のままになる", func(t *testing.T) {
					s, _ := startWithBlockedFirstPublication(t)

					for range 5 {
						s.clock.Advance(s.interval)
					}
					s.settle(t)

					assert.Equal(t, 2, s.queue.RemainingPairs())
				})

				t.Run("1組目の発行が完了し、定期処理の実行間隔がさらに2回経過すると、テスト用の発行先に記録された発行は3件になる", func(t *testing.T) {
					s, gate := startWithBlockedFirstPublication(t)

					gate.Open()
					for range 2 {
						s.tick(t)
					}
					s.waitForPublications(t, 3)
					s.settle(t)

					assert.Len(t, s.publisher.Publications(), 3)
				})

				t.Run("1組目の発行が完了し、定期処理の実行間隔がさらに2回経過すると、3件目のテスト用の発行先に記録された発行の本文のplayersは、player-e、player-fの順になる", func(t *testing.T) {
					s, gate := startWithBlockedFirstPublication(t)

					gate.Open()
					for range 2 {
						s.tick(t)
					}
					s.waitForPublications(t, 3)
					s.settle(t)

					assert.Equal(t, []string{"player-e", "player-f"}, playerIDsOf(s.matchMadeEvent(t, 2)))
				})
			})
		})
	})
}
