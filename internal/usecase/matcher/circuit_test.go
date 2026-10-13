package matcher_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher/matchertest"
)

const (
	circuitInterval         = time.Second
	circuitThreshold        = 3
	failuresBelowThreshold  = circuitThreshold - 1
	intervalsWithinCooldown = 5
	cooldownIntervals       = intervalsWithinCooldown + 1
	circuitCooldown         = cooldownIntervals * circuitInterval
	cooldownShortfall       = time.Millisecond
	circuitDrainTimeout     = 10 * time.Second
	successfulRunCount      = 5
	singleRun               = 1
)

var errPopFailed = errors.New("redis unavailable while popping")

type tickPrep func(s *scenario)

type tickRun struct {
	prep  tickPrep
	times int
}

type circuitRow struct {
	name string
	runs []tickRun
}

type waitRow struct {
	name string
	wait func(t *testing.T, s *scenario)
}

type cooldownRow struct {
	name string
	base func(t *testing.T, s *scenario)
}

func prepPublishFails(s *scenario) {
	s.queue.AddPops(pairOfPlayerAAndPlayerB())
	s.publisher.AddSteps(matchertest.PublishFails(errPublishFailed))
}

func prepPublishFailsWithPairDropped(s *scenario) {
	prepPublishFails(s)
	s.queue.AddReenqueues(matchertest.ReenqueueSkipped())
}

func prepPublishSucceeds(s *scenario) {
	s.queue.AddPops(pairOfPlayerAAndPlayerB())
}

func prepNothingPopped(*scenario) {}

func prepPopFails(s *scenario) {
	s.queue.AddPops(matchertest.PopFails(errPopFailed))
}

func newCircuitScenario(t *testing.T) *scenario {
	t.Helper()
	s := newScenario(t, matcher.Options{
		Interval:         circuitInterval,
		CircuitThreshold: circuitThreshold,
		CircuitCooldown:  circuitCooldown,
		DrainTimeout:     circuitDrainTimeout,
	}, scenarioStartedAt)
	s.start(t)
	return s
}

func (s *scenario) tickWith(t *testing.T, prep tickPrep) {
	t.Helper()
	prep(s)
	s.tick(t)
}

func (s *scenario) runTicks(t *testing.T, runs ...tickRun) {
	t.Helper()
	for _, run := range runs {
		for range run.times {
			s.tickWith(t, run.prep)
		}
	}
}

func (s *scenario) waitShortOfCooldown(t *testing.T) {
	t.Helper()
	s.runTicks(t, tickRun{prepNothingPopped, intervalsWithinCooldown})
	s.advance(t, s.interval-cooldownShortfall)
}

func (s *scenario) waitOutCooldown(t *testing.T, prep tickPrep) {
	t.Helper()
	s.runTicks(t, tickRun{prepNothingPopped, intervalsWithinCooldown})
	s.tickWith(t, prep)
}

func newOpenedByPublishFailures(t *testing.T) *scenario {
	t.Helper()
	s := newCircuitScenario(t)
	s.runTicks(t, tickRun{prepPublishFails, circuitThreshold})
	require.Equal(t, "open", s.circuitState())
	return s
}

func newOpenedByPopFailures(t *testing.T) *scenario {
	t.Helper()
	s := newCircuitScenario(t)
	s.runTicks(t, tickRun{prepPopFails, circuitThreshold})
	require.Equal(t, "open", s.circuitState())
	return s
}

func baseJustOpened(*testing.T, *scenario) {}

func baseReopenedByPublishFailure(t *testing.T, s *scenario) {
	t.Helper()
	s.waitOutCooldown(t, prepPublishFails)
	require.Equal(t, "open", s.circuitState())
}

func baseReopenedByPopFailure(t *testing.T, s *scenario) {
	t.Helper()
	s.waitOutCooldown(t, prepPopFails)
	require.Equal(t, "open", s.circuitState())
}

