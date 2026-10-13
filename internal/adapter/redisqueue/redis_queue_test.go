//go:build integration

package redisqueue_test

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/redisqueue"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/redisqueue/redisqueuetest"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
)

const (
	gatewayOne = "TST-GW1"
	gatewayTwo = "TST-GW2"

	playerA = "TST-PA"
	playerB = "TST-PB"
	playerC = "TST-PC"
	playerD = "TST-PD"
	playerX = "TST-PX"
	playerY = "TST-PY"

	playerOne = "TST-P1"
	playerTen = "TST-P10"

	defaultDeckID int64 = 1
	defaultName         = "TST-NAME"
	defaultLevel  int64 = 1

	deckIDOfA int64 = 11
	levelOfA  int64 = 5
	nameOfA         = "TST-NAME-A"
	deckIDOfB int64 = 22
	levelOfB  int64 = 7
	nameOfB         = "TST-NAME-B"

	enqueueInterval = 5 * time.Millisecond
	concurrentCount = 20
	popPlayerCount  = 10
)

type entryField func(entry domain.QueueEntry) any

func extractDeckID(entry domain.QueueEntry) any         { return entry.DeckID }
func extractLevel(entry domain.QueueEntry) any          { return entry.Level }
func extractName(entry domain.QueueEntry) any           { return entry.Name }
func extractJoinedAtMillis(entry domain.QueueEntry) any { return entry.JoinedAt.UnixMilli() }

func newQueue(t *testing.T, client *redis.Client) *redisqueue.RedisQueue {
	t.Helper()
	redisqueuetest.Flush(t, client)
	return redisqueue.NewRedisQueue(client)
}

func newUnreachableQueue(t *testing.T) *redisqueue.RedisQueue {
	t.Helper()
	return redisqueue.NewRedisQueue(redisqueuetest.NewUnreachableStore(t).Client)
}

func enqueue(t *testing.T, q *redisqueue.RedisQueue, playerID string, deckID int64, name string, level int64, gatewayInstanceID string) int64 {
	t.Helper()
	removed, err := q.Enqueue(t.Context(), playerID, deckID, name, level, gatewayInstanceID)
	require.NoError(t, err)
	return removed
}

func enqueueDefault(t *testing.T, q *redisqueue.RedisQueue, playerID string, gatewayInstanceID string) int64 {
	t.Helper()
	return enqueue(t, q, playerID, defaultDeckID, defaultName, defaultLevel, gatewayInstanceID)
}

func enqueueSpaced(t *testing.T, q *redisqueue.RedisQueue, gatewayInstanceID string, playerIDs ...string) {
	t.Helper()
	for _, playerID := range playerIDs {
		enqueueDefault(t, q, playerID, gatewayInstanceID)
		time.Sleep(enqueueInterval)
	}
}

func enqueueDistinctPair(t *testing.T, q *redisqueue.RedisQueue, gatewayInstanceID string) map[string][2]int64 {
	t.Helper()
	windows := map[string][2]int64{}

	before := time.Now().UnixMilli()
	enqueue(t, q, playerA, deckIDOfA, nameOfA, levelOfA, gatewayInstanceID)
	windows[playerA] = [2]int64{before, time.Now().UnixMilli()}

	before = time.Now().UnixMilli()
	enqueue(t, q, playerB, deckIDOfB, nameOfB, levelOfB, gatewayInstanceID)
	windows[playerB] = [2]int64{before, time.Now().UnixMilli()}
	return windows
}

func popPair(t *testing.T, q *redisqueue.RedisQueue) ([]domain.QueueEntry, string) {
	t.Helper()
	entries, gatewayInstanceID, err := q.PopPair(t.Context())
	require.NoError(t, err)
	return entries, gatewayInstanceID
}

func readQueueSize(t *testing.T, q *redisqueue.RedisQueue) int64 {
	t.Helper()
	size, err := q.Size(t.Context())
	require.NoError(t, err)
	return size
}

func cancelPlayer(t *testing.T, q *redisqueue.RedisQueue, playerID string) bool {
	t.Helper()
	wasQueued, err := q.Cancel(t.Context(), playerID)
	require.NoError(t, err)
	return wasQueued
}

func reenqueue(t *testing.T, q *redisqueue.RedisQueue, entries []domain.QueueEntry, gatewayInstanceID string) bool {
	t.Helper()
	isWrittenBack, err := q.Reenqueue(t.Context(), entries, gatewayInstanceID)
	require.NoError(t, err)
	return isWrittenBack
}

func newEntry(playerID string) domain.QueueEntry {
	return domain.QueueEntry{
		PlayerID: playerID,
		DeckID:   defaultDeckID,
		Name:     defaultName,
		Level:    defaultLevel,
		JoinedAt: time.UnixMilli(1_700_000_000_000),
	}
}

func newEntries(playerIDs ...string) []domain.QueueEntry {
	entries := make([]domain.QueueEntry, 0, len(playerIDs))
	for _, playerID := range playerIDs {
		entries = append(entries, newEntry(playerID))
	}
	return entries
}

func extractPlayerIDs(entries []domain.QueueEntry) []string {
	playerIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		playerIDs = append(playerIDs, entry.PlayerID)
	}
	return playerIDs
}

func findEntry(t *testing.T, entries []domain.QueueEntry, playerID string) domain.QueueEntry {
	t.Helper()
	require.Contains(t, extractPlayerIDs(entries), playerID)
	entriesByPlayerID := map[string]domain.QueueEntry{}
	for _, entry := range entries {
		entriesByPlayerID[entry.PlayerID] = entry
	}
	return entriesByPlayerID[playerID]
}

func runConcurrently[T any](count int, operation func(index int) (T, error)) ([]T, []error) {
	results := make([]T, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = operation(i)
		}()
	}
	wg.Wait()
	return results, errs
}

func requireNoErrors(t *testing.T, errs []error) {
	t.Helper()
	for _, err := range errs {
		require.NoError(t, err)
	}
}

func assertStoreUnreachableError(t *testing.T, err error) {
	t.Helper()
	var opErr *net.OpError
	assert.ErrorAs(t, err, &opErr)
}

