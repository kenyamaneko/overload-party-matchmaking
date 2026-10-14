//go:build integration

package pubsub_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	gpubsub "cloud.google.com/go/pubsub/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/pubsub/pubsubtest"
)

const (
	emulatorStartTimeout = 3 * time.Minute
	emulatorStopTimeout  = 30 * time.Second
	receiveTimeout       = 15 * time.Second
	absenceCheckTimeout  = 3 * time.Second
	publishTimeoutLimit  = time.Minute
)

var emulator *pubsubtest.Emulator

func TestMain(m *testing.M) {
	startCtx, cancelStart := context.WithTimeout(context.Background(), emulatorStartTimeout)
	started, err := pubsubtest.StartEmulator(startCtx, testProjectID)
	cancelStart()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start pubsub emulator: %v\n", err)
		os.Exit(1)
	}
	emulator = started

	code := m.Run()

	stopCtx, cancelStop := context.WithTimeout(context.Background(), emulatorStopTimeout)
	defer cancelStop()
	if err := emulator.Close(stopCtx); err != nil {
		fmt.Fprintf(os.Stderr, "stop pubsub emulator: %v\n", err)
		code = 1
	}
	os.Exit(code)
}

func newEmulatorPublisherClosedByTest(t *testing.T, topicName string) *pubsub.Publisher {
	t.Helper()
	p, err := pubsub.New(context.Background(), testProjectID, topicName, publishTimeoutLimit)
	require.NoError(t, err)
	return p
}

func newEmulatorPublisher(t *testing.T, topicName string) *pubsub.Publisher {
	t.Helper()
	p := newEmulatorPublisherClosedByTest(t, topicName)
	t.Cleanup(func() { assert.NoError(t, p.Close()) })
	return p
}

func extractBodies(msgs []*pubsubtest.Message) []string {
	bodies := make([]string, 0, len(msgs))
	for _, m := range msgs {
		bodies = append(bodies, string(m.Data))
	}
	return bodies
}

func receiveBodies(t *testing.T, sub *pubsubtest.Subscription, count int) []string {
	t.Helper()
	msgs, err := sub.WaitForN(count, receiveTimeout)
	require.NoError(t, err)
	return extractBodies(msgs)
}

