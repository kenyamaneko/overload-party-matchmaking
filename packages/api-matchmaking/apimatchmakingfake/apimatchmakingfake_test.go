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

// Publisher に投げた payload は同一 broker の Subscriber に届き、topic 別 isolation が効く。
func TestBroker_DeliversAndIsolates(t *testing.T) {
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
}

// Publisher.Published() は publish した全メッセージを発行順に snapshot で返す。
func TestPublisher_PublishedSnapshot(t *testing.T) {
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
}

// Stream.Consume は publish されたメッセージを handler に渡し、handled に戻り値を流す。
func TestStream_ConsumesAndExposesHandlerResult(t *testing.T) {
	tests := []struct {
		name        string
		handlerFn   func(ctx context.Context, data []byte) error
		wantHandled error
	}{
		{
			name:        "handler nil 戻り",
			handlerFn:   func(_ context.Context, _ []byte) error { return nil },
			wantHandled: nil,
		},
		{
			name:        "handler error 戻り",
			handlerFn:   func(_ context.Context, _ []byte) error { return errors.New("boom") },
			wantHandled: errors.New("boom"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker := apimatchmakingfake.NewBroker()
			pub := apimatchmakingfake.NewPublisher(broker)
			stream := apimatchmakingfake.NewStream(apimatchmakingfake.NewSubscriber(broker), "t")

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() { _ = stream.Consume(ctx, tt.handlerFn) }()

			require.NoError(t, pub.Publish(ctx, "t", []byte(`x`)))

			got := stream.ExpectHandled(t, time.Second)
			if tt.wantHandled == nil {
				assert.NoError(t, got)
			} else {
				assert.EqualError(t, got, tt.wantHandled.Error())
			}
		})
	}
}

// Expect → Publish → Wait の round-trip で typed publish と typed 受信が矛盾なく繋がる。
func TestMatchMade_ExpectPublishWaitRoundTrip(t *testing.T) {
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
}

// ExpectMatchMade で subscribe していない状態 (publish 先行) では Wait が timeout を返す。
func TestMatchMade_WaitTimesOutWhenPublishedBeforeExpect(t *testing.T) {
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
}
