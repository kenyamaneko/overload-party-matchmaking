//go:build integration

package pubsub_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/pubsub/pubsubtest"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

func TestNew(t *testing.T) {
	t.Run("Publisherの生成", func(t *testing.T) {
		t.Run("プロジェクトIDが空文字の状態でpublisherを生成しようとすると、生成に失敗する", func(t *testing.T) {
			_, err := pubsub.New(context.Background(), "", "some-topic")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "projectID")
		})

		t.Run("トピック名が空文字の状態でpublisherを生成しようとすると、生成に失敗する", func(t *testing.T) {
			_, err := pubsub.New(context.Background(), testProjectID, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "matchMadeTopic")
		})

		t.Run("プロジェクトIDとトピック名が設定された状態でpublisherを生成すると、生成に成功する", func(t *testing.T) {
			topicID := sharedEmulator.CreateTopic(t, "matchmade")

			p, err := pubsub.New(context.Background(), testProjectID, topicID)

			require.NoError(t, err)
			require.NoError(t, p.Close())
		})
	})
}

func TestPublish(t *testing.T) {
	t.Run("イベント発行", func(t *testing.T) {
		t.Run("match_madeのevent_typeでpublishすると、エミュレータ側で該当topicにそのペイロードのメッセージが届く", func(t *testing.T) {
			topicID := sharedEmulator.CreateTopic(t, "matchmade")
			sub := sharedEmulator.Subscribe(t, topicID)
			p, err := pubsub.New(context.Background(), testProjectID, topicID)
			require.NoError(t, err)
			defer func() { _ = p.Close() }()

			payload := []byte(`{"match_id":"mch_1"}`)
			err = p.Publish(context.Background(), apimatchmaking.EventTypeMatchMade, payload)
			require.NoError(t, err)

			msg, err := sub.WaitForMessage(context.Background(), 5*time.Second)
			require.NoError(t, err)
			assert.Equal(t, payload, msg.Data)
		})

		t.Run("未登録のevent_typeでpublishしようとすると、errorになる", func(t *testing.T) {
			topicID := sharedEmulator.CreateTopic(t, "matchmade")
			p, err := pubsub.New(context.Background(), testProjectID, topicID)
			require.NoError(t, err)
			defer func() { _ = p.Close() }()

			err = p.Publish(context.Background(), "unknown_event", []byte(`{}`))

			assert.Error(t, err)
		})

		t.Run("未登録のevent_typeでpublishしようとしても、エミュレータ側にメッセージは届かない", func(t *testing.T) {
			topicID := sharedEmulator.CreateTopic(t, "matchmade")
			sub := sharedEmulator.Subscribe(t, topicID)
			p, err := pubsub.New(context.Background(), testProjectID, topicID)
			require.NoError(t, err)
			defer func() { _ = p.Close() }()

			_ = p.Publish(context.Background(), "unknown_event", []byte(`{}`))

			_, err = sub.WaitForMessage(context.Background(), 500*time.Millisecond)
			assert.ErrorIs(t, err, pubsubtest.ErrTimeout)
		})

		t.Run("未登録のevent_typeでpublishが失敗したあと、match_madeでpublishすると成功し、エミュレータ側にメッセージが届く", func(t *testing.T) {
			topicID := sharedEmulator.CreateTopic(t, "matchmade")
			sub := sharedEmulator.Subscribe(t, topicID)
			p, err := pubsub.New(context.Background(), testProjectID, topicID)
			require.NoError(t, err)
			defer func() { _ = p.Close() }()

			require.Error(t, p.Publish(context.Background(), "unknown_event", []byte(`{}`)))

			payload := []byte(`{"match_id":"mch_2"}`)
			err = p.Publish(context.Background(), apimatchmaking.EventTypeMatchMade, payload)
			require.NoError(t, err)

			msg, err := sub.WaitForMessage(context.Background(), 5*time.Second)
			require.NoError(t, err)
			assert.Equal(t, payload, msg.Data)
		})
	})
}
