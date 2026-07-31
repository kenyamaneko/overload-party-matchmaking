package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	internalauth "github.com/kenyamaneko/overload-party-gateway/packages/internalauth-go"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler"
	mmpubsub "github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/redisqueue"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/secretmanager"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/config"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher"
)

// Upstash Redis 認証情報を保管している Secret Manager 上の secret ID。
const (
	secretIDUpstashEndpoint = "matchmaking-upstash-redis-endpoint"
	secretIDUpstashPassword = "matchmaking-upstash-redis-password"
)

func main() {
	if err := run(); err != nil {
		slog.Error("matchmaking fatal", "error", err)
		os.Exit(1)
	}
}

// setupLogger は APP_ENV に応じてアプリケーションのログ出力を設定する。
func setupLogger(appEnv string) error {
	switch appEnv {
	case config.AppEnvProduction:
		slog.SetDefault(slog.New(newCloudLoggingHandler()).With("service", "matchmaking"))
	case config.AppEnvLocal:
		h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		slog.SetDefault(slog.New(h).With("service", "matchmaking"))
	default:
		return fmt.Errorf("unexpected APP_ENV: %s", appEnv)
	}
	return nil
}

// newCloudLoggingHandler は Cloud Logging に適合するログハンドラを生成する。
func newCloudLoggingHandler() slog.Handler {
	return slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				a.Key = "severity"
				if level, ok := a.Value.Any().(slog.Level); ok {
					switch {
					case level >= slog.LevelError:
						a.Value = slog.StringValue("ERROR")
					case level >= slog.LevelWarn:
						a.Value = slog.StringValue("WARNING")
					case level >= slog.LevelInfo:
						a.Value = slog.StringValue("INFO")
					default:
						a.Value = slog.StringValue("DEBUG")
					}
				}
			}
			if a.Key == slog.MessageKey {
				a.Key = "message"
			}
			return a
		},
	})
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	if err := setupLogger(cfg.AppEnv); err != nil {
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

	publisher, err := mmpubsub.New(ctx, cfg.GoogleCloudProjectID, cfg.MatchMadeTopic)
	if err != nil {
		return err
	}
	defer func() { _ = publisher.Close() }()

	m := matcher.New(q, publisher, matcher.Options{
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

	internalAuthKey, err := internalauth.ParsePublicKeyPEM([]byte(cfg.InternalAuthPublicKey))
	if err != nil {
		return fmt.Errorf("INTERNAL_AUTH_PUBLIC_KEY is invalid: %w", err)
	}
	authVerifier := internalauth.NewVerifier(
		internalauth.StaticPublicKeyResolver(internalAuthKey, internalauth.DefaultKeyID),
	)

	h := httphandler.New(q, m)
	r := httphandler.NewRouter(h, authVerifier)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown requested")
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
