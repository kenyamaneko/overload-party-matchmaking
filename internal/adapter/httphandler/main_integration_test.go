//go:build integration

package httphandler_test

import (
	"os"
	"testing"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler/httphandlertest"
)

func TestMain(m *testing.M) {
	os.Exit(httphandlertest.RunWithValkey(m))
}
