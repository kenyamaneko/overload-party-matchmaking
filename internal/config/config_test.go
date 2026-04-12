package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func setAllRequired(t *testing.T) {
	t.Helper()
	t.Setenv("UPSTASH_REDIS_URL", "rediss://user:pass@host:6379")
	t.Setenv("PUBSUB_PROJECT_ID", "my-project")
	t.Setenv("PUBSUB_TOPIC", "matchmaking-events")
	t.Setenv("PORT", "9004")
	t.Setenv("MATCHMAKING_CIRCUIT_THRESHOLD", "5")
	t.Setenv("MATCHMAKING_CIRCUIT_COOLDOWN_SEC", "30")
	t.Setenv("MATCHMAKING_DRAIN_TIMEOUT_SEC", "10")
}

func TestFromEnvSucceeds(t *testing.T) {
	setAllRequired(t)

	cfg, err := FromEnv()
	require.NoError(t, err)
	require.Equal(t, 9004, cfg.Port)
	require.Equal(t, "rediss://user:pass@host:6379", cfg.RedisURL)
	require.Equal(t, "my-project", cfg.PubsubProjectID)
	require.Equal(t, "matchmaking-events", cfg.PubsubTopic)
	require.Equal(t, 5, cfg.CircuitThreshold)
	require.Equal(t, 30*time.Second, cfg.CircuitCooldown)
	require.Equal(t, 10*time.Second, cfg.DrainTimeout)
}

func TestFromEnvFailsWhenStringVarMissing(t *testing.T) {
	for _, key := range []string{"UPSTASH_REDIS_URL", "PUBSUB_PROJECT_ID", "PUBSUB_TOPIC"} {
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
