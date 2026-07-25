# matchmaking - データ設計

> **DDL は無い。** matchmaking は DB を持たず、永続状態はすべて Redis Sorted Set と Pub/Sub メッセージの中にある。本ドキュメントはそれらのキー名・member / payload フォーマット・Lua スクリプトの不変条件を定義する。

関連ドキュメント:
- 内部動作・設計意図: [ARCHITECTURE.md](ARCHITECTURE.md)
- ビジネス仕様 (FIFO / Exactly-Once 契約など): [FEATURE_SPEC.md](FEATURE_SPEC.md)
- REST 契約: [../data/openapi.yaml](../data/openapi.yaml)
- Pub/Sub 契約: [../data/asyncapi.yaml](../data/asyncapi.yaml)

---

## Redis キー一覧

| キー | 型 | 用途 |
|---|---|---|
| `matchmaking:queue` | Sorted Set | マッチ待機キュー (唯一のキー) |

matchmaking は **このキー 1 つしか書かない**。他の matchmaking:* 名前空間のキーが増える場合は設計を見直す (本来 battle や gateway に置くべき状態を matchmaking に持ち込んでいないか)。

---

## `matchmaking:queue` のスキーマ

Upstash Redis の Sorted Set。

| 要素 | フォーマット | 値 |
|---|---|---|
| member | `<playerID>:<deckID>:<level>:<base64(name)>` | playerID = UUID v4 文字列、deckID / level = 10 進 int64、name = 表示名の Base64 エンコード (パディングなし) |
| score | `float64` | `joinedAt.UnixMilli()` (ミリ秒エポック) |

### member フォーマットの決定事項

- 区切り文字は `:` を使う。パース時は **左から 4 フィールド** (playerID, deckID, level, base64(name)) に分割する (UUID v4 に `:` は含まれず、name は Base64 化により `:` を含まない前提)
- **Hash ではなく Sorted Set にした理由**: Hash だと「FIFO 順序で先頭 2 件を atomic に pop する」操作が ZPOPMIN に相当する標準コマンドを持たず、Lua で順序フィールドを自分で管理する必要が出るため。Sorted Set の score で `joinedAt` を表現する方が自然

### score = `joinedAt` の不変条件

- **score は待機開始時刻を表す**。publish 失敗時の re-enqueue も **元の score を保持** する ([FEATURE_SPEC.md](FEATURE_SPEC.md) の「ペアリング順序契約 (FIFO)」)。後から来たプレイヤーに先に抜かれない FIFO 契約を守るため、新しいタイムスタンプで ZADD し直してはいけない
- 同一 playerID の重複エントリは **禁止**。enqueue 時に同 playerID のメンバーを全削除してから ZADD する (「Lua スクリプトの契約」)
- score が現在時刻から一定時間以上離れたエントリは、マッチングの走査で `removeExpiredScript` により削除される (「掃除」節)

### 同一 playerID 重複禁止の意味

プレイヤー視点で「2 重待機」になると、同じ人がマッチ中に再マッチされ状態が壊れる。enqueue の Lua スクリプトは `<playerID>:` プレフィックスに一致する既存メンバーを全て ZREM してから新メンバーを ZADD することでこの不変条件を Redis 側で強制している。アプリ側で事前に Cancel する運用に依存しない。

---

## Lua スクリプトの契約

全ての書き込み操作は Lua スクリプトで 1 ラウンドトリップ・アトミックに実行する。RTT 削減が主目的ではなく、**競合条件の発生窓を塞ぐ** のが目的。

スクリプト本体は [internal/adapter/redisqueue/lua/](../internal/adapter/redisqueue/lua/) 配下の `.lua` ファイルを SSoT とする (member 文字列の組み立て・分解は Go 側の責務)。

| スクリプト | 入出力 | 不変条件 |
|---|---|---|
| `enqueueScript` | `KEYS[1]=queue, ARGV[1]=playerID, ARGV[2]=member, ARGV[3]=score` | 実行後、同一 playerID のエントリは正確に 1 件存在 |
| `popPairScript` | `KEYS[1]=queue` → `[m1, s1, m2, s2]` または `{}` | ZCARD ≥ 2 のときのみ pop。1 名しかいない場合は pop しない (余り 1 名をキューに残す) |
| `cancelScript` | `KEYS[1]=queue, ARGV[1]=playerID` → 削除件数 | `<playerID>:*` に一致する全エントリを削除 (将来 deckID 変更で複数行発生しても一括除去) |
| `reenqueueScript` | `KEYS[1]=queue, ARGV=(member, score) × N` | 元の score で ZADD。既存メンバーがあれば上書き (通常は pop 済みなので不在のはず) |
| `removeExpiredScript` | `KEYS[1]=queue, ARGV[1]=cutoff` → 削除件数 | score が `cutoff` 以下のエントリを全て削除 (`ZREMRANGEBYSCORE`) |

