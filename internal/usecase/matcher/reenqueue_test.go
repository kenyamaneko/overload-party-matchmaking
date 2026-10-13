package matcher_test

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher/matchertest"
)

var (
	errPublishFailed             = errors.New("publish unavailable")
	errReenqueueFailed           = errors.New("reenqueue unavailable")
	errFirstReenqueueInterrupted = errors.New("reenqueue interrupted at first attempt")
	errFinalReenqueueRefused     = errors.New("reenqueue refused at final attempt")
)

var reenqueueWaits = []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond}

const (
	givenFirstSucceeds         = "書き戻しが1回目で成功するとき"
	givenFirstSkipped          = "書き戻しが1回目で、gateway_instance_idがgw-1から変わっていて書き戻さなかったとき"
	givenSecondSucceeds        = "書き戻しが1回目だけエラーを返し、2回目に成功したとき"
	givenThirdSkipped          = "書き戻しが2回続けてエラーを返し、3回目で、gateway_instance_idがgw-1から変わっていて書き戻さなかったとき"
	givenFifthSucceeds         = "書き戻しが4回続けてエラーを返し、5回目に成功したとき"
	givenFiveFailures          = "書き戻しが5回続けてエラーを返したとき"
	givenShutdownDuringFirst   = "1回目の書き戻しの実行中にシャットダウンが始まるとき"
	givenShutdownDuringFailed  = "1回目の書き戻しの実行中にシャットダウンが始まり、その書き戻しがエラーを返したとき"
	givenShutdownDuringWaiting = "1回目の書き戻しが失敗した後、次の書き戻しまでの待ち時間の途中でシャットダウンが始まるとき"
)

type reenqueuePlan struct {
	steps    func(shutdown func()) []matchertest.ReenqueueStep
	drive    func(t *testing.T, s *scenario)
	requests int
}

type reenqueueRow struct {
	name string
	plan func() reenqueuePlan
}

func repeatReenqueueFailure(count int) []matchertest.ReenqueueStep {
	steps := make([]matchertest.ReenqueueStep, 0, count)
	for range count {
		steps = append(steps, matchertest.ReenqueueFails(errReenqueueFailed))
	}
	return steps
}

func driveNothing(*testing.T, *scenario) {}

func driveRetryWaits(failures int) func(t *testing.T, s *scenario) {
	return func(t *testing.T, s *scenario) {
		t.Helper()
		for _, wait := range reenqueueWaits[:failures] {
			s.waitForTimers(t, 1)
			s.clock.Advance(wait)
		}
	}
}

func driveShutdownDuringWait(t *testing.T, s *scenario) {
	t.Helper()
	s.waitForTimers(t, 1)
	s.shutdown()
}

func planFirstSucceeds() reenqueuePlan {
	return reenqueuePlan{
		steps: func(func()) []matchertest.ReenqueueStep {
			return []matchertest.ReenqueueStep{matchertest.ReenqueueSucceeds()}
		},
		drive:    driveNothing,
		requests: 1,
	}
}

func planFirstSkipped() reenqueuePlan {
	return reenqueuePlan{
		steps: func(func()) []matchertest.ReenqueueStep {
			return []matchertest.ReenqueueStep{matchertest.ReenqueueSkipped()}
		},
		drive:    driveNothing,
		requests: 1,
	}
}

func planSecondSucceedsAfterFailure() reenqueuePlan {
	return reenqueuePlan{
		steps: func(func()) []matchertest.ReenqueueStep {
			return append(repeatReenqueueFailure(1), matchertest.ReenqueueSucceeds())
		},
		drive:    driveRetryWaits(1),
		requests: 2,
	}
}

func planThirdSkippedAfterTwoFailures() reenqueuePlan {
	return reenqueuePlan{
		steps: func(func()) []matchertest.ReenqueueStep {
			return append(repeatReenqueueFailure(2), matchertest.ReenqueueSkipped())
		},
		drive:    driveRetryWaits(2),
		requests: 3,
	}
}

