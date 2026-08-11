# overload-party-matchmaking

マッチメイキングキューを管理する内部マイクロサービス。キューに入ったプレイヤーをペアリングし、`match_made` イベントを Cloud Pub/Sub に publish する。

[テスト観点カタログ](https://kenyamaneko.github.io/overload-party-matchmaking/): テスト名から生成した、テスト済みの観点の一覧。

## サービス間連携

```
Gateway (唯一の呼び出し元)
  ├─ POST /internal/v1/enqueue          ← WS matchmaking_start を中継
  ├─ POST /internal/v1/cancel           ← WS matchmaking_cancel を中継
  ├─ POST /internal/v1/match-abandoned  ← 配信先未接続によるマッチ不成立を申告
  └─ GET  /internal/v1/queue-size
                │
                ▼
Matchmaking (このサービス)
  ├─ Upstash Redis Sorted Set (キュー永続化)
  └─ Cloud Pub/Sub publish → env MATCH_MADE_TOPIC トピック
                                    │
                                    ▼
                              Gateway (subscriber)
                                ├─ battle RPC 呼び出し
                                └─ WS push-back (match_found)
```

- battle を直接呼び出さない。battle トポロジの解決は gateway 側
- DB スキーマなし。すべての状態は Redis と Pub/Sub に存在する

エンドポイント・Pub/Sub 契約は [`data/openapi.yaml`](data/openapi.yaml) と [`data/asyncapi.yaml`](data/asyncapi.yaml) を直接参照する (これらが SSoT)。

## 環境変数

全て必須。未設定なら起動時に即 fail する（暗黙のフォールバックなし）。

**実行環境識別:**

| 変数名 | 説明 |
|---|---|
| `APP_ENV` | `local` または `production`。値に応じて Redis 接続経路が切り替わる（local: `UPSTASH_REDIS_URL` 直接 / production: Secret Manager 経由） |

**Deployment env (インフラ層):**

| 変数名 | 説明 |
|---|---|
| `PORT` | リッスンポート |
| `GOOGLE_CLOUD_PROJECT_ID` | Google Cloud プロジェクト ID（Pub/Sub と Secret Manager 両方で使用） |
| `MATCH_MADE_TOPIC` | `match_made` イベント発行先の物理 Pub/Sub トピック名 |

**ローカル専用 (`APP_ENV=local` のときだけ必須):**

| 変数名 | 説明 |
|---|---|
| `UPSTASH_REDIS_URL` | Valkey 接続 URL (`redis://...`)。production では参照されない |

**ConfigMap (アプリ挙動):**

| 変数名 | 説明 |
|---|---|
| `MATCHMAKING_CIRCUIT_THRESHOLD` | circuit を open にする連続 publish 失敗回数 |
| `MATCHMAKING_CIRCUIT_COOLDOWN_SEC` | circuit open 後、再試行までの秒数 |
| `MATCHMAKING_DRAIN_TIMEOUT_SEC` | shutdown 時に in-flight tick の完了を待つ秒数 |

**内部認証:**

| 変数名 | 説明 |
|---|---|
| `INTERNAL_AUTH_PUBLIC_KEY` | gateway が `/internal/v1/enqueue` / `/internal/v1/cancel` に付与する `X-Internal-Auth` (RS256 JWT) を検証する公開鍵 (PEM) |

**production 経路での Upstash 認証:**

`APP_ENV=production` の場合、Redis の endpoint/password は環境変数ではなく Google Cloud Secret Manager から実行時取得する。Workload Identity で bind された Service Account に `roles/secretmanager.secretAccessor` を付与しておく必要がある。参照する secret ID:

- `matchmaking-upstash-redis-endpoint`：`host:port` 形式
- `matchmaking-upstash-redis-password`：Upstash TCP password
