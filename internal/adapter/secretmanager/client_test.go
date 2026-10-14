package secretmanager_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/secretmanager"
)

func TestNewClientConfigValidation(t *testing.T) {
	t.Run("[シークレット取得]取得先の設定検証", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("プロジェクトIDが空文字のとき、シークレット取得の初期化処理を行うと、プロジェクトIDが空であることを示すエラーになる", func(t *testing.T) {
				_, err := secretmanager.NewClient(context.Background(), "")

				require.Error(t, err)
				assert.ErrorContains(t, err, "projectID is empty")
			})
		})
	})
}