func TestEnqueue(t *testing.T) {
	client := redisqueuetest.Start(t)

	t.Run("[マッチメイキングキュー]プレイヤーの追加", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("一度も追加されていない空のキューのとき", func(t *testing.T) {
				t.Run("プレイヤーAを追加すると、追加の結果が示す、追加によってキューから取り除かれたプレイヤーの人数は0になる", func(t *testing.T) {
					q := newQueue(t, client)

					removed := enqueueDefault(t, q, playerA, gatewayOne)

					assert.Equal(t, int64(0), removed)
				})

				t.Run("プレイヤーAを追加すると、キューサイズは1になる", func(t *testing.T) {
					q := newQueue(t, client)

					enqueueDefault(t, q, playerA, gatewayOne)

					assert.Equal(t, int64(1), readQueueSize(t, q))
				})
			})

			t.Run("gatewayプロセスの識別子TST-GW1でプレイヤーAが待機しているとき、同じgatewayプロセスの識別子TST-GW1でプレイヤーAを追加し直す", func(t *testing.T) {
				sizeCases := []struct {
					name   string
					deckID int64
				}{
					{name: "デッキIDが同じとき、キューサイズは1のままになる", deckID: defaultDeckID},
					{name: "デッキIDが異なるとき、キューサイズは1のままになる", deckID: 2},
				}
				for _, tc := range sizeCases {
					t.Run(tc.name, func(t *testing.T) {
						q := newQueue(t, client)
						enqueueDefault(t, q, playerA, gatewayOne)

						enqueue(t, q, playerA, tc.deckID, defaultName, defaultLevel, gatewayOne)

						assert.Equal(t, int64(1), readQueueSize(t, q))
					})
				}

				t.Run("追加の結果が示す、追加によってキューから取り除かれたプレイヤーの人数は0になる", func(t *testing.T) {
					q := newQueue(t, client)
					enqueueDefault(t, q, playerA, gatewayOne)

					removed := enqueueDefault(t, q, playerA, gatewayOne)

					assert.Equal(t, int64(0), removed)
				})

				t.Run("デッキID・名前・レベルをすべて変えて追加し直し、同じgatewayプロセスの識別子TST-GW1でプレイヤーBを追加してから続けてペアを取り出す", func(t *testing.T) {
					cases := []struct {
						name  string
						field entryField
						want  any
					}{
						{name: "取り出されたプレイヤーAのデッキIDは、追加し直したときの値になる", field: extractDeckID, want: int64(33)},
						{name: "取り出されたプレイヤーAの名前は、追加し直したときの値になる", field: extractName, want: "TST-NAME-RE"},
						{name: "取り出されたプレイヤーAのレベルは、追加し直したときの値になる", field: extractLevel, want: int64(9)},
					}
					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							q := newQueue(t, client)
							enqueueDefault(t, q, playerA, gatewayOne)
							enqueue(t, q, playerA, 33, "TST-NAME-RE", 9, gatewayOne)
							enqueueDefault(t, q, playerB, gatewayOne)

							entries, _ := popPair(t, q)

							assert.Equal(t, tc.want, tc.field(findEntry(t, entries, playerA)))
						})
					}
				})
			})

			t.Run("プレイヤーA、B、Aの順に、2ミリ秒以上の間隔をあけて、すべてgatewayプロセスの識別子TST-GW1で追加したとき、続けてペアを取り出すと、取り出されるプレイヤーの順は、プレイヤーB、プレイヤーAになる", func(t *testing.T) {
				q := newQueue(t, client)
				enqueueSpaced(t, q, gatewayOne, playerA, playerB, playerA)

				entries, _ := popPair(t, q)

				assert.Equal(t, []string{playerB, playerA}, extractPlayerIDs(entries))
			})

			t.Run("待機しているプレイヤーが追加されたのと同じgatewayプロセスの識別子TST-GW1で、プレイヤーを追加する", func(t *testing.T) {
				cases := []struct {
					name     string
					waiting  string
					enqueued string
				}{
					{name: "待機しているプレイヤーがTST-P10で、追加するプレイヤーがTST-P1のとき、キューサイズは2になる", waiting: playerTen, enqueued: playerOne},
					{name: "待機しているプレイヤーがTST-P1で、追加するプレイヤーがTST-P10のとき、キューサイズは2になる", waiting: playerOne, enqueued: playerTen},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						q := newQueue(t, client)
						enqueueDefault(t, q, tc.waiting, gatewayOne)

						enqueueDefault(t, q, tc.enqueued, gatewayOne)

						assert.Equal(t, int64(2), readQueueSize(t, q))
					})
				}
			})

			t.Run("同時に追加するとき", func(t *testing.T) {
				t.Run("同じプレイヤーをgatewayプロセスの識別子TST-GW1で20回同時に追加すると、キューサイズは1になる", func(t *testing.T) {
					q := newQueue(t, client)

					_, errs := runConcurrently(concurrentCount, func(int) (int64, error) {
						return q.Enqueue(t.Context(), playerA, defaultDeckID, defaultName, defaultLevel, gatewayOne)
					})

					requireNoErrors(t, errs)
					assert.Equal(t, int64(1), readQueueSize(t, q))
				})

				t.Run("異なる20人のプレイヤーをgatewayプロセスの識別子TST-GW1で同時に追加すると、キューサイズは20になる", func(t *testing.T) {
					q := newQueue(t, client)

					_, errs := runConcurrently(concurrentCount, func(index int) (int64, error) {
						return q.Enqueue(t.Context(), fmt.Sprintf("TST-PE%d", index), defaultDeckID, defaultName, defaultLevel, gatewayOne)
					})

					requireNoErrors(t, errs)
					assert.Equal(t, int64(20), readQueueSize(t, q))
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("プレイヤーIDが空文字のプレイヤーを追加すると、追加は、プレイヤーIDが空であることを示すエラーで失敗する", func(t *testing.T) {
				q := newQueue(t, client)

				_, err := q.Enqueue(t.Context(), "", defaultDeckID, defaultName, defaultLevel, gatewayOne)

				assert.ErrorContains(t, err, "playerID is empty")
			})

			t.Run("プレイヤーIDがコロンを含むTST:P1のプレイヤーを追加すると、追加は、プレイヤーIDに使えない文字を含むことを示すエラーで失敗する", func(t *testing.T) {
				q := newQueue(t, client)

				_, err := q.Enqueue(t.Context(), "TST:P1", defaultDeckID, defaultName, defaultLevel, gatewayOne)

				assert.ErrorContains(t, err, "playerID contains ':'")
			})

			t.Run("プレイヤーYを、gatewayプロセスの識別子が空文字で追加すると、追加は、gatewayプロセスの識別子が空であることを示すエラーで失敗する", func(t *testing.T) {
				q := newQueue(t, client)

				_, err := q.Enqueue(t.Context(), playerY, defaultDeckID, defaultName, defaultLevel, "")

				assert.ErrorContains(t, err, "gatewayInstanceID is empty")
			})

			t.Run("プレイヤーXがgatewayプロセスの識別子TST-GW1で待機しているとき、プレイヤーIDが空文字のプレイヤーをgatewayプロセスの識別子TST-GW2で追加して失敗したあと、gatewayプロセスの識別子TST-GW1でプレイヤーYを追加すると、キューサイズは2になる", func(t *testing.T) {
				q := newQueue(t, client)
				enqueueDefault(t, q, playerX, gatewayOne)
				_, err := q.Enqueue(t.Context(), "", defaultDeckID, defaultName, defaultLevel, gatewayTwo)
				require.Error(t, err)

				enqueueDefault(t, q, playerY, gatewayOne)

				assert.Equal(t, int64(2), readQueueSize(t, q))
			})

			t.Run("ストアに接続できないとき", func(t *testing.T) {
				t.Run("追加は、ストアに接続できないことを示すエラーで失敗する", func(t *testing.T) {
					q := newUnreachableQueue(t)

					_, err := q.Enqueue(t.Context(), playerA, defaultDeckID, defaultName, defaultLevel, gatewayOne)

					assertStoreUnreachableError(t, err)
				})

				t.Run("追加が失敗したあと、ストアが再び起動してからプレイヤーAを追加すると、キューサイズは1になる", func(t *testing.T) {
					unreachable := redisqueuetest.NewUnreachableStore(t)
					q := redisqueue.NewRedisQueue(unreachable.Client)
					_, err := q.Enqueue(t.Context(), playerA, defaultDeckID, defaultName, defaultLevel, gatewayOne)
					require.Error(t, err)
					unreachable.StartStore()

					enqueueDefault(t, q, playerA, gatewayOne)

					assert.Equal(t, int64(1), readQueueSize(t, q))
				})
			})

			t.Run("追加の実行を中止するよう指示済みのとき", func(t *testing.T) {
				t.Run("追加は、追加の中止の指示を示すエラーで失敗する", func(t *testing.T) {
					q := newQueue(t, client)
					ctx, cancel := context.WithCancel(t.Context())
					cancel()

					_, err := q.Enqueue(ctx, playerA, defaultDeckID, defaultName, defaultLevel, gatewayOne)

					assert.ErrorIs(t, err, context.Canceled)
				})

				t.Run("追加が失敗したあと、キューサイズは0のままになる", func(t *testing.T) {
					q := newQueue(t, client)
					ctx, cancel := context.WithCancel(t.Context())
					cancel()
					_, err := q.Enqueue(ctx, playerA, defaultDeckID, defaultName, defaultLevel, gatewayOne)
					require.Error(t, err)

					assert.Equal(t, int64(0), readQueueSize(t, q))
				})
			})
		})
	})
}

