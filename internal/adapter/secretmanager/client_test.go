package secretmanager_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/secretmanager"
)

func TestNewClient(t *testing.T) {
	t.Run("Secret Managerクライアントの生成", func(t *testing.T) {
		t.Run("プロジェクトIDが空文字の状態でクライアントを生成しようとすると、生成に失敗する", func(t *testing.T) {
			_, err := secretmanager.NewClient(context.Background(), "")
			assert.Error(t, err)
		})
	})
}

func TestAccessLatest(t *testing.T) {
	t.Run("Secret Managerからの最新バージョン取得", func(t *testing.T) {
		t.Run("シークレットIDが空文字の状態で最新バージョンの取得を試みると、取得に失敗する", func(t *testing.T) {
			c, err := secretmanager.NewClient(context.Background(), "dummy-project")
			require.NoError(t, err)

			_, err = c.AccessLatest(context.Background(), "")
			assert.Error(t, err)
		})
	})
}
