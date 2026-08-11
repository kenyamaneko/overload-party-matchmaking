package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envKeys は FromEnv が読む全 env キー。各テストは毎回これらを明示値（または ""）で
// 上書きし、シェル環境からの漏れで Given が非決定にならないようにする。
var envKeys = []string{
	"APP_ENV",
	"UPSTASH_REDIS_URL",
	"GOOGLE_CLOUD_PROJECT_ID",
	"MATCH_MADE_TOPIC",
	"INTERNAL_AUTH_PUBLIC_KEY",
	"PORT",
	"MATCHMAKING_CIRCUIT_THRESHOLD",
	"MATCHMAKING_CIRCUIT_COOLDOWN_SEC",
	"MATCHMAKING_DRAIN_TIMEOUT_SEC",
}

// testPublicKeyPEM は config が値をそのまま保持することの確認にだけ使うダミー。
// 鍵としての妥当性は検証しないため、PEM の体裁だけ揃えている。
const testPublicKeyPEM = "-----BEGIN PUBLIC KEY-----\ndummy-not-a-real-key\n-----END PUBLIC KEY-----\n"

// setEnv は envKeys を一括で上書きする。envs に無いキーは "" (未設定相当) として
// t.Setenv で適用する。os.Getenv は "" と unset を区別しないため、空文字指定で
// required チェックの欠落経路を発火できる。t.Setenv はテスト終了時に元の値へ復元する。
func setEnv(t *testing.T, envs map[string]string) {
	t.Helper()
	for _, k := range envKeys {
		t.Setenv(k, envs[k])
	}
}

// mergeEnv は複数の env map を後勝ちで統合する。
func mergeEnv(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// validEnv は APP_ENV=local での必須環境変数を全て明示した最小構成。
// 各ケースはこれを baseline に override を重ねる。
var validEnv = map[string]string{
	"APP_ENV":                          "local",
	"UPSTASH_REDIS_URL":                "redis://localhost:6379",
	"GOOGLE_CLOUD_PROJECT_ID":          "test-project",
	"MATCH_MADE_TOPIC":                 "match-made",
	"INTERNAL_AUTH_PUBLIC_KEY":         testPublicKeyPEM,
	"PORT":                             "8080",
	"MATCHMAKING_CIRCUIT_THRESHOLD":    "5",
	"MATCHMAKING_CIRCUIT_COOLDOWN_SEC": "30",
	"MATCHMAKING_DRAIN_TIMEOUT_SEC":    "10",
}

func TestFromEnv(t *testing.T) {
	t.Run("環境変数からの設定読み込み", func(t *testing.T) {
		t.Run("全ての必須環境変数が設定されている状態で設定を読み込むと、エラーにならない", func(t *testing.T) {
			setEnv(t, validEnv)

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.NotNil(t, cfg)
		})

		t.Run("PORTのみが未設定の状態で設定を読み込むと、エラーになる", func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{"PORT": ""}))

			_, err := FromEnv()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing required env var: PORT")
		})

		t.Run("PORTとGOOGLE_CLOUD_PROJECT_IDがともに未設定の状態で設定を読み込むと、エラーには両方の欠落が含まれる", func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{
				"PORT":                    "",
				"GOOGLE_CLOUD_PROJECT_ID": "",
			}))

			_, err := FromEnv()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing required env var: PORT")
			assert.Contains(t, err.Error(), "missing required env var: GOOGLE_CLOUD_PROJECT_ID")
		})
	})
}
