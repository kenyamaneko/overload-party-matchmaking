package pubsub_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/pubsub/pubsubtest"
)

const (
	unreachableEmulatorHost = "127.0.0.1:1"
	testProjectID           = "tst-project"
	testTopicName           = "tst-topic"
	testMatchID             = "mch_tst0001"
	shortPublishTimeout     = 200 * time.Millisecond
)

func newUnreachablePublisher(t *testing.T, publishTimeout time.Duration) *pubsub.Publisher {
	t.Helper()
	t.Setenv("PUBSUB_EMULATOR_HOST", unreachableEmulatorHost)
	p, err := pubsub.New(context.Background(), testProjectID, testTopicName, publishTimeout)
	require.NoError(t, err)
	return p
}

func TestPublisherConfigValidation(t *testing.T) {
	t.Run("[マッチ成立通知]送出先の設定検証", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("match_madeの送出の初期化処理を行うとき", func(t *testing.T) {
				tests := []struct {
					name           string
					projectID      string
					topicName      string
					publishTimeout time.Duration
					wantMessage    string
				}{
					{
						name:           "プロジェクトIDが空文字のとき、プロジェクトIDが空であることを示すエラーになる",
						projectID:      "",
						topicName:      testTopicName,
						publishTimeout: time.Minute,
						wantMessage:    "projectID is empty",
					},
					{
						name:           "送出先トピック名が空文字のとき、送出先トピック名が空であることを示すエラーになる",
						projectID:      testProjectID,
						topicName:      "",
						publishTimeout: time.Minute,
						wantMessage:    "matchMadeTopic is required",
					},
					{
						name:           "送出の待ち時間の上限が0秒のとき、送出の待ち時間の上限が0以下であることを示すエラーになる",
						projectID:      testProjectID,
						topicName:      testTopicName,
						publishTimeout: 0,
						wantMessage:    "must be positive",
					},
					{
						name:           "送出の待ち時間の上限が-1秒のとき、送出の待ち時間の上限が0以下であることを示すエラーになる",
						projectID:      testProjectID,
						topicName:      testTopicName,
						publishTimeout: -1 * time.Second,
						wantMessage:    "must be positive",
					},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						t.Setenv("PUBSUB_EMULATOR_HOST", unreachableEmulatorHost)

						_, err := pubsub.New(context.Background(), tt.projectID, tt.topicName, tt.publishTimeout)

						require.Error(t, err)
						assert.ErrorContains(t, err, tt.wantMessage)
					})
				}
			})
		})
	})
}

func TestPublishEventTypeValidation(t *testing.T) {
	t.Run("[マッチ成立通知]送出できるイベント種別の検証", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("match_made以外のイベント種別を送るとき", func(t *testing.T) {
				tests := []struct {
					name        string
					eventType   string
					wantMessage string
				}{
					{
						name:        "イベント種別が空文字のとき、そのイベント種別に対応していないことを示すエラーになる",
						eventType:   "",
						wantMessage: `unknown event type ""`,
					},
					{
						name:        "イベント種別がmatch-made(ハイフン区切り)のとき、そのイベント種別に対応していないことを示すエラーになる",
						eventType:   "match-made",
						wantMessage: `unknown event type "match-made"`,
					},
					{
						name:        "イベント種別がMATCH_MADE(大文字)のとき、そのイベント種別に対応していないことを示すエラーになる",
						eventType:   "MATCH_MADE",
						wantMessage: `unknown event type "MATCH_MADE"`,
					},
					{
						name:        "イベント種別がno_such_eventのとき、そのイベント種別に対応していないことを示すエラーになる",
						eventType:   "no_such_event",
						wantMessage: `unknown event type "no_such_event"`,
					},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						p := newUnreachablePublisher(t, time.Minute)

						err := p.Publish(context.Background(), tt.eventType, []byte(pubsubtest.BuildMatchMadeBody(testMatchID)))

						require.Error(t, err)
						assert.ErrorContains(t, err, tt.wantMessage)
					})
				}
			})
		})
	})
}

func TestPublishTimeout(t *testing.T) {
	t.Run("[マッチ成立通知]送出の待ち時間の上限", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("送出先に何も待ち受けていない状態でmatch_madeを送るとき", func(t *testing.T) {
				t.Run("設定した待ち時間の上限に達すると、送出が期限切れになったことを示すエラーになる", func(t *testing.T) {
					p := newUnreachablePublisher(t, shortPublishTimeout)

					err := p.Publish(context.Background(), "match_made", []byte(pubsubtest.BuildMatchMadeBody(testMatchID)))

					require.Error(t, err)
					assert.ErrorIs(t, err, context.DeadlineExceeded)
				})
			})
		})
	})
}
