package matcher_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher/matchertest"
)

func startWithBlockedPublication(t *testing.T) (*scenario, *matchertest.Gate) {
	t.Helper()
	gate := matchertest.NewGate()
	s := newDefaultScenario(t)
	s.queue.AddPops(pairOfPlayerAAndPlayerB())
	s.publisher.AddSteps(matchertest.PublishBlockedUntil(gate))
	s.start(t)
	t.Cleanup(gate.Open)
	s.tick(t)
	s.waitForPublications(t, 1)
	s.shutdown()
	s.waitForTimers(t, 1)
	return s, gate
}

func startWithPublicationInterruptedByShutdown(t *testing.T) *scenario {
	t.Helper()
	s := newDefaultScenario(t)
	s.queue.AddPops(pairOfPlayerAAndPlayerB())
	s.publisher.AddSteps(matchertest.PublishInterruptedByShutdown())
	s.queue.AddReenqueues(matchertest.ReenqueueFails(errReenqueueFailed), matchertest.ReenqueueSucceeds())
	s.start(t)
	s.tick(t)
	s.waitForPublications(t, 1)
	s.shutdown()
	s.waitUntilDone(t)
	return s
}

func TestMatcherShutdown(t *testing.T) {
	t.Run("[マッチ成立]定期処理の繰り返しの停止", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("実行中の定期処理が無いとき", func(t *testing.T) {
				t.Run("シャットダウンが始まると、定期処理の繰り返しが終了する", func(t *testing.T) {
					s := newDefaultScenario(t)
					s.start(t)

					s.shutdown()

					s.waitUntilDone(t)
				})

				t.Run("定期処理の繰り返しが終了したあとは、テスト用のキューに記録された取り出しの要求は増えない", func(t *testing.T) {
					s := newDefaultScenario(t)
					s.start(t)
					s.shutdown()
					s.waitUntilDone(t)
					before := s.queue.PopCalls()

					for range 3 {
						s.clock.Advance(s.interval)
					}
					s.settle(t)

					assert.Equal(t, before, s.queue.PopCalls())
				})
			})

			t.Run("実行中の定期処理が、発行が終わるのを待って止まっていて、完了待ちの時間が経過する前に完了するとき", func(t *testing.T) {
				t.Run("シャットダウンが始まったあと、完了待ちの時間が経過する前は、実行中の定期処理が完了するまで、定期処理の繰り返しは終了しない", func(t *testing.T) {
					s, _ := startWithBlockedPublication(t)

					s.clock.Advance(defaultOptions().DrainTimeout - time.Millisecond)
					s.settle(t)

					assert.False(t, s.running.IsDone())
				})

				t.Run("実行中の定期処理が完了すると、定期処理の繰り返しが終了する", func(t *testing.T) {
					s, gate := startWithBlockedPublication(t)

					gate.Open()

					s.waitUntilDone(t)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("実行中の定期処理が、取り出しがシャットダウンに反応せず止まっていて、完了待ちの時間が経過しても完了しないとき", func(t *testing.T) {
				t.Run("シャットダウンが始まったあと、完了待ちの時間が経過すると、定期処理の繰り返しが終了する", func(t *testing.T) {
					gate := matchertest.NewGate()
					s := newDefaultScenario(t)
					s.queue.AddPops(matchertest.PopBlockedUntil(gate))
					s.start(t)
					t.Cleanup(gate.Open)
					s.tick(t)
					s.waitForPops(t, 1)
					s.shutdown()
					s.waitForTimers(t, 1)

					s.clock.Advance(defaultOptions().DrainTimeout)

					s.waitUntilDone(t)
				})
			})

			t.Run("ペアが取り出され、発行がシャットダウンで中断され、1回目の書き戻しがエラーを返し、2回目の書き戻しが成功するとき", func(t *testing.T) {
				t.Run("定期処理の繰り返しが終了する", func(t *testing.T) {
					s := newDefaultScenario(t)
					s.queue.AddPops(pairOfPlayerAAndPlayerB())
					s.publisher.AddSteps(matchertest.PublishInterruptedByShutdown())
					s.queue.AddReenqueues(matchertest.ReenqueueFails(errReenqueueFailed), matchertest.ReenqueueSucceeds())
					s.start(t)
					s.tick(t)
					s.waitForPublications(t, 1)

					s.shutdown()

					s.waitUntilDone(t)
				})

				t.Run("シャットダウン中にペアを失ったことを知らせるErrorログは出力されない", func(t *testing.T) {
					s := startWithPublicationInterruptedByShutdown(t)

					assert.Empty(t, s.logs.Entries(slog.LevelError, "matcher lost pair during shutdown"))
				})

				t.Run("1人目がplayer-a、2人目がplayer-bのペアがgateway_instance_idがgw-1で取り出されたとき", func(t *testing.T) {
					t.Run("2件目のテスト用のキューに記録された書き戻しの要求のキューで待つプレイヤーは、player-a、player-bの順になる", func(t *testing.T) {
						s := startWithPublicationInterruptedByShutdown(t)

						requests := s.queue.ReenqueueRequests()
						require.Len(t, requests, 2)
						assert.Equal(t, []string{"player-a", "player-b"}, playerIDsOfEntries(requests[1].Entries))
					})

					t.Run("2件目のテスト用のキューに記録された書き戻しの要求のgateway_instance_idはgw-1になる", func(t *testing.T) {
						s := startWithPublicationInterruptedByShutdown(t)

						requests := s.queue.ReenqueueRequests()
						require.Len(t, requests, 2)
						assert.Equal(t, "gw-1", requests[1].GatewayInstanceID)
					})
				})
			})
		})
	})
}
