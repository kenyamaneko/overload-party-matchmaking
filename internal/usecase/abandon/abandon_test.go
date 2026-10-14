package abandon_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/abandon"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/abandon/abandontest"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/usecasetest"
)

type abandonment struct {
	queue     *abandontest.Queue
	logs      *usecasetest.Logs
	abandoner *abandon.Abandoner
}

func newAbandonment(t *testing.T) *abandonment {
	t.Helper()
	logs := usecasetest.CaptureLogs(t)
	queue := abandontest.NewQueue()
	return &abandonment{queue: queue, logs: logs, abandoner: abandon.New(queue)}
}

func (a *abandonment) abandonPlayersAAndB() error {
	return a.abandoner.Abandon(context.Background(), "mch-test-1", []string{"player-a", "player-b"}, "player_not_connected")
}

var (
	errCancelFailedForA = errors.New("redis timed out while cancelling the first player")
	errCancelFailedForB = errors.New("redis refused while cancelling the second player")
)

func TestAbandonPair(t *testing.T) {
	t.Run("[マッチ不成立]ペアの破棄", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("プレイヤー識別子player-aとplayer-bの申告で、待機にいるプレイヤーと待機にいないプレイヤーの組み合わせごとに破棄するとき", func(t *testing.T) {
				combinations := []struct {
					name  string
					stepA abandontest.CancelStep
					stepB abandontest.CancelStep
				}{
					{"2人とも待機にいるとき", abandontest.PlayerWaiting(), abandontest.PlayerWaiting()},
					{"player-aだけが待機にいるとき", abandontest.PlayerWaiting(), abandontest.PlayerNotWaiting()},
					{"player-bだけが待機にいるとき", abandontest.PlayerNotWaiting(), abandontest.PlayerWaiting()},
					{"2人とも待機にいないとき", abandontest.PlayerNotWaiting(), abandontest.PlayerNotWaiting()},
				}
				for _, tt := range combinations {
					t.Run(tt.name+"、テスト用のキューに記録された取り消しの要求のプレイヤー識別子は、player-a、player-bの順になる", func(t *testing.T) {
						a := newAbandonment(t)
						a.queue.AddCancels("player-a", tt.stepA)
						a.queue.AddCancels("player-b", tt.stepB)

						_ = a.abandonPlayersAAndB()

						assert.Equal(t, []string{"player-a", "player-b"}, a.queue.CancelRequests())
					})
				}

				for _, tt := range combinations {
					t.Run(tt.name+"、破棄はエラーを返さない", func(t *testing.T) {
						a := newAbandonment(t)
						a.queue.AddCancels("player-a", tt.stepA)
						a.queue.AddCancels("player-b", tt.stepB)

						err := a.abandonPlayersAAndB()

						assert.NoError(t, err)
					})
				}
			})

			t.Run("マッチ識別子mch-test-1・理由player_not_connected・プレイヤー2人の申告を破棄するとき", func(t *testing.T) {
				t.Run("破棄を記録するInfoログが1件出力される", func(t *testing.T) {
					a := newAbandonment(t)

					_ = a.abandonPlayersAAndB()

					assert.Len(t, a.logs.Entries(slog.LevelInfo, "match abandoned"), 1)
				})

				loggedTests := []struct {
					name string
					key  string
					want string
				}{
					{"破棄を記録するログのマッチ識別子はmch-test-1になる", "match_id", "mch-test-1"},
					{"破棄を記録するログの理由はplayer_not_connectedになる", "reason", "player_not_connected"},
					{"破棄を記録するログの人数は2になる", "player_count", "2"},
				}
				for _, tt := range loggedTests {
					t.Run(tt.name, func(t *testing.T) {
						a := newAbandonment(t)

						_ = a.abandonPlayersAAndB()

						entries := a.logs.Entries(slog.LevelInfo, "match abandoned")
						require.Len(t, entries, 1)
						assert.Equal(t, tt.want, entries[0].Text(tt.key))
					})
				}
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("プレイヤー識別子player-aとplayer-bの申告で、取り消しがエラーを返したとき", func(t *testing.T) {
				onlyAFails := func(a *abandonment) { a.queue.AddCancels("player-a", abandontest.CancelFails(errCancelFailedForA)) }
				onlyBFails := func(a *abandonment) { a.queue.AddCancels("player-b", abandontest.CancelFails(errCancelFailedForB)) }
				bothFail := func(a *abandonment) {
					onlyAFails(a)
					onlyBFails(a)
				}

				t.Run("マッチ識別子がmch-test-1のとき", func(t *testing.T) {
					tests := []struct {
						name  string
						setup func(a *abandonment)
					}{
						{"player-aの取り消しが失敗したとき", onlyAFails},
						{"player-bの取り消しが失敗したとき", onlyBFails},
						{"player-aとplayer-bの両方の取り消しが失敗したとき", bothFail},
					}
					for _, tt := range tests {
						t.Run(tt.name+"、エラーのメッセージにマッチ識別子mch-test-1が含まれる", func(t *testing.T) {
							a := newAbandonment(t)
							tt.setup(a)

							err := a.abandonPlayersAAndB()

							assert.ErrorContains(t, err, "mch-test-1")
						})
					}
				})

				singleFailureTests := []struct {
					name         string
					setup        func(a *abandonment)
					wantPlayerID string
					wantCause    string
				}{
					{"player-aの取り消しが失敗したとき", onlyAFails, "player-a", "redis timed out while cancelling the first player"},
					{"player-bの取り消しが失敗したとき", onlyBFails, "player-b", "redis refused while cancelling the second player"},
				}
				for _, tt := range singleFailureTests {
					t.Run(tt.name+"、エラーのメッセージに、失敗した取り消しのプレイヤー識別子が含まれる", func(t *testing.T) {
						a := newAbandonment(t)
						tt.setup(a)

						err := a.abandonPlayersAAndB()

						assert.ErrorContains(t, err, tt.wantPlayerID)
					})
				}

				for _, tt := range singleFailureTests {
					t.Run(tt.name+"、エラーのメッセージに、取り消しが返したエラーのメッセージが含まれる", func(t *testing.T) {
						a := newAbandonment(t)
						tt.setup(a)

						err := a.abandonPlayersAAndB()

						assert.ErrorContains(t, err, tt.wantCause)
					})
				}

				failureTests := []struct {
					name  string
					setup func(a *abandonment)
				}{
					{"player-aの取り消しが失敗したとき", onlyAFails},
					{"player-bの取り消しが失敗したとき", onlyBFails},
					{"player-aとplayer-bの両方の取り消しが失敗したとき", bothFail},
				}
				for _, tt := range failureTests {
					t.Run(tt.name+"、破棄を記録するInfoログは出力されない", func(t *testing.T) {
						a := newAbandonment(t)
						tt.setup(a)

						_ = a.abandonPlayersAAndB()

						assert.Empty(t, a.logs.Entries(slog.LevelInfo, "match abandoned"))
					})
				}

				for _, tt := range failureTests {
					t.Run(tt.name+"、テスト用のキューに記録された取り消しの要求のプレイヤー識別子は、player-a、player-bの順になる", func(t *testing.T) {
						a := newAbandonment(t)
						tt.setup(a)

						_ = a.abandonPlayersAAndB()

						assert.Equal(t, []string{"player-a", "player-b"}, a.queue.CancelRequests())
					})
				}

				t.Run("player-aとplayer-bの両方の取り消しが失敗したとき", func(t *testing.T) {
					containedTests := []struct {
						name string
						want string
					}{
						{"エラーのメッセージにplayer-aのプレイヤー識別子が含まれる", "player-a"},
						{"エラーのメッセージにplayer-bのプレイヤー識別子が含まれる", "player-b"},
						{"エラーのメッセージにplayer-aの取り消しが返したエラーのメッセージが含まれる", "redis timed out while cancelling the first player"},
						{"エラーのメッセージにplayer-bの取り消しが返したエラーのメッセージが含まれる", "redis refused while cancelling the second player"},
					}
					for _, tt := range containedTests {
						t.Run(tt.name, func(t *testing.T) {
							a := newAbandonment(t)
							bothFail(a)

							err := a.abandonPlayersAAndB()

							assert.ErrorContains(t, err, tt.want)
						})
					}
				})
			})

			t.Run("プレイヤー識別子player-aとplayer-bの申告で、player-aの取り消しが1回失敗したあとに、同じ申告をやり直したとき", func(t *testing.T) {
				type retry struct {
					abandonment         *abandonment
					requestsBeforeRetry int
					err                 error
				}
				retryAfterFailure := func(t *testing.T) retry {
					t.Helper()
					a := newAbandonment(t)
					a.queue.AddCancels("player-a", abandontest.CancelFails(errCancelFailedForA), abandontest.PlayerWaiting())
					require.Error(t, a.abandonPlayersAAndB())
					requestsBeforeRetry := len(a.queue.CancelRequests())
					return retry{abandonment: a, requestsBeforeRetry: requestsBeforeRetry, err: a.abandonPlayersAAndB()}
				}

				t.Run("破棄はエラーを返さない", func(t *testing.T) {
					r := retryAfterFailure(t)

					assert.NoError(t, r.err)
				})

				t.Run("やり直しでテスト用のキューに記録された取り消しの要求のプレイヤー識別子は、player-a、player-bの順になる", func(t *testing.T) {
					r := retryAfterFailure(t)

					assert.Equal(t, []string{"player-a", "player-b"}, r.abandonment.queue.CancelRequests()[r.requestsBeforeRetry:])
				})

				t.Run("破棄を記録するInfoログが、やり直しで1件だけ出力される", func(t *testing.T) {
					r := retryAfterFailure(t)

					assert.Len(t, r.abandonment.logs.Entries(slog.LevelInfo, "match abandoned"), 1)
				})
			})
		})
	})
}
