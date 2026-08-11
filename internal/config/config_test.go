package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/config"
)

// setValidEnv は必須環境変数をすべて有効な値に設定する。個々のケースは
// この呼び出し後に対象の変数だけを上書き/空文字化して前提を作る。
func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", config.AppEnvProduction)
	t.Setenv("GOOGLE_CLOUD_PROJECT_ID", "test-project")
	t.Setenv("MATCH_MADE_TOPIC", "test-topic")
	t.Setenv("INTERNAL_AUTH_PUBLIC_KEY", "test-internal-auth-key")
	t.Setenv("PORT", "8080")
	t.Setenv("MATCHMAKING_CIRCUIT_THRESHOLD", "5")
	t.Setenv("MATCHMAKING_CIRCUIT_COOLDOWN_SEC", "30")
	t.Setenv("MATCHMAKING_DRAIN_TIMEOUT_SEC", "10")
	t.Setenv("UPSTASH_REDIS_URL", "")
}

func TestFromEnv(t *testing.T) {
	t.Run("環境変数からの設定読み込み", func(t *testing.T) {
		t.Run("APP_ENVが未設定の状態で設定を読み込むと、errorになる", func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("APP_ENV", "")

			_, err := config.FromEnv()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing required env var: APP_ENV")
		})

		t.Run("APP_ENVがlocalでもproductionでもない値の状態で設定を読み込むと、errorになる", func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("APP_ENV", "staging")

			_, err := config.FromEnv()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "APP_ENV")
			assert.Contains(t, err.Error(), "invalid")
		})

		t.Run("APP_ENVがlocalでUPSTASH_REDIS_URLが未設定の状態で設定を読み込むと、errorになる", func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("APP_ENV", config.AppEnvLocal)
			t.Setenv("UPSTASH_REDIS_URL", "")

			_, err := config.FromEnv()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "UPSTASH_REDIS_URL")
		})

		t.Run("APP_ENVがproductionの状態で設定を読み込むと、UPSTASH_REDIS_URLが未設定でも設定を読み込める", func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("APP_ENV", config.AppEnvProduction)
			t.Setenv("UPSTASH_REDIS_URL", "")

			_, err := config.FromEnv()

			assert.NoError(t, err)
		})

		t.Run("GOOGLE_CLOUD_PROJECT_IDが未設定の状態で設定を読み込むと、errorになる", func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("GOOGLE_CLOUD_PROJECT_ID", "")

			_, err := config.FromEnv()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "GOOGLE_CLOUD_PROJECT_ID")
		})

		t.Run("MATCH_MADE_TOPICが未設定の状態で設定を読み込むと、errorになる", func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("MATCH_MADE_TOPIC", "")

			_, err := config.FromEnv()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "MATCH_MADE_TOPIC")
		})

		t.Run("INTERNAL_AUTH_PUBLIC_KEYが未設定の状態で設定を読み込むと、errorになる", func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("INTERNAL_AUTH_PUBLIC_KEY", "")

			_, err := config.FromEnv()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "INTERNAL_AUTH_PUBLIC_KEY")
		})

		t.Run("正の整数として必須の環境変数が未設定のとき", func(t *testing.T) {
			envKeys := []string{
				"PORT",
				"MATCHMAKING_CIRCUIT_THRESHOLD",
				"MATCHMAKING_CIRCUIT_COOLDOWN_SEC",
				"MATCHMAKING_DRAIN_TIMEOUT_SEC",
			}
			for _, envKey := range envKeys {
				t.Run(envKey+"が未設定の状態で設定を読み込むと、errorになる", func(t *testing.T) {
					setValidEnv(t)
					t.Setenv(envKey, "")

					_, err := config.FromEnv()

					require.Error(t, err)
					assert.Contains(t, err.Error(), envKey)
				})
			}
		})

		t.Run("正の整数として必須の環境変数のいずれかが数値に変換できない値の状態で設定を読み込むと、errorになる", func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("PORT", "not-a-number")

			_, err := config.FromEnv()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "PORT")
			assert.Contains(t, err.Error(), "not-a-number")
		})

		t.Run("正の整数として必須の環境変数のいずれかが0以下の状態で設定を読み込むと、errorになる", func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("PORT", "0")

			_, err := config.FromEnv()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "PORT")
			assert.Contains(t, err.Error(), "must be > 0")
		})

		t.Run("全ての必須環境変数が設定されている状態で設定を読み込むと、MATCHMAKING_CIRCUIT_COOLDOWN_SECに設定した秒数がそのまま秒単位の待ち時間として設定に反映される", func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("MATCHMAKING_CIRCUIT_COOLDOWN_SEC", "42")

			got, err := config.FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 42*time.Second, got.CircuitCooldown)
		})

		t.Run("全ての必須環境変数が設定されている状態で設定を読み込むと、MATCHMAKING_DRAIN_TIMEOUT_SECに設定した秒数がそのまま秒単位の待ち時間として設定に反映される", func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("MATCHMAKING_DRAIN_TIMEOUT_SEC", "17")

			got, err := config.FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 17*time.Second, got.DrainTimeout)
		})

		t.Run("PORTとGOOGLE_CLOUD_PROJECT_IDがともに未設定の状態で設定を読み込むと、errorにはその両方の欠落が含まれる", func(t *testing.T) {
			setValidEnv(t)
			t.Setenv("PORT", "")
			t.Setenv("GOOGLE_CLOUD_PROJECT_ID", "")

			_, err := config.FromEnv()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "PORT")
			assert.Contains(t, err.Error(), "GOOGLE_CLOUD_PROJECT_ID")
		})
	})
}
