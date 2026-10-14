package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger(t *testing.T, appEnv string) (*slog.Logger, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	logger, err := newLogger(appEnv, &stdout, &stderr)
	require.NoError(t, err)
	return logger, &stdout, &stderr
}

func decodeLogLine(t *testing.T, out *bytes.Buffer) map[string]any {
	t.Helper()
	var line map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &line))
	return line
}

func TestNewLogger(t *testing.T) {
	t.Run("[ログ]APP_ENVごとのログの出力先と形式", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("APP_ENVがproductionのとき", func(t *testing.T) {
				t.Run("Errorレベルでログを出力すると、標準出力のJSONのseverityがERRORになる", func(t *testing.T) {
					logger, stdout, _ := newTestLogger(t, "production")

					logger.Error("hello")

					assert.Equal(t, "ERROR", decodeLogLine(t, stdout)["severity"])
				})

				t.Run("Warnレベルでログを出力すると、標準出力のJSONのseverityがWARNINGになる", func(t *testing.T) {
					logger, stdout, _ := newTestLogger(t, "production")

					logger.Warn("hello")

					assert.Equal(t, "WARNING", decodeLogLine(t, stdout)["severity"])
				})

				t.Run("Infoレベルでログを出力すると、標準出力のJSONのseverityがINFOになる", func(t *testing.T) {
					logger, stdout, _ := newTestLogger(t, "production")

					logger.Info("hello")

					assert.Equal(t, "INFO", decodeLogLine(t, stdout)["severity"])
				})

				t.Run("メッセージhelloでログを出力すると、標準出力のJSONのmessageがhelloになる", func(t *testing.T) {
					logger, stdout, _ := newTestLogger(t, "production")

					logger.Info("hello")

					assert.Equal(t, "hello", decodeLogLine(t, stdout)["message"])
				})

				t.Run("ログを出力すると、標準出力のJSONのserviceがmatchmakingになる", func(t *testing.T) {
					logger, stdout, _ := newTestLogger(t, "production")

					logger.Info("hello")

					assert.Equal(t, "matchmaking", decodeLogLine(t, stdout)["service"])
				})

				t.Run("Debugレベルでメッセージhelloのログを出力すると、標準出力の内容は空のままになる", func(t *testing.T) {
					logger, stdout, _ := newTestLogger(t, "production")

					logger.Debug("hello")

					assert.Empty(t, stdout.String())
				})
			})

			t.Run("APP_ENVがlocalのとき", func(t *testing.T) {
				t.Run("Debugレベルでメッセージhelloのログを出力すると、標準エラー出力にhelloが含まれる", func(t *testing.T) {
					logger, _, stderr := newTestLogger(t, "local")

					logger.Debug("hello")

					assert.Contains(t, stderr.String(), "hello")
				})

				t.Run("ログを出力すると、標準エラー出力の行にserviceの値としてmatchmakingが含まれる", func(t *testing.T) {
					logger, _, stderr := newTestLogger(t, "local")

					logger.Info("hello")

					assert.Contains(t, stderr.String(), "service=matchmaking")
				})
			})
		})
	})
}