func TestCircuitBreaker(t *testing.T) {
	t.Run("[サーキットブレーカー]開閉の判定", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			closedTests := []circuitRow{
				{"定期処理が1回も実行されていないとき、サーキットブレーカーの状態を取得すると、閉じている", nil},
				{"取り出しが毎回ペアを返し、発行が毎回成功するとき、定期処理の繰り返しを開始したあとにサーキットブレーカーの状態を取得すると、閉じたままになる", []tickRun{{prepPublishSucceeds, successfulRunCount}}},
			}
			for _, tt := range closedTests {
				t.Run(tt.name, func(t *testing.T) {
					s := newCircuitScenario(t)

					s.runTicks(t, tt.runs...)

					assert.Equal(t, "closed", s.circuitState())
				})
			}
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("開く連続失敗回数を設定し、定期処理の繰り返しを開始すると", func(t *testing.T) {
				t.Run("サーキットブレーカーの状態を取得すると", func(t *testing.T) {
					remainsClosedTests := []circuitRow{
						{"発行がエラーを返す定期処理が、開く連続失敗回数より1回少ない回数続いたとき、閉じたままになる", []tickRun{{prepPublishFails, failuresBelowThreshold}}},
						{"取り出しがエラーを返す定期処理が、開く連続失敗回数より1回少ない回数続いたとき、閉じたままになる", []tickRun{{prepPopFails, failuresBelowThreshold}}},
						{"発行がエラーを返す定期処理が開く連続失敗回数より1回少ない回数続いたあと、発行が成功する定期処理を挟んで、発行がエラーを返す定期処理が開く連続失敗回数より1回少ない回数続いたとき、閉じたままになる", []tickRun{{prepPublishFails, failuresBelowThreshold}, {prepPublishSucceeds, singleRun}, {prepPublishFails, failuresBelowThreshold}}},
						{"取り出しがエラーを返す定期処理が開く連続失敗回数より1回少ない回数続いたあと、発行がエラーを返す定期処理が開く連続失敗回数より1回少ない回数続いたとき、閉じたままになる", []tickRun{{prepPopFails, failuresBelowThreshold}, {prepPublishFails, failuresBelowThreshold}}},
						{"取り出しがエラーを返す定期処理が開く連続失敗回数より1回少ない回数続いたあと、取り出されたペアが無い定期処理を挟んで、取り出しがエラーを返す定期処理が開く連続失敗回数より1回少ない回数続いたとき、閉じたままになる", []tickRun{{prepPopFails, failuresBelowThreshold}, {prepNothingPopped, singleRun}, {prepPopFails, failuresBelowThreshold}}},
						{"取り出しがエラーを返す定期処理が開く連続失敗回数より1回少ない回数続いたあと、発行が成功する定期処理を挟んで、取り出しがエラーを返す定期処理が開く連続失敗回数より1回少ない回数続いたとき、閉じたままになる", []tickRun{{prepPopFails, failuresBelowThreshold}, {prepPublishSucceeds, singleRun}, {prepPopFails, failuresBelowThreshold}}},
					}
					for _, tt := range remainsClosedTests {
						t.Run(tt.name, func(t *testing.T) {
							s := newCircuitScenario(t)

							s.runTicks(t, tt.runs...)

							assert.Equal(t, "closed", s.circuitState())
						})
					}

					opensTests := []circuitRow{
						{"発行がエラーを返す定期処理が開く連続失敗回数より1回少ない回数続いたあと、取り出されたペアが無い定期処理を挟んで、発行がエラーを返す定期処理がもう1回続いたとき、開く", []tickRun{{prepPublishFails, failuresBelowThreshold}, {prepNothingPopped, singleRun}, {prepPublishFails, singleRun}}},
						{"発行がエラーを返す定期処理が開く連続失敗回数より1回少ない回数続いたあと、取り出しがエラーを返す定期処理を挟んで、発行がエラーを返す定期処理がもう1回続いたとき、開く", []tickRun{{prepPublishFails, failuresBelowThreshold}, {prepPopFails, singleRun}, {prepPublishFails, singleRun}}},
						{"発行がエラーを返す定期処理が、開く連続失敗回数と同じ回数続いたとき、開く", []tickRun{{prepPublishFails, circuitThreshold}}},
						{"発行がエラーを返し、書き戻しがgateway_instance_idが取り出したときから変わっていて書き戻さなかった定期処理が、開く連続失敗回数と同じ回数続いたとき、開く", []tickRun{{prepPublishFailsWithPairDropped, circuitThreshold}}},
						{"取り出しがエラーを返す定期処理が、開く連続失敗回数と同じ回数続いたとき、開く", []tickRun{{prepPopFails, circuitThreshold}}},
					}
					for _, tt := range opensTests {
						t.Run(tt.name, func(t *testing.T) {
							s := newCircuitScenario(t)

							s.runTicks(t, tt.runs...)

							assert.Equal(t, "open", s.circuitState())
						})
					}
				})

				t.Run("発行がエラーを返す定期処理が開く連続失敗回数と同じ回数続き、サーキットブレーカーが開いたとき", func(t *testing.T) {
					t.Run("サーキットブレーカーの状態を取得すると", func(t *testing.T) {
						remainsOpenTests := []waitRow{
							{"再試行までの時間に1ミリ秒足りない時間が経ったとき、開いたままになる", func(t *testing.T, s *scenario) { s.waitShortOfCooldown(t) }},
							{"再試行までの時間が経ったあとの最初の発行が失敗したとき、開いたままになる", func(t *testing.T, s *scenario) { s.waitOutCooldown(t, prepPublishFails) }},
							{"再試行までの時間が経ったあとの最初の取り出しが空を返したとき、開いたままになる", func(t *testing.T, s *scenario) { s.waitOutCooldown(t, prepNothingPopped) }},
							{"再試行までの時間が経ったあとの最初の取り出しがエラーを返したとき、開いたままになる", func(t *testing.T, s *scenario) { s.waitOutCooldown(t, prepPopFails) }},
						}
						for _, tt := range remainsOpenTests {
							t.Run(tt.name, func(t *testing.T) {
								s := newOpenedByPublishFailures(t)

								tt.wait(t, s)

								assert.Equal(t, "open", s.circuitState())
							})
						}
					})

					t.Run("再試行までの時間に1ミリ秒足りない時間が経ち、その間に定期処理の実行間隔が5回経過しても", func(t *testing.T) {
						rows := []cooldownRow{
							{"最初に開いたとき、テスト用のキューに記録された取り出しの要求は増えない", baseJustOpened},
							{"再試行までの時間が経ったあとの最初の発行が失敗して、サーキットブレーカーが再び開いたとき、テスト用のキューに記録された取り出しの要求は増えない", baseReopenedByPublishFailure},
						}
						for _, tt := range rows {
							t.Run(tt.name, func(t *testing.T) {
								s := newOpenedByPublishFailures(t)
								tt.base(t, s)
								before := s.queue.PopCalls()

								s.waitShortOfCooldown(t)

								assert.Equal(t, before, s.queue.PopCalls())
							})
						}
					})

					t.Run("ちょうど再試行までの時間が経つと", func(t *testing.T) {
						rows := []cooldownRow{
							{"最初に開いたとき、テスト用のキューに記録された取り出しの要求が増える", baseJustOpened},
							{"再試行までの時間が経ったあとの最初の発行が失敗して、サーキットブレーカーが再び開いたとき、テスト用のキューに記録された取り出しの要求が増える", baseReopenedByPublishFailure},
						}
						for _, tt := range rows {
							t.Run(tt.name, func(t *testing.T) {
								s := newOpenedByPublishFailures(t)
								tt.base(t, s)
								before := s.queue.PopCalls()

								s.waitOutCooldown(t, prepNothingPopped)

								assert.Greater(t, s.queue.PopCalls(), before)
							})
						}
					})

					t.Run("再試行までの時間が経ったあとの最初の発行が成功するとき", func(t *testing.T) {
						newClosedByPublishSuccess := func(t *testing.T) *scenario {
							t.Helper()
							s := newOpenedByPublishFailures(t)
							s.waitOutCooldown(t, prepPublishSucceeds)
							return s
						}

						closedTests := []circuitRow{
							{"サーキットブレーカーの状態を取得すると、閉じている", nil},
							{"閉じたあとに、発行がエラーを返す定期処理が開く連続失敗回数より1回少ない回数続いたとき、サーキットブレーカーの状態を取得すると、閉じたままになる", []tickRun{{prepPublishFails, failuresBelowThreshold}}},
						}
						for _, tt := range closedTests {
							t.Run(tt.name, func(t *testing.T) {
								s := newClosedByPublishSuccess(t)

								s.runTicks(t, tt.runs...)

								assert.Equal(t, "closed", s.circuitState())
							})
						}

						t.Run("再試行までの時間が経たないうちに、定期処理の実行間隔が5回経過すると、テスト用のキューに記録された取り出しの要求が5件増える", func(t *testing.T) {
							s := newClosedByPublishSuccess(t)
							before := s.queue.PopCalls()

							s.runTicks(t, tickRun{prepNothingPopped, intervalsWithinCooldown})

							assert.Equal(t, before+intervalsWithinCooldown, s.queue.PopCalls())
						})
					})
				})

				t.Run("取り出しがエラーを返す定期処理が開く連続失敗回数と同じ回数続き、サーキットブレーカーが開いたとき", func(t *testing.T) {
					t.Run("サーキットブレーカーの状態を取得すると", func(t *testing.T) {
						remainsOpenTests := []waitRow{
							{"再試行までの時間に1ミリ秒足りない時間が経ったとき、開いたままになる", func(t *testing.T, s *scenario) { s.waitShortOfCooldown(t) }},
							{"再試行までの時間が経ったあとの最初の取り出しがエラーを返したとき、開いたままになる", func(t *testing.T, s *scenario) { s.waitOutCooldown(t, prepPopFails) }},
						}
						for _, tt := range remainsOpenTests {
							t.Run(tt.name, func(t *testing.T) {
								s := newOpenedByPopFailures(t)

								tt.wait(t, s)

								assert.Equal(t, "open", s.circuitState())
							})
						}

						closesTests := []waitRow{
							{"再試行までの時間が経ったあとの最初の取り出しが空を返したとき、閉じている", func(t *testing.T, s *scenario) { s.waitOutCooldown(t, prepNothingPopped) }},
							{"再試行までの時間が経ったあとの最初の取り出しがペアを返し、発行が成功したとき、閉じている", func(t *testing.T, s *scenario) { s.waitOutCooldown(t, prepPublishSucceeds) }},
						}
						for _, tt := range closesTests {
							t.Run(tt.name, func(t *testing.T) {
								s := newOpenedByPopFailures(t)

								tt.wait(t, s)

								assert.Equal(t, "closed", s.circuitState())
							})
						}
					})

					t.Run("再試行までの時間に1ミリ秒足りない時間が経ち、その間に定期処理の実行間隔が5回経過しても", func(t *testing.T) {
						rows := []cooldownRow{
							{"最初に開いたとき、テスト用のキューに記録された取り出しの要求は増えない", baseJustOpened},
							{"再試行までの時間が経ったあとの最初の取り出しがエラーを返して、開いたまま再試行までの時間を数え直したとき、テスト用のキューに記録された取り出しの要求は増えない", baseReopenedByPopFailure},
						}
						for _, tt := range rows {
							t.Run(tt.name, func(t *testing.T) {
								s := newOpenedByPopFailures(t)
								tt.base(t, s)
								before := s.queue.PopCalls()

								s.waitShortOfCooldown(t)

								assert.Equal(t, before, s.queue.PopCalls())
							})
						}
					})

					t.Run("ちょうど再試行までの時間が経つと", func(t *testing.T) {
						rows := []cooldownRow{
							{"最初に開いたとき、テスト用のキューに記録された取り出しの要求が増える", baseJustOpened},
							{"再試行までの時間が経ったあとの最初の取り出しがエラーを返して、開いたまま再試行までの時間を数え直したとき、テスト用のキューに記録された取り出しの要求が増える", baseReopenedByPopFailure},
						}
						for _, tt := range rows {
							t.Run(tt.name, func(t *testing.T) {
								s := newOpenedByPopFailures(t)
								tt.base(t, s)
								before := s.queue.PopCalls()

								s.waitOutCooldown(t, prepNothingPopped)

								assert.Greater(t, s.queue.PopCalls(), before)
							})
						}
					})
				})
			})
		})
	})
}
