package matcher_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher/matchertest"
)

func positiveOptions() matcher.Options {
	return matcher.Options{
		Interval:         time.Second,
		CircuitThreshold: 3,
		CircuitCooldown:  30 * time.Second,
		DrainTimeout:     5 * time.Second,
	}
}

func createMatcher(opts matcher.Options) (*matcher.Matcher, error) {
	clock := matchertest.NewClock(scenarioStartedAt)
	return matcher.New(matchertest.NewQueue(), matchertest.NewPublisher(), clock, matchertest.NewFixedMatchIDGenerator("mch_TST0000000000000000000000"), opts)
}

func TestMatcherOptions(t *testing.T) {
	t.Run("[マッチ成立]設定値の検証", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("ほかの設定が正の値のとき、定期処理の繰り返しを作成すると", func(t *testing.T) {
				tests := []struct {
					name   string
					modify func(opts *matcher.Options)
				}{
					{"定期処理の実行間隔が1ナノ秒のとき、作成はエラーにならない", func(o *matcher.Options) { o.Interval = time.Nanosecond }},
					{"開く連続失敗回数が1のとき、作成はエラーにならない", func(o *matcher.Options) { o.CircuitThreshold = 1 }},
					{"再試行までの時間が1ナノ秒のとき、作成はエラーにならない", func(o *matcher.Options) { o.CircuitCooldown = time.Nanosecond }},
					{"完了待ちの時間が1ナノ秒のとき、作成はエラーにならない", func(o *matcher.Options) { o.DrainTimeout = time.Nanosecond }},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						opts := positiveOptions()
						tt.modify(&opts)

						_, err := createMatcher(opts)

						assert.NoError(t, err)
					})
				}
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("ほかの設定が正の値のとき、定期処理の繰り返しを作成すると", func(t *testing.T) {
				tests := []struct {
					name        string
					modify      func(opts *matcher.Options)
					wantMessage string
				}{
					{"定期処理の実行間隔が0のとき、作成は、定期処理の実行間隔が0以下であることを示すエラーになる", func(o *matcher.Options) { o.Interval = 0 }, "Interval must be > 0"},
					{"定期処理の実行間隔が-1秒のとき、作成は、定期処理の実行間隔が0以下であることを示すエラーになる", func(o *matcher.Options) { o.Interval = -time.Second }, "Interval must be > 0"},
					{"開く連続失敗回数が0のとき、作成は、開く連続失敗回数が0以下であることを示すエラーになる", func(o *matcher.Options) { o.CircuitThreshold = 0 }, "CircuitThreshold must be > 0"},
					{"開く連続失敗回数が-1のとき、作成は、開く連続失敗回数が0以下であることを示すエラーになる", func(o *matcher.Options) { o.CircuitThreshold = -1 }, "CircuitThreshold must be > 0"},
					{"再試行までの時間が0のとき、作成は、再試行までの時間が0以下であることを示すエラーになる", func(o *matcher.Options) { o.CircuitCooldown = 0 }, "CircuitCooldown must be > 0"},
					{"再試行までの時間が-1秒のとき、作成は、再試行までの時間が0以下であることを示すエラーになる", func(o *matcher.Options) { o.CircuitCooldown = -time.Second }, "CircuitCooldown must be > 0"},
					{"完了待ちの時間が0のとき、作成は、完了待ちの時間が0以下であることを示すエラーになる", func(o *matcher.Options) { o.DrainTimeout = 0 }, "DrainTimeout must be > 0"},
					{"完了待ちの時間が-1秒のとき、作成は、完了待ちの時間が0以下であることを示すエラーになる", func(o *matcher.Options) { o.DrainTimeout = -time.Second }, "DrainTimeout must be > 0"},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						opts := positiveOptions()
						tt.modify(&opts)

						_, err := createMatcher(opts)

						assert.ErrorContains(t, err, tt.wantMessage)
					})
				}
			})
		})
	})
}
