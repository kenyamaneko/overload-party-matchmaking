# overload-party-matchmaking

マッチメイキングキューを管理する内部マイクロサービス。キューに入ったプレイヤーをペアリングし、`match_made` イベントを Cloud Pub/Sub に publish する。

## サービス間連携

```
Gateway (唯一の呼び出し元)
  ├─ POST /internal/v1/enqueue   ← WS matchmaking_start を中継
  ├─ POST /internal/v1/cancel    ← WS matchmaking_cancel を中継
  └─ GET  /internal/v1/queue-size
                │
                ▼
Matchmaking (このサービス)
  ├─ Upstash Redis Sorted Set (キュー永続化)
  └─ Cloud Pub/Sub publish → matchmaking-events トピック
                                    │
                                    ▼
                              Gateway (subscriber)
                                ├─ battle RPC 呼び出し
                                └─ WS push-back (match_found)
```

- battle を直接呼び出さない。battle トポロジの解決は gateway 側
- DB スキーマなし。すべての状態は Redis と Pub/Sub に存在する

エンドポイント一覧は [docs/API_REFERENCE.md](docs/API_REFERENCE.md) を参照。

## 環境変数

全て必須。未設定なら起動時に即 fail する。

**Deployment env (インフラ層):**

| 変数名 | 説明 |
|---|---|
| `PORT` | リッスンポート |
| `PUBSUB_PROJECT_ID` | GCP プロジェクト ID |
| `PUBSUB_TOPIC` | Pub/Sub トピック名 |

**Secret:**

| 変数名 | 説明 |
|---|---|
| `UPSTASH_REDIS_URL` | Upstash Redis TLS URL (`rediss://...`) |

**ConfigMap (アプリ挙動):**

| 変数名 | 説明 |
|---|---|
| `MATCHMAKING_CIRCUIT_THRESHOLD` | circuit を open にする連続 publish 失敗回数 |
| `MATCHMAKING_CIRCUIT_COOLDOWN_SEC` | circuit open 後、再試行までの秒数 |
| `MATCHMAKING_DRAIN_TIMEOUT_SEC` | shutdown 時に in-flight tick の完了を待つ秒数 |

## 公開 Go パッケージ

`packages/api-matchmaking/` — gateway が `go get` で import する REST + Pub/Sub 契約型。

- SSoT: `data/models.yaml`
- 再生成: `python3 scripts/generate_types.py`
- `*_gen.go` は自動生成 — 直接編集しない
