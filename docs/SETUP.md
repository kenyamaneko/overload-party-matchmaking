# セットアップ

## 環境変数

全て必須。未設定なら起動時に即失敗する（暗黙のフォールバックなし）。

**実行環境識別:**

| 変数名 | 説明 |
|---|---|
| `APP_ENV` | `local` または `production`。値に応じて Redis 接続経路が切り替わる（local: `UPSTASH_REDIS_URL` 直接 / production: Secret Manager 経由） |

**インフラ層:**

| 変数名 | 説明 |
|---|---|
| `PORT` | リッスンポート |
| `GOOGLE_CLOUD_PROJECT_ID` | Google Cloud プロジェクト ID（Pub/Sub と Secret Manager 両方で使用） |
| `MATCH_MADE_TOPIC` | `match_made` イベント発行先の物理 Pub/Sub トピック名 |

**ローカル専用 (`APP_ENV=local` のときだけ必須):**

| 変数名 | 説明 |
|---|---|
| `UPSTASH_REDIS_URL` | Valkey 接続 URL (`redis://...`)。production では参照されない |

**アプリ挙動:**

| 変数名 | 説明 |
|---|---|
| `MATCHMAKING_CIRCUIT_THRESHOLD` | サーキットブレーカーを開く連続 publish 失敗回数 |
| `MATCHMAKING_CIRCUIT_COOLDOWN_SEC` | サーキットブレーカーが開いた後、再試行までの秒数 |
| `MATCHMAKING_DRAIN_TIMEOUT_SEC` | シャットダウン時に処理中の定期処理の完了を待つ秒数 |

**内部認証:**

| 変数名 | 説明 |
|---|---|
| `INTERNAL_AUTH_PUBLIC_KEY` | gateway が `/internal/v1/enqueue` / `/internal/v1/cancel` に付与する `X-Internal-Auth` (RS256 JWT) を検証する公開鍵 (PEM) |

**production 経路での Upstash 認証:**

`APP_ENV=production` の場合、Redis のエンドポイント/パスワードは環境変数ではなく Google Cloud Secret Manager から実行時取得する。Workload Identity でバインドされた Service Account に `roles/secretmanager.secretAccessor` を付与しておく必要がある。参照する secret ID:

- `matchmaking-upstash-redis-endpoint`：`host:port` 形式
- `matchmaking-upstash-redis-password`：Upstash の TCP パスワード
