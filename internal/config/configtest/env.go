// Package configtest は設定の読み込みのテストで環境変数を用意する部品を提供する。
package configtest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// Setting は環境変数一つの状態 (未設定・空文字・値) を、テスト終了時に元へ戻る形で作る。
type Setting func(t *testing.T, name string)

// Unset は環境変数を未設定にする Setting。
func Unset(t *testing.T, name string) {
	t.Setenv(name, "")
	require.NoError(t, os.Unsetenv(name))
}

// SetValue は環境変数に value を設定する Setting を返す。空文字も設定できる。
func SetValue(value string) Setting {
	return func(t *testing.T, name string) {
		t.Setenv(name, value)
	}
}

var validEnv = map[string]string{
	"APP_ENV":                          "local",
	"UPSTASH_REDIS_URL":                "redis://dummy-valkey:6379",
	"GOOGLE_CLOUD_PROJECT_ID":          "dummy-project",
	"MATCH_MADE_TOPIC":                 "dummy-topic",
	"INTERNAL_AUTH_PUBLIC_KEY":         "dummy-public-key",
	"PORT":                             "8080",
	"MATCHMAKING_CIRCUIT_THRESHOLD":    "3",
	"MATCHMAKING_CIRCUIT_COOLDOWN_SEC": "30",
	"MATCHMAKING_DRAIN_TIMEOUT_SEC":    "15",
}

// SetEnv は読み込み対象の環境変数をすべて受け入れられる値にしたうえで、overrides の環境変数だけを指定の状態にする。
func SetEnv(t *testing.T, overrides map[string]Setting) {
	t.Helper()
	for name, value := range validEnv {
		t.Setenv(name, value)
	}
	for name, set := range overrides {
		set(t, name)
	}
}
