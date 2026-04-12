# API リファレンス

> 自動生成 -- 直接編集しない。`data/models.yaml` の `endpoint_groups` セクションから
> `python3 scripts/generate_constants.py` で生成される。

生成日時: `2026-04-12T06:24:30Z`

## Internal REST

ClusterIP 内部のみ到達可能。gateway が WS メッセージを変換して呼び出す。

**認証**: `internal`

**ベースパス**: `/internal/v1`

### `POST /internal/v1/enqueue`

プレイヤーをマッチングキューに追加

**リクエスト**: `EnqueueRequest`

| フィールド | 型 | JSON | 説明 |
|---|---|---|---|
| PlayerID | `string` | `playerId` | Player UUID (plain UUID v4 string). |
| DeckID | `number` | `deckId` | Deck id the player queued with. |

**成功レスポンス**: `202` Accepted（レスポンスボディなし）

**エラー**:

| ステータス | 説明 |
|---|---|
| `400` | playerId または deckId が空・不正 |
| `503` | Redis 接続エラー |

> 冪等ではない。同一プレイヤーを二重 enqueue すると ZADD で上書きされる。

### `POST /internal/v1/cancel`

プレイヤーをマッチングキューから除外

**リクエスト**: `CancelRequest`

| フィールド | 型 | JSON | 説明 |
|---|---|---|---|
| PlayerID | `string` | `playerId` | Player UUID to dequeue. |

**成功レスポンス**: `200` OK（レスポンスボディなし）

**エラー**:

| ステータス | 説明 |
|---|---|
| `400` | playerId が空・不正 |
| `404` | プレイヤーがキューに存在しない |
| `503` | Redis 接続エラー |

> 冪等。存在しない場合は 404 を返す。

### `GET /internal/v1/queue-size`

現在のマッチングキューサイズを取得

**レスポンス**: `QueueSizeResponse`

| フィールド | 型 | JSON | 説明 |
|---|---|---|---|
| Size | `number` | `size` | Current ZCARD of matchmaking:queue. |

**エラー**:

| ステータス | 説明 |
|---|---|
| `503` | Redis 接続エラー |

### `GET /internal/v1/health`

ヘルスチェック（サーキットブレーカー状態を含む）

**レスポンス**: `{"status":"ok","circuit":"closed"}` または `{"status":"degraded","circuit":"open"}`

**エラー**:

| ステータス | 説明 |
|---|---|
| `503` | サーキットブレーカーが open 状態 |

---

## Pub/Sub イベント

マッチング成立時に Cloud Pub/Sub へ publish されるイベント。HTTP エンドポイントではない。

### `PUBLISH topic: matchmaking-events`

マッチング成立イベントを Pub/Sub に publish

**レスポンス**: `MatchMadeEvent`

| フィールド | 型 | JSON | 説明 |
|---|---|---|---|
| Type | `string` | `type` | Event type discriminator, always "match_made". |
| MatchID | `string` | `matchId` | ULID-based match id, prefixed `mch_`. Dedup key on the gateway side. |
| Players | `MatchedPlayer[]` | `players` | Two-element slice — order reflects queue pop order. |

> Exactly-Once delivery。gateway の全 Pod が competing consumer として subscribe する。
> publish 失敗時はペアを元の JoinedAt で re-enqueue し、最大 5 回指数バックオフでリトライする。

---