func TestQueueReset(t *testing.T) {
	client := redisqueuetest.Start(t)

	newWaitingPair := func(t *testing.T) *redisqueue.RedisQueue {
		t.Helper()
		q := newQueue(t, client)
		enqueueDefault(t, q, playerA, gatewayOne)
		enqueueDefault(t, q, playerB, gatewayOne)
		return q
	}

	t.Run("[マッチメイキングキュー]キューのリセット", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("プレイヤーAとBがgatewayプロセスの識別子TST-GW1で待機しているとき", func(t *testing.T) {
				t.Run("gatewayプロセスの識別子TST-GW1でプレイヤーCを追加する", func(t *testing.T) {
					t.Run("追加の結果が示す、追加によってキューから取り除かれたプレイヤーの人数は0になる", func(t *testing.T) {
						q := newWaitingPair(t)

						removed := enqueueDefault(t, q, playerC, gatewayOne)

						assert.Equal(t, int64(0), removed)
					})

					t.Run("キューサイズは3になる", func(t *testing.T) {
						q := newWaitingPair(t)

						enqueueDefault(t, q, playerC, gatewayOne)

						assert.Equal(t, int64(3), readQueueSize(t, q))
					})
				})

				t.Run("gatewayプロセスの識別子TST-GW2でプレイヤーCを追加する", func(t *testing.T) {
					t.Run("追加の結果が示す、追加によってキューから取り除かれたプレイヤーの人数は2になる", func(t *testing.T) {
						q := newWaitingPair(t)

						removed := enqueueDefault(t, q, playerC, gatewayTwo)

						assert.Equal(t, int64(2), removed)
					})

					t.Run("キューサイズは1になる", func(t *testing.T) {
						q := newWaitingPair(t)

						enqueueDefault(t, q, playerC, gatewayTwo)

						assert.Equal(t, int64(1), readQueueSize(t, q))
					})

					t.Run("プレイヤーCを除外すると、除外の結果は、プレイヤーがキューにいたことを示す", func(t *testing.T) {
						q := newWaitingPair(t)
						enqueueDefault(t, q, playerC, gatewayTwo)

						wasQueued := cancelPlayer(t, q, playerC)

						assert.True(t, wasQueued)
					})
				})

				t.Run("追加するgatewayプロセスの識別子を、前回の追加で渡されたgatewayプロセスの識別子TST-GW1と少しだけ違う値にして、プレイヤーCを追加する", func(t *testing.T) {
					cases := []struct {
						name              string
						gatewayInstanceID string
					}{
						{name: "末尾に1文字足したTST-GW10のとき、追加の結果が示す、追加によってキューから取り除かれたプレイヤーの人数は2になる", gatewayInstanceID: "TST-GW10"},
						{name: "末尾の1文字を欠いたTST-GWのとき、追加の結果が示す、追加によってキューから取り除かれたプレイヤーの人数は2になる", gatewayInstanceID: "TST-GW"},
						{name: "大文字小文字だけが違うtst-gw1のとき、追加の結果が示す、追加によってキューから取り除かれたプレイヤーの人数は2になる", gatewayInstanceID: "tst-gw1"},
					}
					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							q := newWaitingPair(t)

							removed := enqueueDefault(t, q, playerC, tc.gatewayInstanceID)

							assert.Equal(t, int64(2), removed)
						})
					}
				})
			})

			t.Run("プレイヤーAがgatewayプロセスの識別子TST-GW1で待機していたが除外されてキューが空のとき、gatewayプロセスの識別子TST-GW2でプレイヤーBを追加すると、追加の結果が示す、追加によってキューから取り除かれたプレイヤーの人数は0になる", func(t *testing.T) {
				q := newQueue(t, client)
				enqueueDefault(t, q, playerA, gatewayOne)
				cancelPlayer(t, q, playerA)

				removed := enqueueDefault(t, q, playerB, gatewayTwo)

				assert.Equal(t, int64(0), removed)
			})

			t.Run("プレイヤーAがgatewayプロセスの識別子TST-GW1で待機しているとき、続けて追加する", func(t *testing.T) {
				cases := []struct {
					name string
					act  func(t *testing.T, q *redisqueue.RedisQueue) int64
				}{
					{
						name: "同じプレイヤーAをgatewayプロセスの識別子TST-GW2で追加するとき、最後の追加の結果が示す、追加によってキューから取り除かれたプレイヤーの人数は1になる",
						act: func(t *testing.T, q *redisqueue.RedisQueue) int64 {
							return enqueueDefault(t, q, playerA, gatewayTwo)
						},
					},
					{
						name: "gatewayプロセスの識別子TST-GW2でプレイヤーB、gatewayプロセスの識別子TST-GW1でプレイヤーCを順に追加するとき、最後の追加の結果が示す、追加によってキューから取り除かれたプレイヤーの人数は1になる",
						act: func(t *testing.T, q *redisqueue.RedisQueue) int64 {
							enqueueDefault(t, q, playerB, gatewayTwo)
							return enqueueDefault(t, q, playerC, gatewayOne)
						},
					},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						q := newQueue(t, client)
						enqueueDefault(t, q, playerA, gatewayOne)

						removed := tc.act(t, q)

						assert.Equal(t, int64(1), removed)
					})
				}
			})
		})
	})
}

