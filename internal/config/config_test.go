package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", AppEnvLocal)
	t.Setenv("UPSTASH_REDIS_URL", "redis://localhost:6379")
	t.Setenv("GOOGLE_CLOUD_PROJECT_ID", "test-project")
	t.Setenv("MATCH_MADE_TOPIC", "match-made")
	t.Setenv("INTERNAL_AUTH_PUBLIC_KEY", "test-key")
	t.Setenv("PORT", "8080")
	t.Setenv("MATCHMAKING_CIRCUIT_THRESHOLD", "5")
	t.Setenv("MATCHMAKING_CIRCUIT_COOLDOWN_SEC", "30")
	t.Setenv("MATCHMAKING_DRAIN_TIMEOUT_SEC", "10")
}

func TestFromEnv(t *testing.T) {
	t.Run("環境変数からの設定読み込み", func(t *testing.T) {
		t.Run("全ての必須環境変数が設定されているとき、errorにならない", func(t *testing.T) {
			setValidEnv(t)

			_, err := FromEnv()

			assert.NoError(t, err)
		})

		t.Run("PORTが未設定のとき、errorになる", func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("PORT", "")

			_, err := FromEnv()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "PORT")
		})

		t.Run("PORTとGOOGLE_CLOUD_PROJECT_IDがともに未設定のとき、errorにはその両方の欠落が含まれる", func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("PORT", "")
			t.Setenv("GOOGLE_CLOUD_PROJECT_ID", "")

			_, err := FromEnv()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "PORT")
			assert.Contains(t, err.Error(), "GOOGLE_CLOUD_PROJECT_ID")
		})
	})
}
