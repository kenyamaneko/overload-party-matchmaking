package pubsub

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("Publisher の生成", func(t *testing.T) {
		cases := []struct {
			name           string
			projectID      string
			matchMadeTopic string
			wantSubs       string
		}{
			{
				name:           "projectID が空のとき、projectID is empty エラーになる",
				projectID:      "",
				matchMadeTopic: "match-made",
				wantSubs:       "projectID is empty",
			},
			{
				name:           "matchMadeTopic が空のとき、matchMadeTopic is required エラーになる",
				projectID:      "TST-PROJECT",
				matchMadeTopic: "",
				wantSubs:       "matchMadeTopic is required",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				p, err := New(context.Background(), tc.projectID, tc.matchMadeTopic)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantSubs)
				assert.Nil(t, p)
			})
		}
	})
}

func TestPublish(t *testing.T) {
	t.Run("イベントの配信", func(t *testing.T) {
		t.Run("未登録の種別のイベントを配信しようとしたとき、unknown event type エラーになる", func(t *testing.T) {
			p := &Publisher{}
			err := p.Publish(context.Background(), "unknown-event-type", []byte(`{}`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown event type")
		})
	})
}
