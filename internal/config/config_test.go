package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// setAllRequired は APP_ENV=local を前提に全必須 env をセットする。
func setAllRequired(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", AppEnvLocal)
	t.Setenv("UPSTASH_REDIS_URL", "rediss://user:pass@host:6379")
	t.Setenv("GOOGLE_CLOUD_PROJECT_ID", "my-project")
	t.Setenv("MATCH_MADE_TOPIC", "matchmaking-events")
	t.Setenv("PORT", "9004")
	t.Setenv("MATCHMAKING_CIRCUIT_THRESHOLD", "5")
	t.Setenv("MATCHMAKING_CIRCUIT_COOLDOWN_SEC", "30")
	t.Setenv("MATCHMAKING_DRAIN_TIMEOUT_SEC", "10")
	t.Setenv("INTERNAL_AUTH_SECRET", "ci-test-secret-do-not-use-in-prod")
}

func TestFromEnv(t *testing.T) {
	t.Run("環境変数からの Config 構築", func(t *testing.T) {
		t.Run("APP_ENV=local で全必須 env が揃うとき、各フィールドがパースされる", func(t *testing.T) {
			setAllRequired(t)

			cfg, err := FromEnv()
			require.NoError(t, err)
			require.Equal(t, AppEnvLocal, cfg.AppEnv)
			require.Equal(t, 9004, cfg.Port)
			require.Equal(t, "rediss://user:pass@host:6379", cfg.RedisURL)
			require.Equal(t, "my-project", cfg.GoogleCloudProjectID)
			require.Equal(t, "matchmaking-events", cfg.MatchMadeTopic)
			require.Equal(t, 5, cfg.CircuitThreshold)
			require.Equal(t, 30*time.Second, cfg.CircuitCooldown)
			require.Equal(t, 10*time.Second, cfg.DrainTimeout)
		})

		t.Run("APP_ENV=production で UPSTASH_REDIS_URL が空でも、Config が構築され RedisURL は空になる", func(t *testing.T) {
			// production では Redis 接続情報を Secret Manager 経由で実行時取得するため env は不要。
			setAllRequired(t)
			t.Setenv("APP_ENV", AppEnvProduction)
			t.Setenv("UPSTASH_REDIS_URL", "")

			cfg, err := FromEnv()
			require.NoError(t, err)
			require.Equal(t, AppEnvProduction, cfg.AppEnv)
			require.Empty(t, cfg.RedisURL)
		})

		t.Run("PORT が下限の 1 のとき、Config が構築され Port は 1 になる", func(t *testing.T) {
			setAllRequired(t)
			t.Setenv("PORT", "1")

			cfg, err := FromEnv()
			require.NoError(t, err)
			require.Equal(t, 1, cfg.Port)
		})

		// 欠落・不正な env はデフォルト値へフォールバックせず、原因のキーや理由を含むエラーにする。
		invalidCases := []struct {
			name         string
			mutate       func(t *testing.T)
			wantContains []string
		}{
			{
				name:         "APP_ENV が空のとき、エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("APP_ENV", "") },
				wantContains: []string{"APP_ENV"},
			},
			{
				name:         "APP_ENV が staging (未知値) のとき、invalid エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("APP_ENV", "staging") },
				wantContains: []string{"APP_ENV", "invalid"},
			},
			{
				name:         "APP_ENV=local で UPSTASH_REDIS_URL が空のとき、エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("UPSTASH_REDIS_URL", "") },
				wantContains: []string{"UPSTASH_REDIS_URL"},
			},
			{
				name:         "GOOGLE_CLOUD_PROJECT_ID が空のとき、エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("GOOGLE_CLOUD_PROJECT_ID", "") },
				wantContains: []string{"GOOGLE_CLOUD_PROJECT_ID"},
			},
			{
				name:         "MATCH_MADE_TOPIC が空のとき、エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("MATCH_MADE_TOPIC", "") },
				wantContains: []string{"MATCH_MADE_TOPIC"},
			},
			{
				name:         "INTERNAL_AUTH_SECRET が空のとき、エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("INTERNAL_AUTH_SECRET", "") },
				wantContains: []string{"INTERNAL_AUTH_SECRET"},
			},
			{
				name:         "PORT が空のとき、エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("PORT", "") },
				wantContains: []string{"PORT"},
			},
			{
				name:         "PORT が非数値 abc のとき、原因の値を含むエラーになる",
				mutate:       func(t *testing.T) { t.Setenv("PORT", "abc") },
				wantContains: []string{"PORT", "abc"},
			},
			{
				name:         "PORT が負数 -5 のとき、must be > 0 エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("PORT", "-5") },
				wantContains: []string{"must be > 0"},
			},
			{
				name:         "MATCHMAKING_CIRCUIT_THRESHOLD が空のとき、エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("MATCHMAKING_CIRCUIT_THRESHOLD", "") },
				wantContains: []string{"MATCHMAKING_CIRCUIT_THRESHOLD"},
			},
			{
				name:         "MATCHMAKING_CIRCUIT_COOLDOWN_SEC が空のとき、エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("MATCHMAKING_CIRCUIT_COOLDOWN_SEC", "") },
				wantContains: []string{"MATCHMAKING_CIRCUIT_COOLDOWN_SEC"},
			},
			{
				name:         "MATCHMAKING_DRAIN_TIMEOUT_SEC が空のとき、エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("MATCHMAKING_DRAIN_TIMEOUT_SEC", "") },
				wantContains: []string{"MATCHMAKING_DRAIN_TIMEOUT_SEC"},
			},
			// 0 は暗黙の無効化として扱わず、正の値を必須とする。
			{
				name:         "PORT が 0 のとき、must be > 0 エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("PORT", "0") },
				wantContains: []string{"must be > 0"},
			},
			{
				name:         "MATCHMAKING_CIRCUIT_THRESHOLD が 0 のとき、must be > 0 エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("MATCHMAKING_CIRCUIT_THRESHOLD", "0") },
				wantContains: []string{"must be > 0"},
			},
			{
				name:         "MATCHMAKING_CIRCUIT_COOLDOWN_SEC が 0 のとき、must be > 0 エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("MATCHMAKING_CIRCUIT_COOLDOWN_SEC", "0") },
				wantContains: []string{"must be > 0"},
			},
			{
				name:         "MATCHMAKING_DRAIN_TIMEOUT_SEC が 0 のとき、must be > 0 エラーになる",
				mutate:       func(t *testing.T) { t.Setenv("MATCHMAKING_DRAIN_TIMEOUT_SEC", "0") },
				wantContains: []string{"must be > 0"},
			},
		}
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				setAllRequired(t)
				tc.mutate(t)

				_, err := FromEnv()
				require.Error(t, err)
				for _, want := range tc.wantContains {
					require.Contains(t, err.Error(), want)
				}
			})
		}
	})
}
