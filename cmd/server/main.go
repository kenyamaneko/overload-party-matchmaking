package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler"
	mmpubsub "github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/redisqueue"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/secretmanager"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/config"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher"
)

// Upstash Redis 認証情報を保管している Secret Manager 上の secret ID。
// infra 側の命名と 1:1 で対応する。インフラ側の rename 時はここも追従する。
const (
	secretIDUpstashEndpoint = "matchmaking-upstash-redis-endpoint"
	secretIDUpstashPassword = "matchmaking-upstash-redis-password"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("matchmaking: %v", err)
	}
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	redisClient, err := newRedisClient(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = redisClient.Close() }()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}

	q := redisqueue.NewRedisQueue(redisClient)

	publisher, err := mmpubsub.New(ctx, cfg.GoogleCloudProjectID, cfg.PubsubTopic)
	if err != nil {
		return err
	}
	defer func() { _ = publisher.Close() }()

	eventBuilder := mmpubsub.NewEventBuilder()

	m := matcher.New(q, publisher, eventBuilder, matcher.Options{
		Interval:         time.Second,
		CircuitThreshold: cfg.CircuitThreshold,
		CircuitCooldown:  cfg.CircuitCooldown,
		DrainTimeout:     cfg.DrainTimeout,
	})

	var matcherWG sync.WaitGroup
	matcherWG.Add(1)
	go func() {
		defer matcherWG.Done()
		m.Run(ctx)
	}()

	h := httphandler.New(q, m)
	r := httphandler.NewRouter(h)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("matchmaking: listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Printf("matchmaking: shutdown requested")
	case err := <-errCh:
		return err
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	matcherWG.Wait()
	return nil
}

// newRedisClient は APP_ENV に応じて Redis クライアントを構築する。
// local: .env 経由で渡された URL をそのまま使う（Valkey、TLS なし）。
// production: Secret Manager から endpoint/password を取得し TLS で Upstash に接続する。
func newRedisClient(ctx context.Context, cfg *config.Config) (*redis.Client, error) {
	switch cfg.AppEnv {
	case config.AppEnvLocal:
		opt, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return nil, fmt.Errorf("parse redis url: %w", err)
		}
		return redis.NewClient(opt), nil

	case config.AppEnvProduction:
		sm, err := secretmanager.NewClient(ctx, cfg.GoogleCloudProjectID)
		if err != nil {
			return nil, err
		}
		defer func() { _ = sm.Close() }()

		endpoint, err := sm.AccessLatest(ctx, secretIDUpstashEndpoint)
		if err != nil {
			return nil, err
		}
		password, err := sm.AccessLatest(ctx, secretIDUpstashPassword)
		if err != nil {
			return nil, err
		}
		return redis.NewClient(&redis.Options{
			Addr:      endpoint,
			Password:  password,
			TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		}), nil

	default:
		return nil, fmt.Errorf("unsupported APP_ENV: %q", cfg.AppEnv)
	}
}