func TestPublish(t *testing.T) {
	t.Run("[マッチ成立通知]送出", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("match_madeの送出先トピックが存在するとき", func(t *testing.T) {
				t.Run("match_madeを送ると、購読で受け取った本文は送った本文と一致する", func(t *testing.T) {
					topicName := emulator.CreateTopic(t, "tst-topic")
					sub := emulator.Subscribe(t, topicName)
					p := newEmulatorPublisher(t, topicName)
					body := pubsubtest.BuildMatchMadeBody("mch_tst0001")

					err := p.Publish(context.Background(), "match_made", []byte(body))
					require.NoError(t, err)

					assert.Equal(t, []string{body}, receiveBodies(t, sub, 1))
				})

				t.Run("本文の異なる2件を続けてmatch_madeで送ると、購読で受け取った本文は、送った2件の本文と過不足なく一致する", func(t *testing.T) {
					topicName := emulator.CreateTopic(t, "tst-topic")
					sub := emulator.Subscribe(t, topicName)
					p := newEmulatorPublisher(t, topicName)
					bodyFirst := pubsubtest.BuildMatchMadeBody("mch_tst0002")
					bodySecond := pubsubtest.BuildMatchMadeBody("mch_tst0003")

					require.NoError(t, p.Publish(context.Background(), "match_made", []byte(bodyFirst)))
					require.NoError(t, p.Publish(context.Background(), "match_made", []byte(bodySecond)))

					assert.ElementsMatch(t, []string{bodyFirst, bodySecond}, receiveBodies(t, sub, 2))
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("match_madeの送出先トピックが存在するとき、match_made以外のイベント種別no_such_eventで送って失敗したあとにmatch_madeを送ると、購読で受け取った本文はmatch_madeで送った本文の1件だけになる", func(t *testing.T) {
				topicName := emulator.CreateTopic(t, "tst-topic")
				sub := emulator.Subscribe(t, topicName)
				p := newEmulatorPublisher(t, topicName)
				body := pubsubtest.BuildMatchMadeBody("mch_tst0004")

				failedErr := p.Publish(context.Background(), "no_such_event", []byte(pubsubtest.BuildMatchMadeBody("mch_tst0005")))
				require.ErrorContains(t, failedErr, `unknown event type "no_such_event"`)
				require.NoError(t, p.Publish(context.Background(), "match_made", []byte(body)))

				msgs, err := sub.WaitForN(2, absenceCheckTimeout)
				require.ErrorIs(t, err, pubsubtest.ErrTimeout)
				assert.Equal(t, []string{body}, extractBodies(msgs))
			})

			t.Run("match_madeの送出先トピックが存在しないとき", func(t *testing.T) {
				t.Run("match_madeを送ると、match_madeの送出が送出先トピックが存在しないために失敗したことを示すエラーになる", func(t *testing.T) {
					p := newEmulatorPublisher(t, "tst-topic-absent")

					err := p.Publish(context.Background(), "match_made", []byte(pubsubtest.BuildMatchMadeBody("mch_tst0006")))

					require.Error(t, err)
					assert.Equal(t, codes.NotFound, status.Code(err))
				})

				t.Run("match_madeを送って失敗したあと、そのトピックを作成してからmatch_madeを送り直すと、購読で受け取った本文は送り直した本文と一致する", func(t *testing.T) {
					topicName := pubsubtest.GenerateTopicName("tst-topic-recovering")
					p := newEmulatorPublisher(t, topicName)
					body := pubsubtest.BuildMatchMadeBody("mch_tst0007")
					require.Error(t, p.Publish(context.Background(), "match_made", []byte(pubsubtest.BuildMatchMadeBody("mch_tst0008"))))
					emulator.CreateTopicWithName(t, topicName)
					sub := emulator.Subscribe(t, topicName)

					err := p.Publish(context.Background(), "match_made", []byte(body))
					require.NoError(t, err)

					assert.Equal(t, []string{body}, receiveBodies(t, sub, 1))
				})
			})

			t.Run("アプリがシャットダウンの指示を受けたあとのとき、match_madeを送ると、シャットダウンで取り消されたことを示すエラーになる", func(t *testing.T) {
				topicName := emulator.CreateTopic(t, "tst-topic")
				p := newEmulatorPublisher(t, topicName)
				shutdownCtx, cancel := context.WithCancel(context.Background())
				cancel()

				err := p.Publish(shutdownCtx, "match_made", []byte(pubsubtest.BuildMatchMadeBody("mch_tst0009")))

				require.Error(t, err)
				assert.ErrorIs(t, err, context.Canceled)
			})
		})
	})
}

func TestClose(t *testing.T) {
	t.Run("[マッチ成立通知]送出の終了処理", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("match_madeの送出先トピックが存在し、送出の終了処理を行ったあとに送るとき", func(t *testing.T) {
				tests := []struct {
					name             string
					bodiesBeforeStop []string
				}{
					{
						name:             "終了処理の前に一度も送っていないとき、終了処理済みのため送れないことを示すエラーになる",
						bodiesBeforeStop: nil,
					},
					{
						name:             "終了処理の前にmatch_madeの送出が1件成功していたとき、終了処理済みのため送れないことを示すエラーになる",
						bodiesBeforeStop: []string{pubsubtest.BuildMatchMadeBody("mch_tst0010")},
					},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						topicName := emulator.CreateTopic(t, "tst-topic")
						p := newEmulatorPublisherClosedByTest(t, topicName)
						for _, body := range tt.bodiesBeforeStop {
							require.NoError(t, p.Publish(context.Background(), "match_made", []byte(body)))
						}
						require.NoError(t, p.Close())

						err := p.Publish(context.Background(), "match_made", []byte(pubsubtest.BuildMatchMadeBody("mch_tst0011")))

						require.Error(t, err)
						assert.ErrorIs(t, err, gpubsub.ErrPublisherStopped)
					})
				}
			})

			t.Run("match_madeの送出先トピックが存在し、送出の終了処理を行ったあと、送出の初期化処理をやり直してmatch_madeを送ると、購読で受け取った本文は送った本文と一致する", func(t *testing.T) {
				topicName := emulator.CreateTopic(t, "tst-topic")
				sub := emulator.Subscribe(t, topicName)
				stopped := newEmulatorPublisherClosedByTest(t, topicName)
				require.NoError(t, stopped.Close())
				restarted := newEmulatorPublisher(t, topicName)
				body := pubsubtest.BuildMatchMadeBody("mch_tst0012")

				err := restarted.Publish(context.Background(), "match_made", []byte(body))
				require.NoError(t, err)

				assert.Equal(t, []string{body}, receiveBodies(t, sub, 1))
			})
		})
	})
}
