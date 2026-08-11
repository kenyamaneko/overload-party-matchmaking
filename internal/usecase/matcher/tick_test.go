package matcher_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher"
	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"
)

func TestMatcherTick(t *testing.T) {
	t.Run("マッチングの成立と通知", func(t *testing.T) {
		t.Run("マッチメイキングキューが空の状態でマッチングの周期処理が動くと、送出されたメッセージは1件も無い", func(t *testing.T) {
			q := newFakeQueueWithPairs(0)
			pub := &fakePublisher{}
			m := matcher.New(q, pub, newOptions(matcher.Options{}))
			startMatcher(t, m)

			assert.Never(t, func() bool { return len(pub.publishedMessages()) > 0 }, 50*time.Millisecond, time.Millisecond)
		})

		t.Run("マッチメイキングキューに2人以上いる状態でマッチングの周期処理が動くと、送出されたメッセージが1件ある", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			pub := &fakePublisher{}
			m := matcher.New(q, pub, newOptions(matcher.Options{}))
			startMatcher(t, m)

			require.Eventually(t, func() bool { return len(pub.publishedMessages()) >= 1 }, time.Second, time.Millisecond)
			assert.Len(t, pub.publishedMessages(), 1)
		})

		t.Run("マッチメイキングキューに2人以上いる状態でマッチングの周期処理が動き送出が成功すると、送出されたメッセージには取り出された両プレイヤーのenqueue時点のプレイヤーID・デッキID・名前・レベルがそれぞれ含まれる", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			wantPlayers := q.entriesSnapshot()
			pub := &fakePublisher{}
			m := matcher.New(q, pub, newOptions(matcher.Options{}))
			startMatcher(t, m)

			require.Eventually(t, func() bool { return len(pub.publishedMessages()) >= 1 }, time.Second, time.Millisecond)

			var wire apimatchmaking.MatchMadeEvent
			require.NoError(t, json.Unmarshal(pub.publishedMessages()[0].payload, &wire))
			require.Len(t, wire.Players, 2)
			for i, want := range wantPlayers {
				assert.Equal(t, want.PlayerID, wire.Players[i].PlayerID)
				assert.Equal(t, want.DeckID, wire.Players[i].DeckID)
				assert.Equal(t, want.Name, wire.Players[i].Name)
				assert.Equal(t, want.Level, wire.Players[i].Level)
			}
		})

		t.Run("マッチメイキングキューからの取り出しがエラーを返す状態でマッチングの周期処理が動くと、送出されたメッセージは1件も無い", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			q.popPairErr = assert.AnError
			pub := &fakePublisher{}
			m := matcher.New(q, pub, newOptions(matcher.Options{}))
			startMatcher(t, m)

			assert.Never(t, func() bool { return len(pub.publishedMessages()) > 0 }, 50*time.Millisecond, time.Millisecond)
		})

		t.Run("送出が失敗する状態でマッチングの周期処理が動くと、取り出した2人は元のFIFO順序のままマッチメイキングキューに戻る", func(t *testing.T) {
			q := newFakeQueueWithPairs(1)
			wantPlayers := q.entriesSnapshot()
			pub := &fakePublisher{err: assert.AnError}
			m := matcher.New(q, pub, newOptions(matcher.Options{}))
			startMatcher(t, m)

			require.Eventually(t, func() bool { return q.size() == 2 }, time.Second, time.Millisecond)
			got := q.entriesSnapshot()
			require.Len(t, got, 2)
			assert.Equal(t, wantPlayers[0].PlayerID, got[0].PlayerID)
			assert.Equal(t, wantPlayers[1].PlayerID, got[1].PlayerID)
		})
	})
}
