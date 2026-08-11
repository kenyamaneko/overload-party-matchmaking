//go:build integration

package pubsub_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/pubsub/pubsubtest"
)

const testProjectID = "matchmaking-test"

var sharedEmulator *pubsubtest.Emulator

func TestMain(m *testing.M) {
	ctx := context.Background()

	emulator, err := pubsubtest.StartEmulator(ctx, testProjectID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start pubsub emulator:", err)
		os.Exit(1)
	}
	sharedEmulator = emulator

	code := m.Run()

	_ = emulator.Close(ctx)
	os.Exit(code)
}
