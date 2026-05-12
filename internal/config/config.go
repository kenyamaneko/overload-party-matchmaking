package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// APP_ENV の許容値。
const (
	AppEnvLocal      = "local"
	AppEnvProduction = "production"
)

// Config はマッチメイキングサービスの設定を保持します。
type Config struct {
	AppEnv               string
	Port                 int
	RedisURL             string // APP_ENV=local のときのみセットされる（Valkey 接続用）
	GoogleCloudProjectID string
	MatchMadeTopic       string
	CircuitThreshold     int
	CircuitCooldown      time.Duration
	DrainTimeout         time.Duration
	InternalAuthSecret   string
}

// FromEnv は環境変数から Config を読み込みます。
func FromEnv() (*Config, error) {
	cfg := &Config{
		AppEnv:               os.Getenv("APP_ENV"),
		GoogleCloudProjectID: os.Getenv("GOOGLE_CLOUD_PROJECT_ID"),
		MatchMadeTopic:       os.Getenv("MATCH_MADE_TOPIC"),
		InternalAuthSecret:   os.Getenv("INTERNAL_AUTH_SECRET"),
	}

	switch cfg.AppEnv {
	case "":
		return nil, fmt.Errorf("config: missing required env var: APP_ENV (must be %q or %q)", AppEnvLocal, AppEnvProduction)
	case AppEnvLocal, AppEnvProduction:
	default:
		return nil, fmt.Errorf("config: APP_ENV %q is invalid (must be %q or %q)", cfg.AppEnv, AppEnvLocal, AppEnvProduction)
	}

	var missing []string
	if cfg.AppEnv == AppEnvLocal {
		cfg.RedisURL = os.Getenv("UPSTASH_REDIS_URL")
		if cfg.RedisURL == "" {
			missing = append(missing, "UPSTASH_REDIS_URL")
		}
	}
	if cfg.GoogleCloudProjectID == "" {
		missing = append(missing, "GOOGLE_CLOUD_PROJECT_ID")
	}
	if cfg.MatchMadeTopic == "" {
		missing = append(missing, "MATCH_MADE_TOPIC")
	}
	if cfg.InternalAuthSecret == "" {
		missing = append(missing, "INTERNAL_AUTH_SECRET")
	}

	port, err := requirePositiveInt("PORT")
	if err != nil {
		return nil, err
	}
	cfg.Port = port

	threshold, err := requirePositiveInt("MATCHMAKING_CIRCUIT_THRESHOLD")
	if err != nil {
		return nil, err
	}
	cfg.CircuitThreshold = threshold

	cooldown, err := requirePositiveInt("MATCHMAKING_CIRCUIT_COOLDOWN_SEC")
	if err != nil {
		return nil, err
	}
	cfg.CircuitCooldown = time.Duration(cooldown) * time.Second

	drain, err := requirePositiveInt("MATCHMAKING_DRAIN_TIMEOUT_SEC")
	if err != nil {
		return nil, err
	}
	cfg.DrainTimeout = time.Duration(drain) * time.Second

	if len(missing) > 0 {
		return nil, fmt.Errorf("config: missing required env vars: %v", missing)
	}
	return cfg, nil
}

func requirePositiveInt(key string) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, fmt.Errorf("config: missing required env var: %s", key)
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s %q: %w", key, raw, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("config: %s must be > 0, got %d", key, n)
	}
	return n, nil
}