func TestCancel(t *testing.T) {
	client := redisqueuetest.Start(t)

	t.Run("[マッチメイキングキュー]プレイヤーの除外", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("プレイヤーAだけが待機しているとき", func(t *testing.T) {
				t.Run("プレイヤーAを除外すると、除外の結果は、プレイヤーがキューにいたことを示す", func(t *testing.T) {
					q := newQueue(t, client)
					enqueueDefault(t, q, playerA, gatewayOne)

					wasQueued := cancelPlayer(t, q, playerA)

					assert.True(t, wasQueued)
				})

				t.Run("プレイヤーAを除外すると、キューサイズは0になる", func(t *testing.T) {
					q := newQueue(t, client)
					enqueueDefault(t, q, playerA, gatewayOne)

					cancelPlayer(t, q, playerA)

					assert.Equal(t, int64(0), readQueueSize(t, q))
				})

				t.Run("プレイヤーAを除外したあと、もう一度プレイヤーAを除外すると、除外の結果は、プレイヤーがキューにいなかったことを示す", func(t *testing.T) {
					q := newQueue(t, client)
					enqueueDefault(t, q, playerA, gatewayOne)
					cancelPlayer(t, q, playerA)

					wasQueued := cancelPlayer(t, q, playerA)

					assert.False(t, wasQueued)
				})
			})

			t.Run("gatewayプロセスの識別子TST-GW1でプレイヤーA、B、Cが追加されて待機しているとき", func(t *testing.T) {
				newWaitingTrio := func(t *testing.T) *redisqueue.RedisQueue {
					t.Helper()
					q := newQueue(t, client)
					enqueueDefault(t, q, playerA, gatewayOne)
					enqueueDefault(t, q, playerB, gatewayOne)
					enqueueDefault(t, q, playerC, gatewayOne)
					return q
				}

				t.Run("プレイヤーBを除外すると、キューサイズは2になる", func(t *testing.T) {
					q := newWaitingTrio(t)

					cancelPlayer(t, q, playerB)

					assert.Equal(t, int64(2), readQueueSize(t, q))
				})

				t.Run("プレイヤーBを除外し、続けてペアを取り出すと、取り出されるのはプレイヤーAとCになる", func(t *testing.T) {
					q := newWaitingTrio(t)
					cancelPlayer(t, q, playerB)

					entries, _ := popPair(t, q)

					assert.ElementsMatch(t, []string{playerA, playerC}, extractPlayerIDs(entries))
				})
			})

			t.Run("プレイヤーAが、gatewayプロセスの識別子TST-GW1でデッキIDを変えて2回追加されて待機しているとき、プレイヤーAを除外すると、キューサイズは0になる", func(t *testing.T) {
				q := newQueue(t, client)
				enqueueDefault(t, q, playerA, gatewayOne)
				enqueue(t, q, playerA, 2, defaultName, defaultLevel, gatewayOne)

				cancelPlayer(t, q, playerA)

				assert.Equal(t, int64(0), readQueueSize(t, q))
			})

			t.Run("プレイヤーAを除外する", func(t *testing.T) {
				cases := []struct {
					name  string
					setup func(t *testing.T, q *redisqueue.RedisQueue)
				}{
					{
						name:  "一度も追加されていないキューのとき、除外の結果は、プレイヤーがキューにいなかったことを示す",
						setup: func(t *testing.T, q *redisqueue.RedisQueue) {},
					},
					{
						name: "プレイヤーBだけが待機しているとき、除外の結果は、プレイヤーがキューにいなかったことを示す",
						setup: func(t *testing.T, q *redisqueue.RedisQueue) {
							enqueueDefault(t, q, playerB, gatewayOne)
						},
					},
					{
						name: "プレイヤーAとBがgatewayプロセスの識別子TST-GW1で追加されてペアとして取り出し済みのとき、除外の結果は、プレイヤーがキューにいなかったことを示す",
						setup: func(t *testing.T, q *redisqueue.RedisQueue) {
							enqueueDefault(t, q, playerA, gatewayOne)
							enqueueDefault(t, q, playerB, gatewayOne)
							popPair(t, q)
						},
					},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						q := newQueue(t, client)
						tc.setup(t, q)

						wasQueued := cancelPlayer(t, q, playerA)

						assert.False(t, wasQueued)
					})
				}
			})

			t.Run("プレイヤーBだけが待機しているとき、プレイヤーAを除外しても、キューサイズは1のままになる", func(t *testing.T) {
				q := newQueue(t, client)
				enqueueDefault(t, q, playerB, gatewayOne)

				cancelPlayer(t, q, playerA)

				assert.Equal(t, int64(1), readQueueSize(t, q))
			})

			t.Run("プレイヤーTST-P1とTST-P10がgatewayプロセスの識別子TST-GW1で追加されて待機していて、一方のプレイヤーIDが他方の先頭と一致するとき", func(t *testing.T) {
				newWaitingPrefixPair := func(t *testing.T) *redisqueue.RedisQueue {
					t.Helper()
					q := newQueue(t, client)
					enqueueDefault(t, q, playerOne, gatewayOne)
					enqueueDefault(t, q, playerTen, gatewayOne)
					return q
				}

				t.Run("プレイヤーTST-P1を除外すると、除外の結果は、プレイヤーがキューにいたことを示す", func(t *testing.T) {
					q := newWaitingPrefixPair(t)

					wasQueued := cancelPlayer(t, q, playerOne)

					assert.True(t, wasQueued)
				})

				t.Run("二人を順に除外する", func(t *testing.T) {
					cases := []struct {
						name   string
						first  string
						second string
					}{
						{name: "除外の順がTST-P1、TST-P10のとき、あとに除外したプレイヤーの除外の結果は、プレイヤーがキューにいたことを示す", first: playerOne, second: playerTen},
						{name: "除外の順がTST-P10、TST-P1のとき、あとに除外したプレイヤーの除外の結果は、プレイヤーがキューにいたことを示す", first: playerTen, second: playerOne},
					}
					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							q := newWaitingPrefixPair(t)
							cancelPlayer(t, q, tc.first)

							wasQueued := cancelPlayer(t, q, tc.second)

							assert.True(t, wasQueued)
						})
					}
				})

				t.Run("待機していないプレイヤーTST-Pを除外すると、除外の結果は、プレイヤーがキューにいなかったことを示す", func(t *testing.T) {
					q := newWaitingPrefixPair(t)

					wasQueued := cancelPlayer(t, q, "TST-P")

					assert.False(t, wasQueued)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("プレイヤーIDが空文字で除外すると、除外は、プレイヤーIDが空であることを示すエラーで失敗する", func(t *testing.T) {
				q := newQueue(t, client)

				_, err := q.Cancel(t.Context(), "")

				assert.ErrorContains(t, err, "playerID is empty")
			})

			t.Run("プレイヤーXが待機しているとき、プレイヤーIDが空文字で除外して失敗したあと、プレイヤーXを除外すると、除外の結果は、プレイヤーがキューにいたことを示す", func(t *testing.T) {
				q := newQueue(t, client)
				enqueueDefault(t, q, playerX, gatewayOne)
				_, err := q.Cancel(t.Context(), "")
				require.Error(t, err)

				wasQueued := cancelPlayer(t, q, playerX)

				assert.True(t, wasQueued)
			})

			t.Run("プレイヤーTST-P1がgatewayプロセスの識別子TST-GW1でデッキIDが1で待機しているとき、プレイヤーIDがコロンを含むTST-P1:1のプレイヤーを除外しても、キューサイズは1のままになる", func(t *testing.T) {
				q := newQueue(t, client)
				enqueue(t, q, playerOne, 1, defaultName, defaultLevel, gatewayOne)

				_, _ = q.Cancel(t.Context(), "TST-P1:1")

				assert.Equal(t, int64(1), readQueueSize(t, q))
			})

			t.Run("ストアに接続できないとき、除外は、ストアに接続できないことを示すエラーで失敗する", func(t *testing.T) {
				q := newUnreachableQueue(t)

				_, err := q.Cancel(t.Context(), playerA)

				assertStoreUnreachableError(t, err)
			})
		})
	})
}