func planFifthSucceedsAfterFourFailures() reenqueuePlan {
	return reenqueuePlan{
		steps: func(func()) []matchertest.ReenqueueStep {
			return append(repeatReenqueueFailure(4), matchertest.ReenqueueSucceeds())
		},
		drive:    driveRetryWaits(4),
		requests: 5,
	}
}

func planFiveFailures() reenqueuePlan {
	return reenqueuePlan{
		steps:    func(func()) []matchertest.ReenqueueStep { return repeatReenqueueFailure(5) },
		drive:    driveRetryWaits(4),
		requests: 5,
	}
}

func planShutdownDuringFirstAttempt(second matchertest.ReenqueueStep) reenqueuePlan {
	return reenqueuePlan{
		steps: func(shutdown func()) []matchertest.ReenqueueStep {
			return []matchertest.ReenqueueStep{matchertest.ReenqueueFailsAfterShutdown(shutdown, errFirstReenqueueInterrupted), second}
		},
		drive:    driveNothing,
		requests: 2,
	}
}

func planShutdownDuringFirstAttemptThenSucceeds() reenqueuePlan {
	return planShutdownDuringFirstAttempt(matchertest.ReenqueueSucceeds())
}

func planShutdownDuringFirstAttemptThenSkipped() reenqueuePlan {
	return planShutdownDuringFirstAttempt(matchertest.ReenqueueSkipped())
}

func planShutdownDuringFirstAttemptThenFails() reenqueuePlan {
	return planShutdownDuringFirstAttempt(matchertest.ReenqueueFails(errFinalReenqueueRefused))
}

func planShutdownDuringWaitThenSucceeds() reenqueuePlan {
	return reenqueuePlan{
		steps: func(func()) []matchertest.ReenqueueStep {
			return []matchertest.ReenqueueStep{matchertest.ReenqueueFails(errReenqueueFailed), matchertest.ReenqueueSucceeds()}
		},
		drive:    driveShutdownDuringWait,
		requests: 2,
	}
}

func planFailuresThenElapsed(failures int, elapsed time.Duration, requests int) reenqueuePlan {
	return reenqueuePlan{
		steps: func(func()) []matchertest.ReenqueueStep {
			return append(repeatReenqueueFailure(failures), matchertest.ReenqueueSucceeds())
		},
		drive: func(t *testing.T, s *scenario) {
			t.Helper()
			driveRetryWaits(failures-1)(t, s)
			s.waitForTimers(t, 1)
			s.clock.Advance(elapsed)
		},
		requests: requests,
	}
}

func runReenqueuePlan(t *testing.T, plan reenqueuePlan) *scenario {
	t.Helper()
	s := newDefaultScenario(t)
	s.queue.AddPops(pairOfPlayerAAndPlayerB())
	s.publisher.AddSteps(matchertest.PublishFails(errPublishFailed))
	s.queue.AddReenqueues(plan.steps(s.shutdown)...)
	s.start(t)
	s.tick(t)
	plan.drive(t, s)
	s.waitForReenqueues(t, plan.requests)
	s.settle(t)
	return s
}

func (s *scenario) firstMatchID(t *testing.T) string {
	t.Helper()
	return s.matchMadeEvent(t, 0).MatchID
}

