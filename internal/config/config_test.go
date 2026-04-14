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

// production 経路では UPSTASH_REDIS_URL は不要（Secret Manager から取得する）。
func TestFromEnvSucceedsProductionWithoutRedisURL(t *testing.T) {
	setAllRequired(t)
	t.Setenv("APP_ENV", AppEnvProduction)
	t.Setenv("UPSTASH_REDIS_URL", "")

	cfg, err := FromEnv()
	require.NoError(t, err)
	require.Equal(t, AppEnvProduction, cfg.AppEnv)
	require.Empty(t, cfg.RedisURL)
}

func TestFromEnvRequiresAppEnv(t *testing.T) {
	setAllRequired(t)
	t.Setenv("APP_ENV", "")

	_, err := FromEnv()
	require.Error(t, err)
	require.Contains(t, err.Error(), "APP_ENV")
}

func TestFromEnvRejectsUnknownAppEnv(t *testing.T) {
	setAllRequired(t)
	t.Setenv("APP_ENV", "staging")

	_, err := FromEnv()
	require.Error(t, err)
	require.Contains(t, err.Error(), "APP_ENV")
	require.Contains(t, err.Error(), "invalid")
}

func TestFromEnvLocalRequiresRedisURL(t *testing.T) {
	setAllRequired(t)
	t.Setenv("UPSTASH_REDIS_URL", "")

	_, err := FromEnv()
	require.Error(t, err)
	require.Contains(t, err.Error(), "UPSTASH_REDIS_URL")
}

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