func TestSize(t *testing.T) {
	t.Run("[マッチメイキングキュー]キューサイズの取得", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("ストアに接続できないとき、キューサイズの取得は、ストアに接続できないことを示すエラーで失敗する", func(t *testing.T) {
				q := newUnreachableQueue(t)

				_, err := q.Size(t.Context())

				assertStoreUnreachableError(t, err)
			})
		})
	})
}

func TestPopPair(t *testing.T) {
	client := redisqueuetest.Start(t)

	t.Run("[マッチメイキングキュー]ペアの取り出し", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("待機しているプレイヤーが2人未満のとき、ペアを取り出す", func(t *testing.T) {
				setupNoPlayers := func(t *testing.T, q *redisqueue.RedisQueue) {}
				setupOnePlayer := func(t *testing.T, q *redisqueue.RedisQueue) {
					enqueueDefault(t, q, playerA, gatewayOne)
				}

				entriesCases := []struct {
					name  string
					setup func(t *testing.T, q *redisqueue.RedisQueue)
				}{
					{name: "待機しているプレイヤーが0人のとき、取り出されるプレイヤーは0人になる", setup: setupNoPlayers},
					{name: "待機しているプレイヤーが1人のとき、取り出されるプレイヤーは0人になる", setup: setupOnePlayer},
				}
				for _, tc := range entriesCases {
					t.Run(tc.name, func(t *testing.T) {
						q := newQueue(t, client)
						tc.setup(t, q)

						entries, _ := popPair(t, q)

						assert.Empty(t, entries)
					})
				}

				gatewayCases := []struct {
					name  string
					setup func(t *testing.T, q *redisqueue.RedisQueue)
				}{
					{name: "待機しているプレイヤーが0人のとき、取り出しの結果が示すgatewayプロセスの識別子は空文字になる", setup: setupNoPlayers},
					{name: "待機しているプレイヤーが1人のとき、取り出しの結果が示すgatewayプロセスの識別子は空文字になる", setup: setupOnePlayer},
				}
				for _, tc := range gatewayCases {
					t.Run(tc.name, func(t *testing.T) {
						q := newQueue(t, client)
						tc.setup(t, q)

						_, gatewayInstanceID := popPair(t, q)

						assert.Equal(t, "", gatewayInstanceID)
					})
				}
			})

			t.Run("gatewayプロセスの識別子TST-GW1でプレイヤーAだけが追加されて待機しているとき", func(t *testing.T) {
				t.Run("ペアを取り出すと、キューサイズは1のままになる", func(t *testing.T) {
					q := newQueue(t, client)
					enqueueDefault(t, q, playerA, gatewayOne)

					popPair(t, q)

					assert.Equal(t, int64(1), readQueueSize(t, q))
				})

				t.Run("ペアを取り出したあと、同じgatewayプロセスの識別子TST-GW1でプレイヤーBを追加してペアを取り出すと、取り出されるのはプレイヤーAとBになる", func(t *testing.T) {
					q := newQueue(t, client)
					enqueueDefault(t, q, playerA, gatewayOne)
					popPair(t, q)
					enqueueDefault(t, q, playerB, gatewayOne)

					entries, _ := popPair(t, q)

					assert.ElementsMatch(t, []string{playerA, playerB}, extractPlayerIDs(entries))
				})
			})

			t.Run("デッキID・レベル・名前が互いに異なるプレイヤーAとBが、gatewayプロセスの識別子TST-GW1で追加されて待機しているとき", func(t *testing.T) {
				t.Run("ペアを取り出すと、取り出されるプレイヤーは2人になる", func(t *testing.T) {
					q := newQueue(t, client)
					enqueueDistinctPair(t, q, gatewayOne)

					entries, _ := popPair(t, q)

					assert.Len(t, entries, 2)
				})

				t.Run("ペアを取り出すと、取り出しの結果が示すgatewayプロセスの識別子はTST-GW1になる", func(t *testing.T) {
					q := newQueue(t, client)
					enqueueDistinctPair(t, q, gatewayOne)

					_, gatewayInstanceID := popPair(t, q)

					assert.Equal(t, "TST-GW1", gatewayInstanceID)
				})

				t.Run("ペアを取り出すと、キューサイズは0になる", func(t *testing.T) {
					q := newQueue(t, client)
					enqueueDistinctPair(t, q, gatewayOne)

					popPair(t, q)

					assert.Equal(t, int64(0), readQueueSize(t, q))
				})

				t.Run("ペアを取り出す", func(t *testing.T) {
					joinedAtCases := []struct {
						name     string
						playerID string
					}{
						{name: "取り出されたプレイヤーAの待機開始時刻は、そのプレイヤーを追加した日時になる", playerID: playerA},
						{name: "取り出されたプレイヤーBの待機開始時刻は、そのプレイヤーを追加した日時になる", playerID: playerB},
					}
					for _, tc := range joinedAtCases {
						t.Run(tc.name, func(t *testing.T) {
							q := newQueue(t, client)
							windows := enqueueDistinctPair(t, q, gatewayOne)

							entries, _ := popPair(t, q)

							joinedAt := findEntry(t, entries, tc.playerID).JoinedAt.UnixMilli()
							assert.GreaterOrEqual(t, joinedAt, windows[tc.playerID][0])
							assert.LessOrEqual(t, joinedAt, windows[tc.playerID][1])
						})
					}

					valueCases := []struct {
						name     string
						playerID string
						field    entryField
						want     any
					}{
						{name: "取り出されたプレイヤーAのデッキIDは、追加時と同じになる", playerID: playerA, field: extractDeckID, want: int64(11)},
						{name: "取り出されたプレイヤーAのレベルは、追加時と同じになる", playerID: playerA, field: extractLevel, want: int64(5)},
						{name: "取り出されたプレイヤーAの名前は、追加時と同じになる", playerID: playerA, field: extractName, want: "TST-NAME-A"},
						{name: "取り出されたプレイヤーBのデッキIDは、追加時と同じになる", playerID: playerB, field: extractDeckID, want: int64(22)},
						{name: "取り出されたプレイヤーBのレベルは、追加時と同じになる", playerID: playerB, field: extractLevel, want: int64(7)},
						{name: "取り出されたプレイヤーBの名前は、追加時と同じになる", playerID: playerB, field: extractName, want: "TST-NAME-B"},
					}
					for _, tc := range valueCases {
						t.Run(tc.name, func(t *testing.T) {
							q := newQueue(t, client)
							enqueueDistinctPair(t, q, gatewayOne)

							entries, _ := popPair(t, q)

							assert.Equal(t, tc.want, tc.field(findEntry(t, entries, tc.playerID)))
						})
					}
				})
			})

			t.Run("プレイヤーA、B、Cがgatewayプロセスの識別子TST-GW1で追加されて待機しているとき、ペアを取り出すと、キューサイズは1になる", func(t *testing.T) {
				q := newQueue(t, client)
				enqueueDefault(t, q, playerA, gatewayOne)
				enqueueDefault(t, q, playerB, gatewayOne)
				enqueueDefault(t, q, playerC, gatewayOne)

				popPair(t, q)

				assert.Equal(t, int64(1), readQueueSize(t, q))
			})

			t.Run("プレイヤーA、B、Cが、gatewayプロセスの識別子TST-GW1で、2ミリ秒以上の間隔をあけてこの順に追加されて待機しているとき", func(t *testing.T) {
				t.Run("ペアを取り出すと、取り出されるプレイヤーの順は、プレイヤーA、プレイヤーBになる", func(t *testing.T) {
					q := newQueue(t, client)
					enqueueSpaced(t, q, gatewayOne, playerA, playerB, playerC)

					entries, _ := popPair(t, q)

					assert.Equal(t, []string{playerA, playerB}, extractPlayerIDs(entries))
				})

				t.Run("ペアを取り出したあと、プレイヤーCを追加した2ミリ秒以上あとに、同じgatewayプロセスの識別子TST-GW1でプレイヤーDを追加してペアを取り出すと、取り出されるプレイヤーの順は、プレイヤーC、プレイヤーDになる", func(t *testing.T) {
					q := newQueue(t, client)
					enqueueSpaced(t, q, gatewayOne, playerA, playerB, playerC)
					popPair(t, q)
					enqueueDefault(t, q, playerD, gatewayOne)

					entries, _ := popPair(t, q)

					assert.Equal(t, []string{playerC, playerD}, extractPlayerIDs(entries))
				})
			})

			t.Run("プレイヤーA、B、Cを、Aはgatewayプロセスの識別子TST-GW1、BとCはgatewayプロセスの識別子TST-GW2でこの順に追加したとき", func(t *testing.T) {
				newMixedGatewayQueue := func(t *testing.T) *redisqueue.RedisQueue {
					t.Helper()
					q := newQueue(t, client)
					enqueueSpaced(t, q, gatewayOne, playerA)
					enqueueSpaced(t, q, gatewayTwo, playerB, playerC)
					return q
				}

				t.Run("追加の間隔がどれも2ミリ秒以上のとき、ペアを取り出すと、取り出されるプレイヤーの順は、プレイヤーB、プレイヤーCになる", func(t *testing.T) {
					q := newMixedGatewayQueue(t)

					entries, _ := popPair(t, q)

					assert.Equal(t, []string{playerB, playerC}, extractPlayerIDs(entries))
				})

				t.Run("ペアを取り出すと、取り出しの結果が示すgatewayプロセスの識別子はTST-GW2になる", func(t *testing.T) {
					q := newMixedGatewayQueue(t)

					_, gatewayInstanceID := popPair(t, q)

					assert.Equal(t, "TST-GW2", gatewayInstanceID)
				})
			})

			t.Run("プレイヤーAの名前を指定して、プレイヤーBとともにgatewayプロセスの識別子TST-GW1で追加し、ペアを取り出す", func(t *testing.T) {
				cases := []struct {
					name       string
					playerName string
				}{
					{name: "名前が空文字のとき、取り出されたプレイヤーAの名前は追加した名前と同じになる", playerName: ""},
					{name: "名前がコロンを含むTaro:Hanakoのとき、取り出されたプレイヤーAの名前は追加した名前と同じになる", playerName: "Taro:Hanako"},
					{name: "名前が日本語のテスト太郎のとき、取り出されたプレイヤーAの名前は追加した名前と同じになる", playerName: "テスト太郎"},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						q := newQueue(t, client)
						enqueue(t, q, playerA, defaultDeckID, tc.playerName, defaultLevel, gatewayOne)
						enqueueDefault(t, q, playerB, gatewayOne)

						entries, _ := popPair(t, q)

						assert.Equal(t, tc.playerName, findEntry(t, entries, playerA).Name)
					})
				}
			})

			t.Run("gatewayプロセスの識別子TST-GW1で追加された10人が待機していて、ペアを10回同時に取り出す", func(t *testing.T) {
				popConcurrently := func(t *testing.T) [][]domain.QueueEntry {
					t.Helper()
					q := newQueue(t, client)
					for index := range popPlayerCount {
						enqueueDefault(t, q, fmt.Sprintf("TST-PQ%d", index), gatewayOne)
					}
					results, errs := runConcurrently(popPlayerCount, func(int) ([]domain.QueueEntry, error) {
						entries, _, err := q.PopPair(t.Context())
						return entries, err
					})
					requireNoErrors(t, errs)
					return results
				}

				t.Run("ペアの取り出しのうち、2人が取り出されたものは5回になる", func(t *testing.T) {
					results := popConcurrently(t)

					pairCountsByLength := map[int]int{}
					for _, entries := range results {
						pairCountsByLength[len(entries)]++
					}
					assert.Equal(t, 5, pairCountsByLength[2])
				})

				t.Run("取り出された10人は全員異なるプレイヤーになる", func(t *testing.T) {
					results := popConcurrently(t)

					var poppedPlayerIDs []string
					for _, entries := range results {
						poppedPlayerIDs = append(poppedPlayerIDs, extractPlayerIDs(entries)...)
					}
					assert.ElementsMatch(t, []string{
						"TST-PQ0", "TST-PQ1", "TST-PQ2", "TST-PQ3", "TST-PQ4",
						"TST-PQ5", "TST-PQ6", "TST-PQ7", "TST-PQ8", "TST-PQ9",
					}, poppedPlayerIDs)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("ストアに接続できないとき、ペアの取り出しは、ストアに接続できないことを示すエラーで失敗する", func(t *testing.T) {
				q := newUnreachableQueue(t)

				_, _, err := q.PopPair(t.Context())

				assertStoreUnreachableError(t, err)
			})
		})
	})
}

