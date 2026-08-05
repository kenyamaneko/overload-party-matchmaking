package secretmanager

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	t.Run("Secret Managerクライアントの生成", func(t *testing.T) {
		t.Run("プロジェクトIDが空のとき、エラーになる", func(t *testing.T) {
			c, err := NewClient(context.Background(), "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "projectID is empty")
			assert.Nil(t, c)
		})
	})
}

func TestAccessLatest(t *testing.T) {
	t.Run("シークレットの取得", func(t *testing.T) {
		t.Run("secret IDが空のとき、エラーになる", func(t *testing.T) {
			c := &Client{projectID: "TST-PROJECT"}
			value, err := c.AccessLatest(context.Background(), "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "secretID is empty")
			assert.Empty(t, value)
		})
	})
}
