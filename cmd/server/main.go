package main

import (
	"context"
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
	"github.com/kenyamaneko/overload-party-matchmaking/internal/config"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher"
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

	redisOpt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("parse redis url: %w", err)
	}
	redisClient := redis.NewClient(redisOpt)
	defer func() { _ = redisClient.Close() }()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}

	q := redisqueue.NewRedisQueue(redisClient)

	publisher, err := mmpubsub.NewPublisher(ctx, cfg.PubsubProjectID, cfg.PubsubTopic)
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
