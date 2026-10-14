package config_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/config"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/config/configtest"
)

type settingCase struct {
	name    string
	setting configtest.Setting
}

type settingErrorCase struct {
	name    string
	setting configtest.Setting
	wantErr string
}

func loadConfig(t *testing.T, overrides map[string]configtest.Setting) (*config.Config, error) {
	t.Helper()
	configtest.SetEnv(t, overrides)
	return config.FromEnv()
}

func TestFromEnv(t *testing.T) {
	t.Run("[環境変数]APP_ENVの読み込み", func(t *testing.T) {
		t.Run("他の環境変数がすべて受け入れられる値のとき", func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				t.Run("APP_ENVがlocalのとき、サービスの設定の実行環境がlocalになる", func(t *testing.T) {
					cfg, err := loadConfig(t, map[string]configtest.Setting{"APP_ENV": configtest.SetValue("local")})

					require.NoError(t, err)
					assert.Equal(t, "local", cfg.AppEnv)
				})

				t.Run("APP_ENVがproductionのとき、サービスの設定の実行環境がproductionになる", func(t *testing.T) {
					cfg, err := loadConfig(t, map[string]configtest.Setting{"APP_ENV": configtest.SetValue("production")})

					require.NoError(t, err)
					assert.Equal(t, "production", cfg.AppEnv)
				})
			})

			t.Run("異常系", func(t *testing.T) {
				missingCases := []settingCase{
					{"APP_ENVが未設定のとき、読み込みはAPP_ENVが設定されていないことを伝えるエラーで失敗する", configtest.Unset},
					{"APP_ENVが空文字のとき、読み込みはAPP_ENVが設定されていないことを伝えるエラーで失敗する", configtest.SetValue("")},
				}
				for _, tc := range missingCases {
					t.Run(tc.name, func(t *testing.T) {
						_, err := loadConfig(t, map[string]configtest.Setting{"APP_ENV": tc.setting})

						assert.EqualError(t, err, `config: missing required env var: APP_ENV (must be "local" or "production")`)
					})
				}

				invalidCases := []settingErrorCase{
					{"APP_ENVがstagingのとき、読み込みはAPP_ENVがlocalでもproductionでもないことを伝えるエラーで失敗する", configtest.SetValue("staging"), `config: APP_ENV "staging" is invalid (must be "local" or "production")`},
					{"APP_ENVがLOCALのとき、読み込みはAPP_ENVがlocalでもproductionでもないことを伝えるエラーで失敗する", configtest.SetValue("LOCAL"), `config: APP_ENV "LOCAL" is invalid (must be "local" or "production")`},
				}
				for _, tc := range invalidCases {
					t.Run(tc.name, func(t *testing.T) {
						_, err := loadConfig(t, map[string]configtest.Setting{"APP_ENV": tc.setting})

						assert.EqualError(t, err, tc.wantErr)
					})
				}
			})
		})
	})

	t.Run("[環境変数]Valkey接続URLの読み込み", func(t *testing.T) {
		t.Run("他の環境変数がすべて受け入れられる値のとき", func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				t.Run("APP_ENVがlocalでUPSTASH_REDIS_URLがredis://dummy-valkey:6379のとき、サービスの設定のValkey接続URLがredis://dummy-valkey:6379になる", func(t *testing.T) {
					cfg, err := loadConfig(t, map[string]configtest.Setting{
						"APP_ENV":           configtest.SetValue("local"),
						"UPSTASH_REDIS_URL": configtest.SetValue("redis://dummy-valkey:6379"),
					})

					require.NoError(t, err)
					assert.Equal(t, "redis://dummy-valkey:6379", cfg.RedisURL)
				})

				t.Run("APP_ENVがproductionのとき", func(t *testing.T) {
					cases := []settingCase{
						{"UPSTASH_REDIS_URLが未設定のとき、サービスの設定のValkey接続URLが空文字になる", configtest.Unset},
						{"UPSTASH_REDIS_URLが空文字のとき、サービスの設定のValkey接続URLが空文字になる", configtest.SetValue("")},
						{"UPSTASH_REDIS_URLがredis://dummy-valkey:6379のとき、サービスの設定のValkey接続URLが空文字になる", configtest.SetValue("redis://dummy-valkey:6379")},
					}
					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							cfg, err := loadConfig(t, map[string]configtest.Setting{
								"APP_ENV":           configtest.SetValue("production"),
								"UPSTASH_REDIS_URL": tc.setting,
							})

							require.NoError(t, err)
							assert.Equal(t, "", cfg.RedisURL)
						})
					}
				})
			})

			t.Run("異常系", func(t *testing.T) {
				t.Run("APP_ENVがlocalのとき", func(t *testing.T) {
					cases := []settingCase{
						{"UPSTASH_REDIS_URLが未設定のとき、読み込みはUPSTASH_REDIS_URLが設定されていないことを伝えるエラーで失敗する", configtest.Unset},
						{"UPSTASH_REDIS_URLが空文字のとき、読み込みはUPSTASH_REDIS_URLが設定されていないことを伝えるエラーで失敗する", configtest.SetValue("")},
						{"UPSTASH_REDIS_URLが空白だけのとき、読み込みはUPSTASH_REDIS_URLが設定されていないことを伝えるエラーで失敗する", configtest.SetValue(" ")},
					}
					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							_, err := loadConfig(t, map[string]configtest.Setting{
								"APP_ENV":           configtest.SetValue("local"),
								"UPSTASH_REDIS_URL": tc.setting,
							})

							assert.EqualError(t, err, "config: missing required env var: UPSTASH_REDIS_URL")
						})
					}
				})
			})
		})
	})

	t.Run("[環境変数]GoogleCloudプロジェクトIDの読み込み", func(t *testing.T) {
		t.Run("他の環境変数がすべて受け入れられる値のとき", func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				t.Run("GOOGLE_CLOUD_PROJECT_IDがdummy-projectのとき、サービスの設定のGoogleCloudプロジェクトIDがdummy-projectになる", func(t *testing.T) {
					cfg, err := loadConfig(t, map[string]configtest.Setting{"GOOGLE_CLOUD_PROJECT_ID": configtest.SetValue("dummy-project")})

					require.NoError(t, err)
					assert.Equal(t, "dummy-project", cfg.GoogleCloudProjectID)
				})
			})

			t.Run("異常系", func(t *testing.T) {
				cases := []settingCase{
					{"GOOGLE_CLOUD_PROJECT_IDが未設定のとき、読み込みはGOOGLE_CLOUD_PROJECT_IDが設定されていないことを伝えるエラーで失敗する", configtest.Unset},
					{"GOOGLE_CLOUD_PROJECT_IDが空文字のとき、読み込みはGOOGLE_CLOUD_PROJECT_IDが設定されていないことを伝えるエラーで失敗する", configtest.SetValue("")},
					{"GOOGLE_CLOUD_PROJECT_IDが空白だけのとき、読み込みはGOOGLE_CLOUD_PROJECT_IDが設定されていないことを伝えるエラーで失敗する", configtest.SetValue(" ")},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						_, err := loadConfig(t, map[string]configtest.Setting{"GOOGLE_CLOUD_PROJECT_ID": tc.setting})

						assert.EqualError(t, err, "config: missing required env var: GOOGLE_CLOUD_PROJECT_ID")
					})
				}
			})
		})
	})

	t.Run("[環境変数]マッチ成立イベントの発行先トピックの読み込み", func(t *testing.T) {
		t.Run("他の環境変数がすべて受け入れられる値のとき", func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				t.Run("MATCH_MADE_TOPICがdummy-topicのとき、サービスの設定のマッチ成立イベントの発行先トピック名がdummy-topicになる", func(t *testing.T) {
					cfg, err := loadConfig(t, map[string]configtest.Setting{"MATCH_MADE_TOPIC": configtest.SetValue("dummy-topic")})

					require.NoError(t, err)
					assert.Equal(t, "dummy-topic", cfg.MatchMadeTopic)
				})
			})

			t.Run("異常系", func(t *testing.T) {
				cases := []settingCase{
					{"MATCH_MADE_TOPICが未設定のとき、読み込みはMATCH_MADE_TOPICが設定されていないことを伝えるエラーで失敗する", configtest.Unset},
					{"MATCH_MADE_TOPICが空文字のとき、読み込みはMATCH_MADE_TOPICが設定されていないことを伝えるエラーで失敗する", configtest.SetValue("")},
					{"MATCH_MADE_TOPICが空白だけのとき、読み込みはMATCH_MADE_TOPICが設定されていないことを伝えるエラーで失敗する", configtest.SetValue(" ")},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						_, err := loadConfig(t, map[string]configtest.Setting{"MATCH_MADE_TOPIC": tc.setting})

						assert.EqualError(t, err, "config: missing required env var: MATCH_MADE_TOPIC")
					})
				}
			})
		})
	})

	t.Run("[環境変数]内部認証公開鍵の読み込み", func(t *testing.T) {
		t.Run("他の環境変数がすべて受け入れられる値のとき", func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				t.Run("INTERNAL_AUTH_PUBLIC_KEYがdummy-public-keyのとき、サービスの設定の内部認証公開鍵がdummy-public-keyになる", func(t *testing.T) {
					cfg, err := loadConfig(t, map[string]configtest.Setting{"INTERNAL_AUTH_PUBLIC_KEY": configtest.SetValue("dummy-public-key")})

					require.NoError(t, err)
					assert.Equal(t, "dummy-public-key", cfg.InternalAuthPublicKey)
				})
			})

			t.Run("異常系", func(t *testing.T) {
				cases := []settingCase{
					{"INTERNAL_AUTH_PUBLIC_KEYが未設定のとき、読み込みはINTERNAL_AUTH_PUBLIC_KEYが設定されていないことを伝えるエラーで失敗する", configtest.Unset},
					{"INTERNAL_AUTH_PUBLIC_KEYが空文字のとき、読み込みはINTERNAL_AUTH_PUBLIC_KEYが設定されていないことを伝えるエラーで失敗する", configtest.SetValue("")},
					{"INTERNAL_AUTH_PUBLIC_KEYが空白だけのとき、読み込みはINTERNAL_AUTH_PUBLIC_KEYが設定されていないことを伝えるエラーで失敗する", configtest.SetValue(" ")},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						_, err := loadConfig(t, map[string]configtest.Setting{"INTERNAL_AUTH_PUBLIC_KEY": tc.setting})

						assert.EqualError(t, err, "config: missing required env var: INTERNAL_AUTH_PUBLIC_KEY")
					})
				}
			})
		})
	})

	t.Run("[環境変数]正の整数で指定する項目の読み込み", func(t *testing.T) {
		t.Run("他の環境変数がすべて受け入れられる値のとき", func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				t.Run("PORTが1のとき、サービスの設定のリッスンポートが1になる", func(t *testing.T) {
					cfg, err := loadConfig(t, map[string]configtest.Setting{"PORT": configtest.SetValue("1")})

					require.NoError(t, err)
					assert.Equal(t, 1, cfg.Port)
				})

				t.Run("PORTが65535のとき、サービスの設定のリッスンポートが65535になる", func(t *testing.T) {
					cfg, err := loadConfig(t, map[string]configtest.Setting{"PORT": configtest.SetValue("65535")})

					require.NoError(t, err)
					assert.Equal(t, 65535, cfg.Port)
				})
			})

			t.Run("異常系", func(t *testing.T) {
				missingCases := []settingCase{
					{"PORTが未設定のとき、読み込みはPORTが設定されていないことを伝えるエラーで失敗する", configtest.Unset},
					{"PORTが空文字のとき、読み込みはPORTが設定されていないことを伝えるエラーで失敗する", configtest.SetValue("")},
				}
				for _, tc := range missingCases {
					t.Run(tc.name, func(t *testing.T) {
						_, err := loadConfig(t, map[string]configtest.Setting{"PORT": tc.setting})

						assert.EqualError(t, err, "config: missing required env var: PORT")
					})
				}

				notPositiveCases := []settingErrorCase{
					{"PORTが0のとき、読み込みはPORTが0より大きい値でないことを伝えるエラーで失敗する", configtest.SetValue("0"), "config: PORT must be > 0, got 0"},
					{"PORTが-1のとき、読み込みはPORTが0より大きい値でないことを伝えるエラーで失敗する", configtest.SetValue("-1"), "config: PORT must be > 0, got -1"},
				}
				for _, tc := range notPositiveCases {
					t.Run(tc.name, func(t *testing.T) {
						_, err := loadConfig(t, map[string]configtest.Setting{"PORT": tc.setting})

						assert.EqualError(t, err, tc.wantErr)
					})
				}

				t.Run("PORTが65536のとき、読み込みはPORTが65535を超える値であることを伝えるエラーで失敗する", func(t *testing.T) {
					_, err := loadConfig(t, map[string]configtest.Setting{"PORT": configtest.SetValue("65536")})

					assert.EqualError(t, err, "config: PORT must be <= 65535, got 65536")
				})

				notIntegerCases := []settingErrorCase{
					{"PORTがabcのとき、読み込みはPORTが整数として読めないことを伝えるエラーで失敗する", configtest.SetValue("abc"), `config: PORT "abc": `},
					{"PORTが8.5のとき、読み込みはPORTが整数として読めないことを伝えるエラーで失敗する", configtest.SetValue("8.5"), `config: PORT "8.5": `},
				}
				for _, tc := range notIntegerCases {
					t.Run(tc.name, func(t *testing.T) {
						_, err := loadConfig(t, map[string]configtest.Setting{"PORT": tc.setting})

						require.Error(t, err)
						assert.Regexp(t, "^"+regexp.QuoteMeta(tc.wantErr), err.Error())
					})
				}
			})
		})
	})

	t.Run("[環境変数]秒数で指定する項目の読み込み", func(t *testing.T) {
		t.Run("他の環境変数がすべて受け入れられる値のとき", func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				t.Run("MATCHMAKING_CIRCUIT_COOLDOWN_SECが9223372036のとき、サービスの設定のサーキットブレーカーが開いた後の再試行までの時間が9223372036秒になる", func(t *testing.T) {
					cfg, err := loadConfig(t, map[string]configtest.Setting{"MATCHMAKING_CIRCUIT_COOLDOWN_SEC": configtest.SetValue("9223372036")})

					require.NoError(t, err)
					assert.Equal(t, 9223372036*time.Second, cfg.CircuitCooldown)
				})
			})

			t.Run("異常系", func(t *testing.T) {
				t.Run("MATCHMAKING_CIRCUIT_COOLDOWN_SECが9223372037のとき、読み込みはMATCHMAKING_CIRCUIT_COOLDOWN_SECが時間に変換できる上限を超えていることを伝えるエラーで失敗する", func(t *testing.T) {
					_, err := loadConfig(t, map[string]configtest.Setting{"MATCHMAKING_CIRCUIT_COOLDOWN_SEC": configtest.SetValue("9223372037")})

					assert.EqualError(t, err, "config: MATCHMAKING_CIRCUIT_COOLDOWN_SEC must be <= 9223372036, got 9223372037")
				})
			})
		})
	})

	t.Run("[環境変数]サーキットブレーカーを開く連続失敗回数の読み込み", func(t *testing.T) {
		t.Run("他の環境変数がすべて受け入れられる値のとき", func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				t.Run("MATCHMAKING_CIRCUIT_THRESHOLDが3のとき、サービスの設定のサーキットブレーカーを開く連続失敗回数が3になる", func(t *testing.T) {
					cfg, err := loadConfig(t, map[string]configtest.Setting{"MATCHMAKING_CIRCUIT_THRESHOLD": configtest.SetValue("3")})

					require.NoError(t, err)
					assert.Equal(t, 3, cfg.CircuitThreshold)
				})
			})

			t.Run("異常系", func(t *testing.T) {
				cases := []settingCase{
					{"MATCHMAKING_CIRCUIT_THRESHOLDが未設定のとき、読み込みはMATCHMAKING_CIRCUIT_THRESHOLDが設定されていないことを伝えるエラーで失敗する", configtest.Unset},
					{"MATCHMAKING_CIRCUIT_THRESHOLDが空文字のとき、読み込みはMATCHMAKING_CIRCUIT_THRESHOLDが設定されていないことを伝えるエラーで失敗する", configtest.SetValue("")},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						_, err := loadConfig(t, map[string]configtest.Setting{"MATCHMAKING_CIRCUIT_THRESHOLD": tc.setting})

						assert.EqualError(t, err, "config: missing required env var: MATCHMAKING_CIRCUIT_THRESHOLD")
					})
				}
			})
		})
	})

	t.Run("[環境変数]サーキットブレーカーの再試行までの時間の読み込み", func(t *testing.T) {
		t.Run("他の環境変数がすべて受け入れられる値のとき", func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				t.Run("MATCHMAKING_CIRCUIT_COOLDOWN_SECが45のとき、サービスの設定のサーキットブレーカーが開いた後の再試行までの時間が45秒になる", func(t *testing.T) {
					cfg, err := loadConfig(t, map[string]configtest.Setting{"MATCHMAKING_CIRCUIT_COOLDOWN_SEC": configtest.SetValue("45")})

					require.NoError(t, err)
					assert.Equal(t, 45*time.Second, cfg.CircuitCooldown)
				})
			})

			t.Run("異常系", func(t *testing.T) {
				cases := []settingCase{
					{"MATCHMAKING_CIRCUIT_COOLDOWN_SECが未設定のとき、読み込みはMATCHMAKING_CIRCUIT_COOLDOWN_SECが設定されていないことを伝えるエラーで失敗する", configtest.Unset},
					{"MATCHMAKING_CIRCUIT_COOLDOWN_SECが空文字のとき、読み込みはMATCHMAKING_CIRCUIT_COOLDOWN_SECが設定されていないことを伝えるエラーで失敗する", configtest.SetValue("")},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						_, err := loadConfig(t, map[string]configtest.Setting{"MATCHMAKING_CIRCUIT_COOLDOWN_SEC": tc.setting})

						assert.EqualError(t, err, "config: missing required env var: MATCHMAKING_CIRCUIT_COOLDOWN_SEC")
					})
				}
			})
		})
	})

	t.Run("[環境変数]シャットダウン時の定期処理の完了待ち時間の読み込み", func(t *testing.T) {
		t.Run("他の環境変数がすべて受け入れられる値のとき", func(t *testing.T) {
			t.Run("正常系", func(t *testing.T) {
				t.Run("MATCHMAKING_DRAIN_TIMEOUT_SECが20のとき、サービスの設定のシャットダウン時に処理中の定期処理の完了を待つ時間が20秒になる", func(t *testing.T) {
					cfg, err := loadConfig(t, map[string]configtest.Setting{"MATCHMAKING_DRAIN_TIMEOUT_SEC": configtest.SetValue("20")})

					require.NoError(t, err)
					assert.Equal(t, 20*time.Second, cfg.DrainTimeout)
				})
			})

			t.Run("異常系", func(t *testing.T) {
				cases := []settingCase{
					{"MATCHMAKING_DRAIN_TIMEOUT_SECが未設定のとき、読み込みはMATCHMAKING_DRAIN_TIMEOUT_SECが設定されていないことを伝えるエラーで失敗する", configtest.Unset},
					{"MATCHMAKING_DRAIN_TIMEOUT_SECが空文字のとき、読み込みはMATCHMAKING_DRAIN_TIMEOUT_SECが設定されていないことを伝えるエラーで失敗する", configtest.SetValue("")},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						_, err := loadConfig(t, map[string]configtest.Setting{"MATCHMAKING_DRAIN_TIMEOUT_SEC": tc.setting})

						assert.EqualError(t, err, "config: missing required env var: MATCHMAKING_DRAIN_TIMEOUT_SEC")
					})
				}
			})
		})
	})

	t.Run("[環境変数]読み込みエラーに示す環境変数の不備", func(t *testing.T) {
		t.Run("異常系", func(t *testing.T) {
			t.Run("APP_ENVがlocalで、UPSTASH_REDIS_URL・GOOGLE_CLOUD_PROJECT_ID・MATCH_MADE_TOPIC・INTERNAL_AUTH_PUBLIC_KEY・PORT・MATCHMAKING_CIRCUIT_THRESHOLD・MATCHMAKING_CIRCUIT_COOLDOWN_SEC・MATCHMAKING_DRAIN_TIMEOUT_SECが全て未設定のとき", func(t *testing.T) {
				cases := []struct {
					name    string
					envName string
				}{
					{"読み込むと、エラーの内容にUPSTASH_REDIS_URLが設定されていないことを伝える文言が含まれる", "UPSTASH_REDIS_URL"},
					{"読み込むと、エラーの内容にGOOGLE_CLOUD_PROJECT_IDが設定されていないことを伝える文言が含まれる", "GOOGLE_CLOUD_PROJECT_ID"},
					{"読み込むと、エラーの内容にMATCH_MADE_TOPICが設定されていないことを伝える文言が含まれる", "MATCH_MADE_TOPIC"},
					{"読み込むと、エラーの内容にINTERNAL_AUTH_PUBLIC_KEYが設定されていないことを伝える文言が含まれる", "INTERNAL_AUTH_PUBLIC_KEY"},
					{"読み込むと、エラーの内容にPORTが設定されていないことを伝える文言が含まれる", "PORT"},
					{"読み込むと、エラーの内容にMATCHMAKING_CIRCUIT_THRESHOLDが設定されていないことを伝える文言が含まれる", "MATCHMAKING_CIRCUIT_THRESHOLD"},
					{"読み込むと、エラーの内容にMATCHMAKING_CIRCUIT_COOLDOWN_SECが設定されていないことを伝える文言が含まれる", "MATCHMAKING_CIRCUIT_COOLDOWN_SEC"},
					{"読み込むと、エラーの内容にMATCHMAKING_DRAIN_TIMEOUT_SECが設定されていないことを伝える文言が含まれる", "MATCHMAKING_DRAIN_TIMEOUT_SEC"},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						_, err := loadConfig(t, map[string]configtest.Setting{
							"APP_ENV":                          configtest.SetValue("local"),
							"UPSTASH_REDIS_URL":                configtest.Unset,
							"GOOGLE_CLOUD_PROJECT_ID":          configtest.Unset,
							"MATCH_MADE_TOPIC":                 configtest.Unset,
							"INTERNAL_AUTH_PUBLIC_KEY":         configtest.Unset,
							"PORT":                             configtest.Unset,
							"MATCHMAKING_CIRCUIT_THRESHOLD":    configtest.Unset,
							"MATCHMAKING_CIRCUIT_COOLDOWN_SEC": configtest.Unset,
							"MATCHMAKING_DRAIN_TIMEOUT_SEC":    configtest.Unset,
						})

						assert.ErrorContains(t, err, "missing required env var: "+tc.envName)
					})
				}
			})

			t.Run("APP_ENVがlocalで、GOOGLE_CLOUD_PROJECT_IDが未設定、PORTが0、MATCHMAKING_CIRCUIT_THRESHOLDがabcで、他の環境変数がすべて受け入れられる値のとき", func(t *testing.T) {
				overrides := map[string]configtest.Setting{
					"APP_ENV":                       configtest.SetValue("local"),
					"GOOGLE_CLOUD_PROJECT_ID":       configtest.Unset,
					"PORT":                          configtest.SetValue("0"),
					"MATCHMAKING_CIRCUIT_THRESHOLD": configtest.SetValue("abc"),
				}

				t.Run("読み込むと、エラーの内容にGOOGLE_CLOUD_PROJECT_IDが設定されていないことを伝える文言が含まれる", func(t *testing.T) {
					_, err := loadConfig(t, overrides)

					assert.ErrorContains(t, err, "missing required env var: GOOGLE_CLOUD_PROJECT_ID")
				})

				t.Run("読み込むと、エラーの内容にPORTが0より大きい値でないことを伝える文言が含まれる", func(t *testing.T) {
					_, err := loadConfig(t, overrides)

					assert.ErrorContains(t, err, "PORT must be > 0, got 0")
				})

				t.Run("読み込むと、エラーの内容にMATCHMAKING_CIRCUIT_THRESHOLDが整数として読めないことを伝える文言が含まれる", func(t *testing.T) {
					_, err := loadConfig(t, overrides)

					assert.ErrorContains(t, err, `MATCHMAKING_CIRCUIT_THRESHOLD "abc": `)
				})
			})
		})
	})
}