func TestPairReenqueue(t *testing.T) {
	t.Run("[マッチ成立]ペアのキューへの書き戻し", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("ペアが取り出され、発行がエラーを返すとき、定期処理の繰り返しを開始すると", func(t *testing.T) {
				t.Run("テスト用の発行先に記録された発行は1件になる", func(t *testing.T) {
					s := runReenqueuePlan(t, planFirstSucceeds())

					assert.Len(t, s.publisher.Publications(), 1)
				})

				t.Run("発行失敗を知らせるErrorログが1件出力される", func(t *testing.T) {
					s := runReenqueuePlan(t, planFirstSucceeds())

					assert.Len(t, s.logs.Entries(slog.LevelError, "matcher publish failed, re-enqueueing pair"), 1)
				})

				t.Run("発行失敗を知らせるログのエラーの内容に、発行が返したエラーのメッセージが含まれる", func(t *testing.T) {
					s := runReenqueuePlan(t, planFirstSucceeds())

					entries := s.logs.Entries(slog.LevelError, "matcher publish failed, re-enqueueing pair")
					require.Len(t, entries, 1)
					assert.Contains(t, entries[0].Text("error"), "publish unavailable")
				})

				oneRequestTests := []reenqueueRow{
					{givenFirstSucceeds, planFirstSucceeds},
					{givenFirstSkipped, planFirstSkipped},
				}
				for _, tt := range oneRequestTests {
					t.Run(tt.name+"、テスト用のキューに記録された書き戻しの要求は1件になる", func(t *testing.T) {
						s := runReenqueuePlan(t, tt.plan())

						assert.Len(t, s.queue.ReenqueueRequests(), 1)
					})
				}

				twoRequestTests := []reenqueueRow{
					{givenSecondSucceeds, planSecondSucceedsAfterFailure},
					{givenShutdownDuringFailed, planShutdownDuringFirstAttemptThenSucceeds},
					{givenShutdownDuringWaiting, planShutdownDuringWaitThenSucceeds},
				}
				for _, tt := range twoRequestTests {
					t.Run(tt.name+"、テスト用のキューに記録された書き戻しの要求は2件になる", func(t *testing.T) {
						s := runReenqueuePlan(t, tt.plan())

						assert.Len(t, s.queue.ReenqueueRequests(), 2)
					})
				}

				t.Run("書き戻しが2回続けてエラーを返し、3回目で、gateway_instance_idがgw-1から変わっていて書き戻さなかったとき、テスト用のキューに記録された書き戻しの要求は3件になる", func(t *testing.T) {
					s := runReenqueuePlan(t, planThirdSkippedAfterTwoFailures())

					assert.Len(t, s.queue.ReenqueueRequests(), 3)
				})

				fiveRequestTests := []reenqueueRow{
					{givenFifthSucceeds, planFifthSucceedsAfterFourFailures},
					{givenFiveFailures, planFiveFailures},
				}
				for _, tt := range fiveRequestTests {
					t.Run(tt.name+"、テスト用のキューに記録された書き戻しの要求は5件になる", func(t *testing.T) {
						s := runReenqueuePlan(t, tt.plan())

						assert.Len(t, s.queue.ReenqueueRequests(), 5)
					})
				}

				t.Run("書き戻しが失敗したあとの待ち時間", func(t *testing.T) {
					notIncreasedTests := []struct {
						name     string
						failures int
						elapsed  time.Duration
					}{
						{"1回目の書き戻しが失敗したあと、99ミリ秒が経ったとき", 1, 99 * time.Millisecond},
						{"2回目の書き戻しが失敗したあと、199ミリ秒が経ったとき", 2, 199 * time.Millisecond},
						{"3回目の書き戻しが失敗したあと、399ミリ秒が経ったとき", 3, 399 * time.Millisecond},
						{"4回目の書き戻しが失敗したあと、799ミリ秒が経ったとき", 4, 799 * time.Millisecond},
					}
					for _, tt := range notIncreasedTests {
						t.Run(tt.name+"、テスト用のキューに記録された書き戻しの要求は増えない", func(t *testing.T) {
							s := runReenqueuePlan(t, planFailuresThenElapsed(tt.failures, tt.elapsed, tt.failures))

							assert.Len(t, s.queue.ReenqueueRequests(), tt.failures)
						})
					}

					increasedTests := []struct {
						name     string
						failures int
						elapsed  time.Duration
					}{
						{"1回目の書き戻しが失敗したあと、100ミリ秒が経ったとき", 1, 100 * time.Millisecond},
						{"2回目の書き戻しが失敗したあと、200ミリ秒が経ったとき", 2, 200 * time.Millisecond},
						{"3回目の書き戻しが失敗したあと、400ミリ秒が経ったとき", 3, 400 * time.Millisecond},
						{"4回目の書き戻しが失敗したあと、800ミリ秒が経ったとき", 4, 800 * time.Millisecond},
					}
					for _, tt := range increasedTests {
						t.Run(tt.name+"、テスト用のキューに記録された書き戻しの要求が1件増える", func(t *testing.T) {
							s := runReenqueuePlan(t, planFailuresThenElapsed(tt.failures, tt.elapsed, tt.failures+1))

							assert.Len(t, s.queue.ReenqueueRequests(), tt.failures+1)
						})
					}
				})

				t.Run("書き戻しがgateway_instance_idがgw-1から変わっていて書き戻さなかったとき", func(t *testing.T) {
					skippedTests := []reenqueueRow{
						{"書き戻しが1回目だったとき", planFirstSkipped},
						{"書き戻しが2回続けてエラーを返したあとの3回目だったとき", planThirdSkippedAfterTwoFailures},
						{"1回目の書き戻しの実行中にシャットダウンが始まったあとの2件目の書き戻しだったとき", planShutdownDuringFirstAttemptThenSkipped},
					}
					for _, tt := range skippedTests {
						t.Run(tt.name+"、書き戻さずにペアを破棄したことを知らせるWarnログが1件出力される", func(t *testing.T) {
							s := runReenqueuePlan(t, tt.plan())

							assert.Len(t, s.logs.Entries(slog.LevelWarn, "matcher dropped stale pair instead of re-enqueueing"), 1)
						})
					}

					for _, tt := range skippedTests {
						t.Run(tt.name+"、書き戻さずにペアを破棄したことを知らせるログのマッチ識別子は、1件目のテスト用の発行先に記録された発行の本文のmatch_idと同じになる", func(t *testing.T) {
							s := runReenqueuePlan(t, tt.plan())

							entries := s.logs.Entries(slog.LevelWarn, "matcher dropped stale pair instead of re-enqueueing")
							require.Len(t, entries, 1)
							assert.Equal(t, s.firstMatchID(t), entries[0].Text("match_id"))
						})
					}
				})

				t.Run("書き戻しが4回続けてエラーを返し、5回目に成功したとき", func(t *testing.T) {
					t.Run("試行の失敗を知らせるWarnログは4件出力される", func(t *testing.T) {
						s := runReenqueuePlan(t, planFifthSucceedsAfterFourFailures())

						assert.Len(t, s.logs.Entries(slog.LevelWarn, "matcher re-enqueue attempt failed"), 4)
					})

					t.Run("ペアを失ったことを知らせるErrorログは出力されない", func(t *testing.T) {
						s := runReenqueuePlan(t, planFifthSucceedsAfterFourFailures())

						assert.Empty(t, s.logs.Entries(slog.LevelError, "matcher lost pair after re-enqueue attempts"))
					})
				})

				t.Run("書き戻しが5回続けてエラーを返したとき", func(t *testing.T) {
					t.Run("試行の失敗を知らせるWarnログは5件出力される", func(t *testing.T) {
						s := runReenqueuePlan(t, planFiveFailures())

						assert.Len(t, s.logs.Entries(slog.LevelWarn, "matcher re-enqueue attempt failed"), 5)
					})

					t.Run("試行の失敗を知らせるログのエラーの内容に、書き戻しが返したエラーのメッセージが含まれる", func(t *testing.T) {
						s := runReenqueuePlan(t, planFiveFailures())

						entries := s.logs.Entries(slog.LevelWarn, "matcher re-enqueue attempt failed")
						require.Len(t, entries, 5)
						for _, entry := range entries {
							assert.Contains(t, entry.Text("error"), "reenqueue unavailable")
						}
					})

					t.Run("5回目の失敗のあと時間が経たなくても、ペアを失ったことを知らせるErrorログが1件出力される", func(t *testing.T) {
						s := runReenqueuePlan(t, planFiveFailures())

						assert.Len(t, s.logs.Entries(slog.LevelError, "matcher lost pair after re-enqueue attempts"), 1)
					})

					t.Run("ペアを失ったことを知らせるログのマッチ識別子は、1件目のテスト用の発行先に記録された発行の本文のmatch_idと同じになる", func(t *testing.T) {
						s := runReenqueuePlan(t, planFiveFailures())

						entries := s.logs.Entries(slog.LevelError, "matcher lost pair after re-enqueue attempts")
						require.Len(t, entries, 1)
						assert.Equal(t, s.firstMatchID(t), entries[0].Text("match_id"))
					})
				})

				t.Run("1回目の書き戻しがエラーを返すとき", func(t *testing.T) {
					t.Run("2件目の書き戻しが成功するとき", func(t *testing.T) {
						noLossTests := []reenqueueRow{
							{givenShutdownDuringFirst, planShutdownDuringFirstAttemptThenSucceeds},
							{givenShutdownDuringWaiting, planShutdownDuringWaitThenSucceeds},
						}
						for _, tt := range noLossTests {
							t.Run(tt.name+"、シャットダウン中にペアを失ったことを知らせるErrorログは出力されない", func(t *testing.T) {
								s := runReenqueuePlan(t, tt.plan())

								assert.Empty(t, s.logs.Entries(slog.LevelError, "matcher lost pair during shutdown"))
							})
						}
					})

					t.Run("1回目の書き戻しの実行中にシャットダウンが始まり、2件目の書き戻しがエラーを返すとき", func(t *testing.T) {
						t.Run("シャットダウン中にペアを失ったことを知らせるErrorログが1件出力される", func(t *testing.T) {
							s := runReenqueuePlan(t, planShutdownDuringFirstAttemptThenFails())

							assert.Len(t, s.logs.Entries(slog.LevelError, "matcher lost pair during shutdown"), 1)
						})

						t.Run("シャットダウン中にペアを失ったことを知らせるログのマッチ識別子は、1件目のテスト用の発行先に記録された発行の本文のmatch_idと同じになる", func(t *testing.T) {
							s := runReenqueuePlan(t, planShutdownDuringFirstAttemptThenFails())

							entries := s.logs.Entries(slog.LevelError, "matcher lost pair during shutdown")
							require.Len(t, entries, 1)
							assert.Equal(t, s.firstMatchID(t), entries[0].Text("match_id"))
						})

						t.Run("シャットダウン中にペアを失ったことを知らせるログのエラーの内容に、2件目の書き戻しが返したエラーのメッセージが含まれる", func(t *testing.T) {
							s := runReenqueuePlan(t, planShutdownDuringFirstAttemptThenFails())

							entries := s.logs.Entries(slog.LevelError, "matcher lost pair during shutdown")
							require.Len(t, entries, 1)
							assert.Contains(t, entries[0].Text("error"), "reenqueue refused at final attempt")
						})
					})
				})

				t.Run("player-cとplayer-dの2組目のペアが、書き戻しの終わったあとの定期処理で取り出され、その発行が成功するとき", func(t *testing.T) {
					laterPairTests := []reenqueueRow{
						{"直前の書き戻しが成功していたとき", planFirstSucceeds},
						{"直前の書き戻しが5回続けてエラーを返していたとき", planFiveFailures},
					}
					for _, tt := range laterPairTests {
						t.Run(tt.name+"、2件目のテスト用の発行先に記録された発行の本文のplayersは、player-c、player-dの順になる", func(t *testing.T) {
							s := runReenqueuePlan(t, tt.plan())
							s.queue.AddPops(pairOfPlayers("player-c", "player-d"))
							s.tick(t)
							s.waitForPublications(t, 2)
							s.settle(t)

							assert.Equal(t, []string{"player-c", "player-d"}, playerIDsOf(s.matchMadeEvent(t, 1)))
						})
					}
				})
			})

			t.Run("1人目がplayer-a、2人目がplayer-bのペアがgateway_instance_idがgw-1で取り出され、発行がエラーを返すとき、定期処理の繰り返しを開始すると", func(t *testing.T) {
				t.Run("書き戻しが1回目で成功するとき", func(t *testing.T) {
					t.Run("テスト用のキューに記録された書き戻しの要求のキューで待つプレイヤーは、player-a、player-bの順になる", func(t *testing.T) {
						s := runReenqueuePlan(t, planFirstSucceeds())

						assert.Equal(t, []string{"player-a", "player-b"}, playerIDsOfEntries(s.queue.ReenqueueRequests()[0].Entries))
					})

					t.Run("テスト用のキューに記録された書き戻しの要求のgateway_instance_idはgw-1になる", func(t *testing.T) {
						s := runReenqueuePlan(t, planFirstSucceeds())

						assert.Equal(t, "gw-1", s.queue.ReenqueueRequests()[0].GatewayInstanceID)
					})

					t.Run("1人目がdeck_idが101・nameがalice・levelが7・待機開始時刻が2026-01-01T00:00:00Z、2人目がdeck_idが202・nameがbob・levelが12・待機開始時刻が2026-01-01T00:00:05Zのとき", func(t *testing.T) {
						preservedTests := []struct {
							name   string
							actual func(entries []domain.QueueEntry) any
							want   any
						}{
							{"テスト用のキューに記録された書き戻しの要求の1人目の待機開始時刻は2026-01-01T00:00:00Zのままになる", func(e []domain.QueueEntry) any { return e[0].JoinedAt.UTC().Format(time.RFC3339) }, "2026-01-01T00:00:00Z"},
							{"テスト用のキューに記録された書き戻しの要求の2人目の待機開始時刻は2026-01-01T00:00:05Zのままになる", func(e []domain.QueueEntry) any { return e[1].JoinedAt.UTC().Format(time.RFC3339) }, "2026-01-01T00:00:05Z"},
							{"テスト用のキューに記録された書き戻しの要求の1人目のdeck_idは101のままになる", func(e []domain.QueueEntry) any { return e[0].DeckID }, int64(101)},
							{"テスト用のキューに記録された書き戻しの要求の1人目のnameはaliceのままになる", func(e []domain.QueueEntry) any { return e[0].Name }, "alice"},
							{"テスト用のキューに記録された書き戻しの要求の1人目のlevelは7のままになる", func(e []domain.QueueEntry) any { return e[0].Level }, int64(7)},
							{"テスト用のキューに記録された書き戻しの要求の2人目のdeck_idは202のままになる", func(e []domain.QueueEntry) any { return e[1].DeckID }, int64(202)},
							{"テスト用のキューに記録された書き戻しの要求の2人目のnameはbobのままになる", func(e []domain.QueueEntry) any { return e[1].Name }, "bob"},
							{"テスト用のキューに記録された書き戻しの要求の2人目のlevelは12のままになる", func(e []domain.QueueEntry) any { return e[1].Level }, int64(12)},
						}
						for _, tt := range preservedTests {
							t.Run(tt.name, func(t *testing.T) {
								s := runReenqueuePlan(t, planFirstSucceeds())

								assert.Equal(t, tt.want, tt.actual(s.queue.ReenqueueRequests()[0].Entries))
							})
						}
					})
				})

				allRequestsTests := []reenqueueRow{
					{givenSecondSucceeds, planSecondSucceedsAfterFailure},
					{givenThirdSkipped, planThirdSkippedAfterTwoFailures},
					{givenFifthSucceeds, planFifthSucceedsAfterFourFailures},
					{givenFiveFailures, planFiveFailures},
				}
				for _, tt := range allRequestsTests {
					t.Run(tt.name+"、全てのテスト用のキューに記録された書き戻しの要求のキューで待つプレイヤーは、player-a、player-bの順になる", func(t *testing.T) {
						s := runReenqueuePlan(t, tt.plan())

						requests := s.queue.ReenqueueRequests()
						require.NotEmpty(t, requests)
						for _, request := range requests {
							assert.Equal(t, []string{"player-a", "player-b"}, playerIDsOfEntries(request.Entries))
						}
					})
				}

				for _, tt := range allRequestsTests {
					t.Run(tt.name+"、全てのテスト用のキューに記録された書き戻しの要求のgateway_instance_idはgw-1になる", func(t *testing.T) {
						s := runReenqueuePlan(t, tt.plan())

						requests := s.queue.ReenqueueRequests()
						require.NotEmpty(t, requests)
						for _, request := range requests {
							assert.Equal(t, "gw-1", request.GatewayInstanceID)
						}
					})
				}

				t.Run("書き戻しがgateway_instance_idがgw-1から変わっていて書き戻さなかったとき", func(t *testing.T) {
					skippedTests := []reenqueueRow{
						{"書き戻しが1回目だったとき", planFirstSkipped},
						{"書き戻しが2回続けてエラーを返したあとの3回目だったとき", planThirdSkippedAfterTwoFailures},
						{"1回目の書き戻しの実行中にシャットダウンが始まったあとの2件目の書き戻しだったとき", planShutdownDuringFirstAttemptThenSkipped},
					}
					for _, tt := range skippedTests {
						t.Run(tt.name+"、書き戻さずにペアを破棄したことを知らせるログのプレイヤー識別子の並びは、player-a、player-bの順になる", func(t *testing.T) {
							s := runReenqueuePlan(t, tt.plan())

							entries := s.logs.Entries(slog.LevelWarn, "matcher dropped stale pair instead of re-enqueueing")
							require.Len(t, entries, 1)
							assert.Equal(t, []string{"player-a", "player-b"}, entries[0].Texts("players"))
						})
					}
				})

				t.Run("書き戻しが5回続けてエラーを返したとき、ペアを失ったことを知らせるログのプレイヤー識別子の並びは、player-a、player-bの順になる", func(t *testing.T) {
					s := runReenqueuePlan(t, planFiveFailures())

					entries := s.logs.Entries(slog.LevelError, "matcher lost pair after re-enqueue attempts")
					require.Len(t, entries, 1)
					assert.Equal(t, []string{"player-a", "player-b"}, entries[0].Texts("players"))
				})

				t.Run("1回目の書き戻しがエラーを返すとき", func(t *testing.T) {
					shutdownTests := []reenqueueRow{
						{givenShutdownDuringFirst, planShutdownDuringFirstAttemptThenSucceeds},
						{givenShutdownDuringWaiting, planShutdownDuringWaitThenSucceeds},
					}
					for _, tt := range shutdownTests {
						t.Run(tt.name+"、2件目のテスト用のキューに記録された書き戻しの要求のキューで待つプレイヤーは、player-a、player-bの順になる", func(t *testing.T) {
							s := runReenqueuePlan(t, tt.plan())

							requests := s.queue.ReenqueueRequests()
							require.Len(t, requests, 2)
							assert.Equal(t, []string{"player-a", "player-b"}, playerIDsOfEntries(requests[1].Entries))
						})
					}

					for _, tt := range shutdownTests {
						t.Run(tt.name+"、2件目のテスト用のキューに記録された書き戻しの要求のgateway_instance_idはgw-1になる", func(t *testing.T) {
							s := runReenqueuePlan(t, tt.plan())

							requests := s.queue.ReenqueueRequests()
							require.Len(t, requests, 2)
							assert.Equal(t, "gw-1", requests[1].GatewayInstanceID)
						})
					}

					t.Run("1回目の書き戻しの実行中にシャットダウンが始まり、2件目の書き戻しがエラーを返したとき、シャットダウン中にペアを失ったことを知らせるログのプレイヤー識別子の並びは、player-a、player-bの順になる", func(t *testing.T) {
						s := runReenqueuePlan(t, planShutdownDuringFirstAttemptThenFails())

						entries := s.logs.Entries(slog.LevelError, "matcher lost pair during shutdown")
						require.Len(t, entries, 1)
						assert.Equal(t, []string{"player-a", "player-b"}, entries[0].Texts("players"))
					})
				})
			})
		})
	})
}
