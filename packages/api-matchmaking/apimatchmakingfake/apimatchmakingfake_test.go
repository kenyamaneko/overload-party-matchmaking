package apimatchmakingfake_test

import (
	"context"
	"errors"
	"testing"
	"time"

	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
	"github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking/apimatchmakingfake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroker(t *testing.T) {
	t.Run("ブローカーの配送と分離", func(t *testing.T) {
		t.Run("publish した payload は同一 broker の subscriber に届き、topic が異なると届かない", func(t *testing.T) {
			broker := apimatchmakingfake.NewBroker()
			pub := apimatchmakingfake.NewPublisher(broker)
			sub := apimatchmakingfake.NewSubscriber(broker)

			chA := sub.Messages("topic-a")
			chB := sub.Messages("topic-b")

			require.NoError(t, pub.Publish(context.Background(), "topic-a", []byte(`a`)))

			select {
			case got := <-chA:
				assert.Equal(t, `a`, string(got))
			case <-time.After(time.Second):
				t.Fatal("topic-a subscriber did not receive")
			}
			select {
			case got := <-chB:
				t.Fatalf("topic-b should not receive, got %s", got)
			case <-time.After(50 * time.Millisecond):
			}
		})
	})
}

func TestPublisher(t *testing.T) {
	t.Run("Publisher の発行スナップショット", func(t *testing.T) {
		t.Run("Published() は publish 順に snapshot を返し、呼び出し側の変更が内部状態に影響しない", func(t *testing.T) {
			broker := apimatchmakingfake.NewBroker()
			pub := apimatchmakingfake.NewPublisher(broker)
			ctx := context.Background()

			require.NoError(t, pub.Publish(ctx, "t", []byte(`a`)))
			require.NoError(t, pub.Publish(ctx, "t", []byte(`b`)))

			snap := pub.Published()
			require.Len(t, snap, 2)
			snap[0].Data[0] = 'X' // caller mutation should not affect internal state

			again := pub.Published()
			assert.Equal(t, `a`, string(again[0].Data))
		})
	})
}

func TestStream(t *testing.T) {
	t.Run("Stream の consume と handler 結果の公開", func(t *testing.T) {
		t.Run("handler が nil を返すとき、handled に nil が流れる", func(t *testing.T) {
			broker := apimatchmakingfake.NewBroker()
			pub := apimatchmakingfake.NewPublisher(broker)
			stream := apimatchmakingfake.NewStream(apimatchmakingfake.NewSubscriber(broker), "t")

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() { _ = stream.Consume(ctx, func(_ context.Context, _ []byte) error { return nil }) }()

			require.NoError(t, pub.Publish(ctx, "t", []byte(`x`)))

			got := stream.ExpectHandled(t, time.Second)
			assert.NoError(t, got)
		})

		t.Run("handler が error を返すとき、handled に同じ error が流れる", func(t *testing.T) {
			broker := apimatchmakingfake.NewBroker()
			pub := apimatchmakingfake.NewPublisher(broker)
			stream := apimatchmakingfake.NewStream(apimatchmakingfake.NewSubscriber(broker), "t")

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				_ = stream.Consume(ctx, func(_ context.Context, _ []byte) error { return errors.New("boom") })
			}()

			require.NoError(t, pub.Publish(ctx, "t", []byte(`x`)))

			got := stream.ExpectHandled(t, time.Second)
			assert.EqualError(t, got, "boom")
		})
	})
}

func TestMatchMade(t *testing.T) {
	t.Run("MatchMade typed helper", func(t *testing.T) {
		t.Run("Expect → Publish → Wait すると、typed publish と typed 受信が一致する", func(t *testing.T) {
			broker := apimatchmakingfake.NewBroker()
			pub := apimatchmakingfake.NewPublisher(broker)
			sub := apimatchmakingfake.NewSubscriber(broker)
			ctx := context.Background()

			exp := apimatchmakingfake.ExpectMatchMade(sub)

			require.NoError(t, apimatchmakingfake.PublishMatchMade(ctx, pub, apimatchmaking.MatchMadeEvent{
				Players: []apimatchmaking.MatchedPlayer{
					{PlayerID: "p-1", DeckID: 10},
					{PlayerID: "p-2", DeckID: 20},
				},
			}))

			got, err := exp.Wait(time.Second)
			require.NoError(t, err)
			assert.Equal(t, apimatchmaking.EventTypeMatchMade, got.EventType, "EventType は契約で固定")
			assert.NotEmpty(t, got.MatchID, "MatchID は未指定なら自動生成される")
			require.Len(t, got.Players, 2)
			assert.Equal(t, "p-1", got.Players[0].PlayerID)
			assert.Equal(t, int64(10), got.Players[0].DeckID)
			assert.Equal(t, "p-2", got.Players[1].PlayerID)
		})

		t.Run("Expect より先に Publish したとき、Wait が timeout する", func(t *testing.T) {
			broker := apimatchmakingfake.NewBroker()
			pub := apimatchmakingfake.NewPublisher(broker)
			sub := apimatchmakingfake.NewSubscriber(broker)
			ctx := context.Background()

			require.NoError(t, apimatchmakingfake.PublishMatchMade(ctx, pub, apimatchmaking.MatchMadeEvent{
				Players: []apimatchmaking.MatchedPlayer{{PlayerID: "p-1"}},
			}))

			exp := apimatchmakingfake.ExpectMatchMade(sub)
			_, err := exp.Wait(50 * time.Millisecond)
			require.ErrorContains(t, err, "timeout")
		})
	})
}
