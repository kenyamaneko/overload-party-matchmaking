package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
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
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/matchid"
	mmpubsub "github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/redisqueue"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/secretmanager"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/systemclock"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/config"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/abandon"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher"
)

// Upstash Redis 認証情報を保管している Secret Manager 上の secret ID。
const (
	secretIDUpstashEndpoint = "matchmaking-upstash-redis-endpoint"
	secretIDUpstashPassword = "matchmaking-upstash-redis-password"
)

const (
	// matcherInterval は定期処理の実行間隔。
	matcherInterval = time.Second
	// publishTimeout は match_made の送出が応答を待つ時間の上限。応答が返らない送出が定期処理を止め続けないよう設ける。
	publishTimeout = 5 * time.Second
	// readHeaderTimeout は HTTP リクエストのヘッダを読み切る時間の上限。
	readHeaderTimeout = 10 * time.Second
	// httpShutdownTimeout は HTTP サーバーの停止を待つ時間の上限。
	httpShutdownTimeout = 15 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("matchmaking fatal", "error", err)
		os.Exit(1)
	}
}

// newLogger は APP_ENV に応じたロガーを生成する。production は stdout に Cloud Logging 形式の JSON を、
// local は stderr に text 形式を出力する。
func newLogger(appEnv string, stdout, stderr io.Writer) (*slog.Logger, error) {
	switch appEnv {
	case config.AppEnvProduction:
		return slog.New(newCloudLoggingHandler(stdout)).With("service", "matchmaking"), nil
	case config.AppEnvLocal:
		h := slog.NewTextHandler(stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		return slog.New(h).With("service", "matchmaking"), nil
	default:
		return nil, fmt.Errorf("unexpected APP_ENV: %s", appEnv)
	}
}

// newCloudLoggingHandler は Cloud Logging に適合するログハンドラを生成する。
func newCloudLoggingHandler(w io.Writer) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				// 利用者が付けた "level" キーの属性にも呼ばれるため、slog.Level でない値はそのまま返す。
				level, ok := a.Value.Any().(slog.Level)
				if !ok {
					return a
				}
				a.Key = "severity"
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

	logger, err := newLogger(cfg.AppEnv, os.Stdout, os.Stderr)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)

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

	publisher, err := mmpubsub.New(ctx, cfg.GoogleCloudProjectID, cfg.MatchMadeTopic, publishTimeout)
	if err != nil {
		return err
	}
	defer func() { _ = publisher.Close() }()

	clock := systemclock.New()
	m, err := matcher.New(q, publisher, clock, matchid.NewULIDGenerator(clock), newMatcherOptions(cfg))
	if err != nil {
		return err
	}

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

	h := httphandler.New(q, m, abandon.New(q))
	r := httphandler.NewRouter(h, authVerifier)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           r,
		ReadHeaderTimeout: readHeaderTimeout,
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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	// publisher は defer で run() 終了時に close する。tick が publish 中のまま閉じないよう、
	// close (関数末尾の defer) より前にここで matcher の停止を待つ。
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

// newMatcherOptions は Config から matcher.Options を組み立てる。
func newMatcherOptions(cfg *config.Config) matcher.Options {
	return matcher.Options{
		Interval:         matcherInterval,
		CircuitThreshold: cfg.CircuitThreshold,
		CircuitCooldown:  cfg.CircuitCooldown,
		DrainTimeout:     cfg.DrainTimeout,
	}
}
