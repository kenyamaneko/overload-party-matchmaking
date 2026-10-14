package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// APP_ENV の許容値。
const (
	AppEnvLocal      = "local"
	AppEnvProduction = "production"
)

const (
	maxPort = 65535
	// maxDurationSeconds は time.Duration に変換しても桁あふれしない秒数の上限。
	maxDurationSeconds = int(math.MaxInt64 / int64(time.Second))
	// noUpperBound は上限を設けない整数の環境変数に渡す値。
	noUpperBound = math.MaxInt
)

// Config はマッチメイキングサービスの設定を保持します。
type Config struct {
	AppEnv                string
	Port                  int
	RedisURL              string // APP_ENV=local のときのみセットされる（Valkey 接続用）
	GoogleCloudProjectID  string
	MatchMadeTopic        string
	CircuitThreshold      int           // ConfigMap 経由。負荷試験やインシデント時にコード変更なしで調整できるよう env で持つ
	CircuitCooldown       time.Duration // ConfigMap 経由。負荷試験やインシデント時にコード変更なしで調整できるよう env で持つ
	DrainTimeout          time.Duration // ConfigMap 経由。負荷試験やインシデント時にコード変更なしで調整できるよう env で持つ
	InternalAuthPublicKey string
}

// FromEnv は環境変数から Config を読み込みます。
func FromEnv() (*Config, error) {
	cfg := &Config{AppEnv: os.Getenv("APP_ENV")}

	switch cfg.AppEnv {
	case "":
		return nil, fmt.Errorf("config: missing required env var: APP_ENV (must be %q or %q)", AppEnvLocal, AppEnvProduction)
	case AppEnvLocal, AppEnvProduction:
	default:
		return nil, fmt.Errorf("config: APP_ENV %q is invalid (must be %q or %q)", cfg.AppEnv, AppEnvLocal, AppEnvProduction)
	}

	var problems []string
	appendStringProblem := func(key string) string {
		value, problem := readRequiredString(key)
		if problem != "" {
			problems = append(problems, problem)
		}
		return value
	}
	appendIntProblem := func(key string, upperBound int) int {
		n, problem := readPositiveInt(key, upperBound)
		if problem != "" {
			problems = append(problems, problem)
		}
		return n
	}

	if cfg.AppEnv == AppEnvLocal {
		cfg.RedisURL = appendStringProblem("UPSTASH_REDIS_URL")
	}
	cfg.GoogleCloudProjectID = appendStringProblem("GOOGLE_CLOUD_PROJECT_ID")
	cfg.MatchMadeTopic = appendStringProblem("MATCH_MADE_TOPIC")
	cfg.InternalAuthPublicKey = appendStringProblem("INTERNAL_AUTH_PUBLIC_KEY")

	cfg.Port = appendIntProblem("PORT", maxPort)
	cfg.CircuitThreshold = appendIntProblem("MATCHMAKING_CIRCUIT_THRESHOLD", noUpperBound)
	cfg.CircuitCooldown = time.Duration(appendIntProblem("MATCHMAKING_CIRCUIT_COOLDOWN_SEC", maxDurationSeconds)) * time.Second
	cfg.DrainTimeout = time.Duration(appendIntProblem("MATCHMAKING_DRAIN_TIMEOUT_SEC", maxDurationSeconds)) * time.Second

	if len(problems) > 0 {
		return nil, fmt.Errorf("config: %s", strings.Join(problems, "; "))
	}
	return cfg, nil
}

// readRequiredString は環境変数 key を必須の文字列として読み込みます。
// 未設定、空文字、空白だけの値は problem に理由を返し、value は空文字になります。
func readRequiredString(key string) (value, problem string) {
	raw := os.Getenv(key)
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Sprintf("missing required env var: %s", key)
	}
	return raw, ""
}

// readPositiveInt は環境変数 key を 1 以上 upperBound 以下の整数として読み込みます。
// 読み込めない場合は problem に理由を返し、n は 0 になります。
func readPositiveInt(key string, upperBound int) (n int, problem string) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, fmt.Sprintf("missing required env var: %s", key)
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Sprintf("%s %q: %v", key, raw, err)
	}
	if n <= 0 {
		return 0, fmt.Sprintf("%s must be > 0, got %d", key, n)
	}
	if n > upperBound {
		return 0, fmt.Sprintf("%s must be <= %d, got %d", key, upperBound, n)
	}
	return n, ""
}
