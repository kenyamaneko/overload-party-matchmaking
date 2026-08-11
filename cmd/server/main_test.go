package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/config"
)

func TestNewMatcherOptions(t *testing.T) {
	t.Run("設定値の起動時伝播", func(t *testing.T) {
		t.Run("連続失敗の閾値に設定した値を渡すと、起動時の設定にも同じ値が入る", func(t *testing.T) {
			cfg := &config.Config{CircuitThreshold: 7}

			got := newMatcherOptions(cfg)

			assert.Equal(t, 7, got.CircuitThreshold)
		})

		t.Run("再開までの待ち時間(クールダウン)に設定した値を渡すと、起動時の設定にも同じ値が入る", func(t *testing.T) {
			cfg := &config.Config{CircuitCooldown: 45 * time.Second}

			got := newMatcherOptions(cfg)

			assert.Equal(t, 45*time.Second, got.CircuitCooldown)
		})

		t.Run("停止時の待ち時間(ドレインタイムアウト)に設定した値を渡すと、起動時の設定にも同じ値が入る", func(t *testing.T) {
			cfg := &config.Config{DrainTimeout: 12 * time.Second}

			got := newMatcherOptions(cfg)

			assert.Equal(t, 12*time.Second, got.DrainTimeout)
		})
	})
}
