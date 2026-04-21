package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// setAllRequired は APP_ENV=local を前提に全必須 env をセットする。
// production 経路のテストでは UPSTASH_REDIS_URL を落とす。
func setAllRequired(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", AppEnvLocal)
	t.Setenv("UPSTASH_REDIS_URL", "rediss://user:pass@host:6379")
	t.Setenv("GOOGLE_CLOUD_PROJECT_ID", "my-project")
	t.Setenv("PUBSUB_TOPIC", "matchmaking-events")
	t.Setenv("PORT", "9004")
	t.Setenv("MATCHMAKING_CIRCUIT_THRESHOLD", "5")
	t.Setenv("MATCHMAKING_CIRCUIT_COOLDOWN_SEC", "30")
	t.Setenv("MATCHMAKING_DRAIN_TIMEOUT_SEC", "10")
}

// TestFromEnvSucceedsLocal は APP_ENV=local で全必須 env が揃っているとき、
// FromEnv が正しくパースした Config を返すことを検証する (正常系)。
func TestFromEnvSucceedsLocal(t *testing.T) {
	setAllRequired(t)

	cfg, err := FromEnv()
	require.NoError(t, err)
	require.Equal(t, AppEnvLocal, cfg.AppEnv)
	require.Equal(t, 9004, cfg.Port)
	require.Equal(t, "rediss://user:pass@host:6379", cfg.RedisURL)
	require.Equal(t, "my-project", cfg.GoogleCloudProjectID)
	require.Equal(t, "matchmaking-events", cfg.PubsubTopic)
	require.Equal(t, 5, cfg.CircuitThreshold)
	require.Equal(t, 30*time.Second, cfg.CircuitCooldown)
	require.Equal(t, 10*time.Second, cfg.DrainTimeout)
}

// TestFromEnvSucceedsProductionWithoutRedisURL は APP_ENV=production のとき
// UPSTASH_REDIS_URL が未設定でも FromEnv が成功することを検証する
// (production では Secret Manager から実行時取得するため env は不要)。
func TestFromEnvSucceedsProductionWithoutRedisURL(t *testing.T) {
	setAllRequired(t)
	t.Setenv("APP_ENV", AppEnvProduction)
	t.Setenv("UPSTASH_REDIS_URL", "")

	cfg, err := FromEnv()
	require.NoError(t, err)
	require.Equal(t, AppEnvProduction, cfg.AppEnv)
	require.Empty(t, cfg.RedisURL)
}

// TestFromEnvRequiresAppEnv は APP_ENV が空の場合、FromEnv が起動時エラーを返すことを検証する
// (暗黙フォールバック禁止)。
func TestFromEnvRequiresAppEnv(t *testing.T) {
	setAllRequired(t)
	t.Setenv("APP_ENV", "")

	_, err := FromEnv()
	require.Error(t, err)
	require.Contains(t, err.Error(), "APP_ENV")
}

// TestFromEnvRejectsUnknownAppEnv は APP_ENV に local/production 以外の値が入っている場合、
// FromEnv が invalid エラーを返すことを検証する (未知の値の暗黙許容を防ぐ)。
func TestFromEnvRejectsUnknownAppEnv(t *testing.T) {
	setAllRequired(t)
	t.Setenv("APP_ENV", "staging")

	_, err := FromEnv()
	require.Error(t, err)
	require.Contains(t, err.Error(), "APP_ENV")
	require.Contains(t, err.Error(), "invalid")
}

// TestFromEnvLocalRequiresRedisURL は APP_ENV=local の場合のみ UPSTASH_REDIS_URL が必須で、
// 空ならエラーになることを検証する (production とは経路が違うという条件分岐の確認)。
func TestFromEnvLocalRequiresRedisURL(t *testing.T) {
	setAllRequired(t)
	t.Setenv("UPSTASH_REDIS_URL", "")

	_, err := FromEnv()
	require.Error(t, err)
	require.Contains(t, err.Error(), "UPSTASH_REDIS_URL")
}

// TestFromEnvFailsWhenStringVarMissing は必須文字列 env (GOOGLE_CLOUD_PROJECT_ID / PUBSUB_TOPIC) が
// 空の場合、FromEnv がそのキー名を含むエラーを返すことを検証する。
func TestFromEnvFailsWhenStringVarMissing(t *testing.T) {
	for _, key := range []string{"GOOGLE_CLOUD_PROJECT_ID", "PUBSUB_TOPIC"} {
		t.Run(key, func(t *testing.T) {
			setAllRequired(t)
			t.Setenv(key, "")

			_, err := FromEnv()
			require.Error(t, err)
			require.Contains(t, err.Error(), key)
		})
	}
}

// TestFromEnvFailsWhenIntVarMissing は必須整数 env (PORT / CIRCUIT_THRESHOLD / CIRCUIT_COOLDOWN_SEC /
// DRAIN_TIMEOUT_SEC) が空の場合、FromEnv がそのキー名を含むエラーを返すことを検証する。
func TestFromEnvFailsWhenIntVarMissing(t *testing.T) {
	for _, key := range []string{"PORT", "MATCHMAKING_CIRCUIT_THRESHOLD", "MATCHMAKING_CIRCUIT_COOLDOWN_SEC", "MATCHMAKING_DRAIN_TIMEOUT_SEC"} {
		t.Run(key, func(t *testing.T) {
			setAllRequired(t)
			t.Setenv(key, "")

			_, err := FromEnv()
			require.Error(t, err)
			require.Contains(t, err.Error(), key)
		})
	}
}

// TestFromEnvRejectsNonPositiveInt は必須整数 env に 0 が指定された場合、
// FromEnv が "must be > 0" エラーを返すことを検証する (0 を暗黙の無効化として扱わせない)。
func TestFromEnvRejectsNonPositiveInt(t *testing.T) {
	for _, key := range []string{"PORT", "MATCHMAKING_CIRCUIT_THRESHOLD", "MATCHMAKING_CIRCUIT_COOLDOWN_SEC", "MATCHMAKING_DRAIN_TIMEOUT_SEC"} {
		t.Run(key, func(t *testing.T) {
			setAllRequired(t)
			t.Setenv(key, "0")

			_, err := FromEnv()
			require.Error(t, err)
			require.Contains(t, err.Error(), "must be > 0")
		})
	}
}
