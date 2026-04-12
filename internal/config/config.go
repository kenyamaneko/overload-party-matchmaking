package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config はマッチメイキングサービスの設定を保持します。
type Config struct {
	Port             int
	RedisURL         string
	PubsubProjectID  string
	PubsubTopic      string
	CircuitThreshold int
	CircuitCooldown  time.Duration
	DrainTimeout     time.Duration
}

// FromEnv は環境変数から Config を読み込みます。
func FromEnv() (*Config, error) {
	cfg := &Config{
		RedisURL:        os.Getenv("UPSTASH_REDIS_URL"),
		PubsubProjectID: os.Getenv("PUBSUB_PROJECT_ID"),
		PubsubTopic:     os.Getenv("PUBSUB_TOPIC"),
	}

	var missing []string
	if cfg.RedisURL == "" {
		missing = append(missing, "UPSTASH_REDIS_URL")
	}
	if cfg.PubsubProjectID == "" {
		missing = append(missing, "PUBSUB_PROJECT_ID")
	}
	if cfg.PubsubTopic == "" {
		missing = append(missing, "PUBSUB_TOPIC")
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