### `popPairScript` が 2 未満で no-op にする理由

1 人しかいないときに pop してしまうと、その 1 人をアプリ側で保持した状態で次の tick まで滞留し、並行する Cancel や新規 Enqueue と競合しうる。「pop したら必ずマッチ成立する」契約にするため、2 未満の場合は **キューに残したまま** no-op で戻す。

### `reenqueueScript` の用途

publish 失敗時に pop 済みペアをキューに戻す専用。通常経路では呼ばれない (Enqueue は `enqueueScript`)。`joinedAt` を元 score のまま復元するため、FIFO 順序が維持される。

### `removeExpiredScript` の用途

マッチングの走査 (tick) ごとに、`joinedAt` が閾値より古いエントリを削除する。詳細は [ARCHITECTURE.md](ARCHITECTURE.md) の「掃除」、業務仕様は [FEATURE_SPEC.md](FEATURE_SPEC.md) の「期限切れエントリの掃除」を参照。

---

## Pub/Sub トピック

### `match-made` (論理 channel)

| 項目 | 値 |
|---|---|
| 論理 channel | `match-made` (asyncapi.yaml の MatchMade channel address) |
| 物理 topic 解決 | env `MATCH_MADE_TOPIC` (Terraform / k8s ConfigMap が SSoT) |
| Subscription (gateway 側) | infra リポで管理 |
| DLQ | infra リポで管理 |
| 配信保証 | Exactly-Once |
| payload スキーマ | `MatchMadeEvent` (`packages/api-matchmaking/asyncapi_gen.go`) |

### payload (`MatchMadeEvent`)

```json
{
  "event_type": "match_made",
  "match_id": "mch_01HXXXXXXXXXXXXXXXXXXXXXXX",
  "players": [
    {"player_id": "<uuid>", "deck_id": 123, "name": "<表示名>", "level": 7},
    {"player_id": "<uuid>", "deck_id": 456, "name": "<表示名>", "level": 12}
  ]
}
```

- `event_type` は discriminator (将来別種イベントを同トピックに乗せる余地)
- `match_id` は `mch_` + ULID。ULID のタイムスタンプ部は発行時刻。gateway 側の dedup キーとして使う (Exactly-Once が破れた場合の保険、[FEATURE_SPEC.md](FEATURE_SPEC.md) の「配信保証 (gateway との契約)」)
- `players` は常に 2 要素。順序は pop 順 (先に enqueue した方が index 0)
- `name` / `level` は enqueue 時に受け取った player summary snapshot をそのまま同梱する (matchmaking は account を呼ばない)

### スキーマ変更手順

1. `data/asyncapi.yaml` の `MatchMadeEvent` schema を編集
2. `scripts/generate_types.sh` で `packages/api-matchmaking/asyncapi_gen.go` を再生成
3. gateway 側で `go get` するバージョンを上げる (`packages/api-matchmaking` のタグは `publish.yaml` で発行)

`*_gen.go` を直接編集しない。spec 変更後に再生成を忘れると CI の codegen drift 検出で落ちる。

---

## 永続化の境界とリカバリ

| 障害 | キューの扱い |
|---|---|
| matchmaking Pod 再起動 | **失われない** (キュー状態は Redis 側) |
| Upstash Redis 障害 | enqueue/cancel/pop 全てが 503。サーキット open で traffic ドレイン。復旧後はキュー状態がそのまま継続 |
| Pub/Sub 障害 | publish 失敗 → re-enqueue で元の `joinedAt` のまま復元。プレイヤーはキュー先頭に戻る |
| graceful shutdown 中の in-flight pop | ctx キャンセル検出後、2 秒の背景コンテキストで最終 re-enqueue 1 回。失敗時のみ `LOST pair` ログ |

キューの永続化は Upstash Redis に委譲しており、matchmaking 側ではスナップショット・バックアップを取らない。Upstash 側の可用性・耐久性保証がキュー全体の SLA 上限となる。