func TestReenqueue(t *testing.T) {
	client := redisqueuetest.Start(t)

	t.Run("[マッチメイキングキュー]ペアの書き戻し", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("デッキID・レベル・名前が互いに異なるプレイヤーAとBがgatewayプロセスの識別子TST-GW1で追加されて待機していて、ペアを取り出したあと、取り出したときに返されたgatewayプロセスの識別子で書き戻して、もう一度ペアを取り出したとき", func(t *testing.T) {
				cases := []struct {
					name     string
					playerID string
					field    entryField
				}{
					{name: "取り出されたプレイヤーAの待機開始時刻は、書き戻す前に最初にペアを取り出したときと同じになる", playerID: playerA, field: extractJoinedAtMillis},
					{name: "取り出されたプレイヤーBの待機開始時刻は、書き戻す前に最初にペアを取り出したときと同じになる", playerID: playerB, field: extractJoinedAtMillis},
					{name: "取り出されたプレイヤーAのデッキIDは、書き戻す前に最初にペアを取り出したときと同じになる", playerID: playerA, field: extractDeckID},
					{name: "取り出されたプレイヤーAのレベルは、書き戻す前に最初にペアを取り出したときと同じになる", playerID: playerA, field: extractLevel},
					{name: "取り出されたプレイヤーAの名前は、書き戻す前に最初にペアを取り出したときと同じになる", playerID: playerA, field: extractName},
					{name: "取り出されたプレイヤーBのデッキIDは、書き戻す前に最初にペアを取り出したときと同じになる", playerID: playerB, field: extractDeckID},
					{name: "取り出されたプレイヤーBのレベルは、書き戻す前に最初にペアを取り出したときと同じになる", playerID: playerB, field: extractLevel},
					{name: "取り出されたプレイヤーBの名前は、書き戻す前に最初にペアを取り出したときと同じになる", playerID: playerB, field: extractName},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						q := newQueue(t, client)
						enqueueDistinctPair(t, q, gatewayOne)
						first, gatewayInstanceID := popPair(t, q)
						reenqueue(t, q, first, gatewayInstanceID)

						second, _ := popPair(t, q)

						assert.Equal(t, tc.field(findEntry(t, first, tc.playerID)), tc.field(findEntry(t, second, tc.playerID)))
					})
				}
			})

			t.Run("プレイヤーAとBをgatewayプロセスの識別子TST-GW1で追加してペアを取り出したあと、gatewayプロセスの識別子TST-GW1でプレイヤーCを追加し、取り出したときに返されたgatewayプロセスの識別子でAとBを書き戻したとき", func(t *testing.T) {
				newReenqueuedQueue := func(t *testing.T) *redisqueue.RedisQueue {
					t.Helper()
					q := newQueue(t, client)
					enqueueSpaced(t, q, gatewayOne, playerA, playerB)
					entries, gatewayInstanceID := popPair(t, q)
					enqueueDefault(t, q, playerC, gatewayOne)
					reenqueue(t, q, entries, gatewayInstanceID)
					return q
				}

				t.Run("A、B、Cの追加の間隔がどれも2ミリ秒以上のとき、ペアを取り出すと、取り出されるプレイヤーの順は、プレイヤーA、プレイヤーBになる", func(t *testing.T) {
					q := newReenqueuedQueue(t)

					entries, _ := popPair(t, q)

					assert.Equal(t, []string{playerA, playerB}, extractPlayerIDs(entries))
				})

				t.Run("ペアを取り出すと、キューサイズは1になる", func(t *testing.T) {
					q := newReenqueuedQueue(t)

					popPair(t, q)

					assert.Equal(t, int64(1), readQueueSize(t, q))
				})
			})

			t.Run("gatewayプロセスの識別子TST-GW1でプレイヤーAを追加したあと、プレイヤーA以外のプレイヤーを、同じgatewayプロセスの識別子TST-GW1で書き戻す", func(t *testing.T) {
				cases := []struct {
					name    string
					entries []domain.QueueEntry
				}{
					{name: "書き戻すプレイヤーが1人のとき、書き戻しの結果は、書き戻しが成立したことを示す", entries: newEntries("TST-PR1")},
					{name: "書き戻すプレイヤーが2人のとき、書き戻しの結果は、書き戻しが成立したことを示す", entries: newEntries("TST-PR1", "TST-PR2")},
					{name: "書き戻すプレイヤーが3人のとき、書き戻しの結果は、書き戻しが成立したことを示す", entries: newEntries("TST-PR1", "TST-PR2", "TST-PR3")},
					{name: "書き戻すプレイヤーを何も指定しないとき、書き戻しの結果は、書き戻しが成立したことを示す", entries: nil},
					{name: "書き戻すプレイヤーの一覧が空のとき、書き戻しの結果は、書き戻しが成立したことを示す", entries: []domain.QueueEntry{}},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						q := newQueue(t, client)
						enqueueDefault(t, q, playerA, gatewayOne)

						isWrittenBack := reenqueue(t, q, tc.entries, gatewayOne)

						assert.True(t, isWrittenBack)
					})
				}
			})

			t.Run("プレイヤーAがgatewayプロセスの識別子TST-GW1で追加されて待機しているとき、プレイヤーA以外のプレイヤーを、同じgatewayプロセスの識別子TST-GW1で書き戻す", func(t *testing.T) {
				cases := []struct {
					name     string
					entries  []domain.QueueEntry
					wantSize int64
				}{
					{name: "書き戻すプレイヤーが1人のとき、キューサイズは書き戻す前のキューサイズに書き戻したプレイヤーの人数を足した値になる", entries: newEntries("TST-PR1"), wantSize: 2},
					{name: "書き戻すプレイヤーが2人のとき、キューサイズは書き戻す前のキューサイズに書き戻したプレイヤーの人数を足した値になる", entries: newEntries("TST-PR1", "TST-PR2"), wantSize: 3},
					{name: "書き戻すプレイヤーが3人のとき、キューサイズは書き戻す前のキューサイズに書き戻したプレイヤーの人数を足した値になる", entries: newEntries("TST-PR1", "TST-PR2", "TST-PR3"), wantSize: 4},
					{name: "書き戻すプレイヤーを何も指定しないとき、キューサイズは書き戻す前のキューサイズに書き戻したプレイヤーの人数を足した値になる", entries: nil, wantSize: 1},
					{name: "書き戻すプレイヤーの一覧が空のとき、キューサイズは書き戻す前のキューサイズに書き戻したプレイヤーの人数を足した値になる", entries: []domain.QueueEntry{}, wantSize: 1},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						q := newQueue(t, client)
						enqueueDefault(t, q, playerA, gatewayOne)
						reenqueue(t, q, tc.entries, gatewayOne)

						assert.Equal(t, tc.wantSize, readQueueSize(t, q))
					})
				}
			})

			t.Run("プレイヤーAがgatewayプロセスの識別子TST-GW1でデッキIDが1で待機しているとき、同じgatewayプロセスの識別子TST-GW1で、デッキIDが2のプレイヤーAを書き戻す", func(t *testing.T) {
				newOverwrittenQueue := func(t *testing.T) *redisqueue.RedisQueue {
					t.Helper()
					q := newQueue(t, client)
					enqueue(t, q, playerA, 1, defaultName, defaultLevel, gatewayOne)
					rewritten := newEntry(playerA)
					rewritten.DeckID = 2
					reenqueue(t, q, []domain.QueueEntry{rewritten}, gatewayOne)
					return q
				}

				t.Run("キューサイズは1のままになる", func(t *testing.T) {
					q := newOverwrittenQueue(t)

					assert.Equal(t, int64(1), readQueueSize(t, q))
				})

				t.Run("同じgatewayプロセスの識別子TST-GW1でプレイヤーBを追加してペアを取り出すと、取り出されたプレイヤーAのデッキIDは2になる", func(t *testing.T) {
					q := newOverwrittenQueue(t)
					enqueueDefault(t, q, playerB, gatewayOne)

					entries, _ := popPair(t, q)

					assert.Equal(t, int64(2), findEntry(t, entries, playerA).DeckID)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("プレイヤーIDが空文字のプレイヤーを書き戻すと、書き戻しは、プレイヤーIDが空であることを示すエラーで失敗する", func(t *testing.T) {
				q := newQueue(t, client)
				enqueueDefault(t, q, playerA, gatewayOne)

				_, err := q.Reenqueue(t.Context(), newEntries(""), gatewayOne)

				assert.ErrorContains(t, err, "playerID is empty")
			})

			t.Run("プレイヤーIDがコロンを含むTST:P1のプレイヤーを書き戻すと、書き戻しは、プレイヤーIDに使えない文字を含むことを示すエラーで失敗する", func(t *testing.T) {
				q := newQueue(t, client)
				enqueueDefault(t, q, playerA, gatewayOne)

				_, err := q.Reenqueue(t.Context(), newEntries("TST:P1"), gatewayOne)

				assert.ErrorContains(t, err, "playerID contains ':'")
			})

			t.Run("gatewayプロセスの識別子TST-GW10でプレイヤーAを追加したあと、別のgatewayプロセスの識別子TST-GW2で書き戻す", func(t *testing.T) {
				cases := []struct {
					name    string
					entries []domain.QueueEntry
				}{
					{name: "書き戻すプレイヤーを何も指定しないとき、書き戻しの結果は、書き戻しが成立しなかったことを示す", entries: nil},
					{name: "書き戻すプレイヤーの一覧が空のとき、書き戻しの結果は、書き戻しが成立しなかったことを示す", entries: []domain.QueueEntry{}},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						q := newQueue(t, client)
						enqueueDefault(t, q, playerA, "TST-GW10")

						isWrittenBack := reenqueue(t, q, tc.entries, gatewayTwo)

						assert.False(t, isWrittenBack)
					})
				}
			})

			t.Run("一度も追加されていない空のキューに、2人を書き戻す", func(t *testing.T) {
				t.Run("空文字のgatewayプロセスの識別子で書き戻すと、書き戻しの結果は、書き戻しが成立したことを示す", func(t *testing.T) {
					q := newQueue(t, client)

					isWrittenBack := reenqueue(t, q, newEntries(playerA, playerB), "")

					assert.True(t, isWrittenBack)
				})

				t.Run("空文字のgatewayプロセスの識別子で書き戻すと、キューサイズは2になる", func(t *testing.T) {
					q := newQueue(t, client)
					reenqueue(t, q, newEntries(playerA, playerB), "")

					assert.Equal(t, int64(2), readQueueSize(t, q))
				})

				t.Run("gatewayプロセスの識別子TST-GW1で書き戻すと、書き戻しの結果は、書き戻しが成立しなかったことを示す", func(t *testing.T) {
					q := newQueue(t, client)

					isWrittenBack := reenqueue(t, q, newEntries(playerA, playerB), gatewayOne)

					assert.False(t, isWrittenBack)
				})

				t.Run("gatewayプロセスの識別子TST-GW1で書き戻すと、キューサイズは0のままになる", func(t *testing.T) {
					q := newQueue(t, client)
					reenqueue(t, q, newEntries(playerA, playerB), gatewayOne)

					assert.Equal(t, int64(0), readQueueSize(t, q))
				})
			})

			t.Run("プレイヤーAとBをgatewayプロセスの識別子TST-GW1で追加して取り出したあとに、別のgatewayプロセスの識別子TST-GW2でプレイヤーCが追加されたとき", func(t *testing.T) {
				newSupersededQueue := func(t *testing.T) (*redisqueue.RedisQueue, []domain.QueueEntry, string) {
					t.Helper()
					q := newQueue(t, client)
					enqueueDefault(t, q, playerA, gatewayOne)
					enqueueDefault(t, q, playerB, gatewayOne)
					entries, gatewayInstanceID := popPair(t, q)
					enqueueDefault(t, q, playerC, gatewayTwo)
					return q, entries, gatewayInstanceID
				}

				t.Run("取り出したプレイヤーAとBを、取り出したときに返されたgatewayプロセスの識別子で書き戻すと、書き戻しの結果は、書き戻しが成立しなかったことを示す", func(t *testing.T) {
					q, entries, gatewayInstanceID := newSupersededQueue(t)

					isWrittenBack := reenqueue(t, q, entries, gatewayInstanceID)

					assert.False(t, isWrittenBack)
				})

				t.Run("取り出したプレイヤーAとBを、取り出したときに返されたgatewayプロセスの識別子で書き戻すと、キューサイズは1のままになる", func(t *testing.T) {
					q, entries, gatewayInstanceID := newSupersededQueue(t)
					reenqueue(t, q, entries, gatewayInstanceID)

					assert.Equal(t, int64(1), readQueueSize(t, q))
				})
			})

			t.Run("gatewayプロセスの識別子TST-GW10でプレイヤーを追加したあと、渡すgatewayプロセスの識別子を変えて2人を書き戻す", func(t *testing.T) {
				cases := []struct {
					name              string
					gatewayInstanceID string
				}{
					{name: "空文字のとき、書き戻しの結果は、書き戻しが成立しなかったことを示す", gatewayInstanceID: ""},
					{name: "末尾の1文字を欠いたTST-GW1のとき、書き戻しの結果は、書き戻しが成立しなかったことを示す", gatewayInstanceID: "TST-GW1"},
					{name: "末尾に1文字足したTST-GW100のとき、書き戻しの結果は、書き戻しが成立しなかったことを示す", gatewayInstanceID: "TST-GW100"},
					{name: "大文字小文字だけが違うtst-gw10のとき、書き戻しの結果は、書き戻しが成立しなかったことを示す", gatewayInstanceID: "tst-gw10"},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						q := newQueue(t, client)
						enqueueDefault(t, q, playerA, "TST-GW10")

						isWrittenBack := reenqueue(t, q, newEntries(playerB, playerC), tc.gatewayInstanceID)

						assert.False(t, isWrittenBack)
					})
				}

				t.Run("gatewayプロセスの識別子TST-GW1での書き戻しの結果が、書き戻しが成立しなかったことを示すものになったあと、gatewayプロセスの識別子TST-GW10で書き戻すと、書き戻しの結果は、書き戻しが成立したことを示す", func(t *testing.T) {
					q := newQueue(t, client)
					enqueueDefault(t, q, playerA, "TST-GW10")
					reenqueue(t, q, newEntries(playerB, playerC), "TST-GW1")

					isWrittenBack := reenqueue(t, q, newEntries(playerB, playerC), "TST-GW10")

					assert.True(t, isWrittenBack)
				})
			})

			t.Run("ストアに接続できないとき、書き戻しは、ストアに接続できないことを示すエラーで失敗する", func(t *testing.T) {
				q := newUnreachableQueue(t)

				_, err := q.Reenqueue(t.Context(), newEntries(playerA), gatewayOne)

				assertStoreUnreachableError(t, err)
			})
		})
	})
}
