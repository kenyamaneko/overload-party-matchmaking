//go:build integration

package redisqueue_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/redisqueue"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/valkeytest"
)

var sharedClient *redis.Client

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := valkeytest.New(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start valkey container:", err)
		os.Exit(1)
	}

	opt, err := redis.ParseURL(container.URL())
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse valkey url:", err)
		os.Exit(1)
	}
	sharedClient = redis.NewClient(opt)

	code := m.Run()

	_ = sharedClient.Close()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

// newQueue は前のケースの影響が残らないよう Valkey を空にしてから
// RedisQueue を返す。
func newQueue(t *testing.T) *redisqueue.RedisQueue {
	t.Helper()
	require.NoError(t, sharedClient.FlushDB(context.Background()).Err())
	return redisqueue.NewRedisQueue(sharedClient)
}
