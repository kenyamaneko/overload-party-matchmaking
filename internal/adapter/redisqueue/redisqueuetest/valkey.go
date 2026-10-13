// Package redisqueuetest は redisqueue の結合テストが使う Valkey を testcontainers で起動するヘルパを提供する。
package redisqueuetest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	valkeyImage          = "valkey/valkey:8-alpine"
	valkeyPort           = "6379/tcp"
	valkeyReadyLog       = "Ready to accept connections"
	valkeyStartupTimeout = 60 * time.Second
)

// Container は起動済み Valkey コンテナのハンドル。
type Container struct {
	container testcontainers.Container
	url       string
}

// New は Valkey コンテナを起動し、redis:// 接続 URL を持つハンドルを返す。
func New(ctx context.Context) (*Container, error) {
	req := testcontainers.ContainerRequest{
		Image:        valkeyImage,
		ExposedPorts: []string{valkeyPort},
		WaitingFor:   wait.ForLog(valkeyReadyLog).WithStartupTimeout(valkeyStartupTimeout),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start valkey container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("container host: %w", err), container.Terminate(ctx))
	}
	mapped, err := container.MappedPort(ctx, valkeyPort)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("mapped port: %w", err), container.Terminate(ctx))
	}
	return &Container{
		container: container,
		url:       fmt.Sprintf("redis://%s:%s", host, mapped.Port()),
	}, nil
}

// URL は redis://host:port 形式の Valkey 接続 URL を返す。
func (c *Container) URL() string { return c.url }

// Terminate はコンテナを停止する。
func (c *Container) Terminate(ctx context.Context) error { return c.container.Terminate(ctx) }
