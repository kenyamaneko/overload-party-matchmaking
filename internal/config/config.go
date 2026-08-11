package config

import (
	"fmt"
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
	cfg := &Config{
		AppEnv:                os.Getenv("APP_ENV"),
		GoogleCloudProjectID:  os.Getenv("GOOGLE_CLOUD_PROJECT_ID"),
		MatchMadeTopic:        os.Getenv("MATCH_MADE_TOPIC"),
		InternalAuthPublicKey: os.Getenv("INTERNAL_AUTH_PUBLIC_KEY"),
	}

	switch cfg.AppEnv {
	case "":
		return nil, fmt.Errorf("config: missing required env var: APP_ENV (must be %q or %q)", AppEnvLocal, AppEnvProduction)
	case AppEnvLocal, AppEnvProduction:
	default:
		return nil, fmt.Errorf("config: APP_ENV %q is invalid (must be %q or %q)", cfg.AppEnv, AppEnvLocal, AppEnvProduction)
	}

	var problems []string
	if cfg.AppEnv == AppEnvLocal {
		cfg.RedisURL = os.Getenv("UPSTASH_REDIS_URL")
		if cfg.RedisURL == "" {
			problems = append(problems, "missing required env var: UPSTASH_REDIS_URL")
		}
	}
	if cfg.GoogleCloudProjectID == "" {
		problems = append(problems, "missing required env var: GOOGLE_CLOUD_PROJECT_ID")
	}
	if cfg.MatchMadeTopic == "" {
		problems = append(problems, "missing required env var: MATCH_MADE_TOPIC")
	}
	if cfg.InternalAuthPublicKey == "" {
		problems = append(problems, "missing required env var: INTERNAL_AUTH_PUBLIC_KEY")
	}

	appendPositiveIntProblem := func(key string) int {
		n, problem := parsePositiveInt(key)
		if problem != "" {
			problems = append(problems, problem)
		}
		return n
	}
	cfg.Port = appendPositiveIntProblem("PORT")
	cfg.CircuitThreshold = appendPositiveIntProblem("MATCHMAKING_CIRCUIT_THRESHOLD")
	cfg.CircuitCooldown = time.Duration(appendPositiveIntProblem("MATCHMAKING_CIRCUIT_COOLDOWN_SEC")) * time.Second
	cfg.DrainTimeout = time.Duration(appendPositiveIntProblem("MATCHMAKING_DRAIN_TIMEOUT_SEC")) * time.Second

	if len(problems) > 0 {
		return nil, fmt.Errorf("config: %s", strings.Join(problems, "; "))
	}
	return cfg, nil
}

// parsePositiveInt は環境変数 key を正の整数として読み込みます。
// 読み込めない場合は problem に理由を返し、n は 0 になります。
func parsePositiveInt(key string) (n int, problem string) {
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
	return n, ""
}
