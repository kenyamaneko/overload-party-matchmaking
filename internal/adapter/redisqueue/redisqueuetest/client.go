package redisqueuetest

import (
	"context"
	"net"
	"sync/atomic"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

const loopbackHost = "127.0.0.1"

// Start は Valkey を起動し、それに接続するクライアントを返す。テスト終了時に両方を片付ける。
// 起動できないときはテストを失敗させる。
func Start(t testing.TB) *redis.Client {
	t.Helper()
	container, err := New(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })

	options, err := redis.ParseURL(container.URL())
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

// Flush はクライアントが指すストアの全データを消す。
func Flush(t testing.TB, client *redis.Client) {
	t.Helper()
	require.NoError(t, client.FlushDB(context.Background()).Err())
}

// UnreachableStore は、何も起動していない場所を指すクライアントと、その場所に Valkey を起動する操作を束ねる。
type UnreachableStore struct {
	// Client は、StartStore を呼ぶまで接続に失敗するクライアント。
	Client *redis.Client

	t      testing.TB
	target atomic.Value
}

// NewUnreachableStore は、何も起動していない場所を指すクライアントを持つ UnreachableStore を返す。
func NewUnreachableStore(t testing.TB) *UnreachableStore {
	t.Helper()
	listener, err := net.Listen("tcp", net.JoinHostPort(loopbackHost, "0"))
	require.NoError(t, err)
	closedAddr := listener.Addr().String()
	require.NoError(t, listener.Close())

	store := &UnreachableStore{t: t}
	store.target.Store(closedAddr)
	dialer := &net.Dialer{}
	store.Client = redis.NewClient(&redis.Options{
		Addr: closedAddr,
		Dialer: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, store.target.Load().(string))
		},
	})
	t.Cleanup(func() { require.NoError(t, store.Client.Close()) })
	return store
}

// StartStore はクライアントが指す場所に Valkey を起動する。テスト終了時に片付ける。
func (s *UnreachableStore) StartStore() {
	s.t.Helper()
	container, err := New(context.Background())
	require.NoError(s.t, err)
	s.t.Cleanup(func() { require.NoError(s.t, container.Terminate(context.Background())) })

	options, err := redis.ParseURL(container.URL())
	require.NoError(s.t, err)
	s.target.Store(options.Addr)
}
