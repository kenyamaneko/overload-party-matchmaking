//go:build integration

package httphandlertest

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/redisqueue"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/redisqueue/redisqueuetest"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/abandon"
)

var valkeyClient *redis.Client

// RunWithValkey は Valkey を起動して m.Run を実行し、終了コードを返す。起動できないときは失敗の終了コードを返す。
func RunWithValkey(m *testing.M) int {
	ctx := context.Background()
	container, err := redisqueuetest.New(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "valkey を起動できない:", err)
		return 1
	}
	defer func() { _ = container.Terminate(ctx) }()

	options, err := redis.ParseURL(container.URL())
	if err != nil {
		fmt.Fprintln(os.Stderr, "valkey の接続 URL を読み取れない:", err)
		return 1
	}
	valkeyClient = redis.NewClient(options)
	defer func() { _ = valkeyClient.Close() }()
	return m.Run()
}

// NewServerWithRealQueue は Valkey を空にし、実際のキューとマッチの破棄をつないだルーターを返す。サーキットブレーカーは閉じている。
func NewServerWithRealQueue(t *testing.T) http.Handler {
	t.Helper()
	require.NoError(t, valkeyClient.FlushAll(context.Background()).Err())
	queue := redisqueue.NewRedisQueue(valkeyClient)
	return NewServer(queue, &FakeCircuit{}, abandon.New(queue))
}
