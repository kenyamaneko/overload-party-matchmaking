//go:build integration

package pubsub

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/pubsub/pubsubtest"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/presenter"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

var sharedEmulator *pubsubtest.Emulator

// TestMain は package 内の全 integration test で共有する emulator を起動する。
// container 起動コストは高いので per-test ではなく package scope で償却する。
// test 毎の分離は topic / subscription の UUID suffix で担保する。
func TestMain(m *testing.M) {
	ctx := context.Background()
	em, err := pubsubtest.StartEmulator(ctx, "matchmaking-test")
	if err != nil {
		log.Fatalf("start pubsub emulator: %v", err)
	}
	sharedEmulator = em

	code := m.Run()

	if cerr := em.Close(ctx); cerr != nil {
		log.Printf("close emulator: %v", cerr)
	}
	os.Exit(code)
}

// setupPublisher は emulator 上に match-made topic を作成した Publisher を返す。
func setupPublisher(t *testing.T) (*Publisher, string) {
	t.Helper()
	topic := sharedEmulator.CreateTopic(t, "match-made")

	ctx := context.Background()
	pub, err := New(ctx, sharedEmulator.ProjectID(), topic)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pub.Close() })

	return pub, topic
}

func TestPublishIntegration(t *testing.T) {
	t.Run("PublisherのPub/Sub配信", func(t *testing.T) {
		t.Run("マッチ成立イベントを配信すると、購読者に配信内容がそのまま届く", func(t *testing.T) {
			pub, topic := setupPublisher(t)
			sub := sharedEmulator.Subscribe(t, topic)

			ctx := context.Background()
			event := domain.MatchMadeEvent{
				MatchID: "mch_TST-0001",
				Players: []domain.MatchedPlayer{
					{PlayerID: "p1", DeckID: 1, Name: "alice", Level: 7},
					{PlayerID: "p2", DeckID: 2, Name: "bob", Level: 12},
				},
			}
			eventType, payload, err := presenter.ToMatchMadeWire(event)
			require.NoError(t, err)
			require.NoError(t, pub.Publish(ctx, eventType, payload))

			msg, err := sub.WaitForMessage(ctx, 5*time.Second)
			require.NoError(t, err)
			assert.Equal(t, payload, msg.Data, "受信内容が送信内容とバイト一致する")

			var decoded apimatchmaking.MatchMadeEvent
			require.NoError(t, json.Unmarshal(msg.Data, &decoded))
			assert.Equal(t, apimatchmaking.EventTypeMatchMade, decoded.EventType)
			assert.Equal(t, "mch_TST-0001", decoded.MatchID)
			require.Len(t, decoded.Players, 2)
			assert.Equal(t, "p1", decoded.Players[0].PlayerID)
			assert.Equal(t, "p2", decoded.Players[1].PlayerID)
		})

		t.Run("何も配信しないとき、購読者には何も届かない", func(t *testing.T) {
			_, topic := setupPublisher(t)
			sub := sharedEmulator.Subscribe(t, topic)

			_, err := sub.WaitForMessage(context.Background(), 500*time.Millisecond)
			assert.ErrorIs(t, err, pubsubtest.ErrTimeout)
		})
	})
}
