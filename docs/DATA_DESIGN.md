# matchmaking - データ設計

> **DDL は無い。** matchmaking は DB を持たず、永続状態はすべて Redis Sorted Set と Pub/Sub メッセージの中にある。本ドキュメントはそれらのキー名・member / payload フォーマット・Lua スクリプトの不変条件を定義する。

関連ドキュメント:
- 設計判断 (Why): [common の ADR](https://github.com/kenyamaneko/overload-party-common/tree/main/docs/adr)
- REST 契約: [../data/openapi.yaml](../data/openapi.yaml)
- Pub/Sub 契約: [../data/asyncapi.yaml](../data/asyncapi.yaml)

---

## Redis キー一覧

| キー | 型 | 用途 |
|---|---|---|
| `matchmaking:queue` | Sorted Set | マッチ待機キュー |
| `matchmaking:gateway_instance_id` | String | Enqueue が最後に受け取った gateway プロセスの識別子 |

他の matchmaking:* 名前空間のキーが増える場合は設計を見直す (本来 battle や gateway に置くべき状態を matchmaking に持ち込んでいないか)。`gateway_instance_id` は例外ではなく、matchmaking 自身が持つ `matchmaking:queue` を古い gateway の登録から守るための状態 (どの gateway プロセスの登録を最後に受け付けたか) であり、gateway や battle が所有すべき状態ではない。

---

## `matchmaking:queue` のスキーマ

Upstash Redis の Sorted Set。

| 要素 | フォーマット | 値 |
|---|---|---|
| member | `<playerID>:<deckID>:<level>:<base64(name)>` | playerID = UUID v4 文字列、deckID / level = 10 進 int64、name = 表示名の Base64 エンコード (パディングなし) |
| score | `float64` | `joinedAt.UnixMilli()` (ミリ秒エポック) |

### member フォーマットの決定事項

- 区切り文字は `:` を使う。パース時は **左から 4 フィールド** (playerID, deckID, level, base64(name)) に分割する (name は Base64 化により `:` を含まない前提)
- playerID は UUID v4 で `:` を含まない。`:` を含む playerID は member の先頭フィールドの境界が曖昧になり、別のプレイヤーのエントリと取り違えるため、Enqueue と Reenqueue がエラーにする
- **Hash ではなく Sorted Set にした理由**: Hash だと「FIFO 順序で先頭 2 件を atomic に pop する」操作が ZPOPMIN に相当する標準コマンドを持たず、Lua で順序フィールドを自分で管理する必要が出るため。Sorted Set の score で `joinedAt` を表現する方が自然

### score = `joinedAt` の不変条件

- **score は待機開始時刻を表す**。publish 失敗時の re-enqueue も **元の score を保持** する。後から来たプレイヤーに先に抜かれない FIFO 契約を守るため、新しいタイムスタンプで ZADD し直してはいけない
- 同一 playerID の重複エントリは **禁止**。enqueue 時と re-enqueue 時に同 playerID のメンバーを全削除してから ZADD する (「Lua スクリプトの契約」)

### 同一 playerID 重複禁止の意味

プレイヤー視点で「2 重待機」になると、同じ人がマッチ中に再マッチされ状態が壊れる。enqueue と reenqueue の Lua スクリプトは、先頭フィールドが playerID と完全に一致する既存メンバーを全て ZREM してから新メンバーを ZADD することでこの不変条件を Redis 側で強制している。アプリ側で事前に Cancel する運用に依存しない。re-enqueue も同じ規則にするのは、pop から書き戻しまでの間に同じプレイヤーが再び待機に入っていても、2 重待機にしないため。

先頭フィールドで比較するのは、`<playerID>:` の前方一致だと、`:` を含む playerID (例: `a:1`) の操作が別のプレイヤー (playerID `a`、deckID `1`) のエントリに一致するため。cancel も同じ比較を使うので、`:` を含む playerID の除外はどのエントリにも一致しない。

---

## `matchmaking:gateway_instance_id` のスキーマ

値は gateway が Enqueue リクエストに載せる識別子の文字列そのもの。

キューからの取消 (Cancel) は gateway プロセス内から送られるため、そのプロセスが取消を送る前に消えると待機不在のプレイヤーがキューに残り続ける。gateway は起動時に生成した識別子を Enqueue のたびに送り、matchmaking はこのキーに保持する値と比較する。異なれば別プロセスへの切り替わりとみなし、`matchmaking:queue` を空にしてから登録する。

個別エントリではなくキュー全体を空にする設計は、同時に動く gateway インスタンスが 1 つであることに依存する ([ADR-058 Amendment](https://github.com/kenyamaneko/overload-party-common/blob/main/docs/adr/058-gateway-on-cloudrun-single-instance.md))。gateway の最大インスタンス数を増やす場合、この掃除方式ごと見直しが必要になる。

---

## Lua スクリプトの契約

全ての書き込み操作は Lua スクリプトで 1 ラウンドトリップ・アトミックに実行する。RTT 削減が主目的ではなく、**競合条件の発生窓を塞ぐ** のが目的。

スクリプト本体は [internal/adapter/redisqueue/lua/](../internal/adapter/redisqueue/lua/) 配下の `.lua` ファイルを SSoT とする (member 文字列の組み立て・分解は Go 側の責務)。同一 playerID のエントリを取り除く関数 (`remove_player_entries.lua`) は、enqueue・cancel・reenqueue の先頭に Go 側で連結して共有する。

| スクリプト | 入出力 | 不変条件 |
|---|---|---|
| `enqueueScript` | `KEYS[1]=queue, KEYS[2]=gatewayInstanceKey, ARGV[1]=playerID, ARGV[2]=member, ARGV[3]=score, ARGV[4]=gatewayInstanceID` → リセットで削除した件数 | 実行後、同一 playerID のエントリは正確に 1 件存在。gatewayInstanceKey の保持値と ARGV[4] が異なれば queue を空にしてから登録する (保持値が無い場合も異なる値として扱う) |
| `popPairScript` | `KEYS[1]=queue, KEYS[2]=gatewayInstanceKey` → `[instanceID, m1, s1, m2, s2]` または `{}` | ZCARD ≥ 2 のときのみ pop。1 名しかいない場合は pop しない (余り 1 名をキューに残す)。取り出した時点で保持していた gatewayInstanceKey の値も合わせて返す |
| `cancelScript` | `KEYS[1]=queue, ARGV[1]=playerID` → 削除件数 | 先頭フィールドが playerID と完全に一致する全エントリを削除 (将来 deckID 変更で複数行発生しても一括除去) |
| `reenqueueScript` | `KEYS[1]=queue, KEYS[2]=gatewayInstanceKey, ARGV[1]=expectedGatewayInstanceID, ARGV[2..]=(playerID, member, score) × N` → 書き戻したとき 1、書き戻さなかったとき 0 | gatewayInstanceKey の保持値 (未保存なら空文字として扱う) が ARGV[1] と一致する場合のみ、各エントリの同一 playerID の既存エントリを取り除いてから元の score で ZADD。一致しなければ書き戻さず 0 を返す。N が 0 でも一致を判定する |

### `popPairScript` が 2 未満で no-op にする理由

1 人しかいないときに pop してしまうと、その 1 人をアプリ側で保持した状態で次の tick まで滞留し、並行する Cancel や新規 Enqueue と競合しうる。「pop したら必ずマッチ成立する」契約にするため、2 未満の場合は **キューに残したまま** no-op で戻す。

### `reenqueueScript` の用途

publish 失敗時に pop 済みペアをキューに戻す専用。通常経路では呼ばれない (Enqueue は `enqueueScript`)。`joinedAt` を元 score のまま復元するため、FIFO 順序が維持される。

`ZPOPMIN` で同時に取り出す 1 ペアは常に同じ gateway プロセス由来のため、識別子の一致判定はエントリ単位ではなくペア単位で行う。pop 時点と書き戻し時点の間に gateway プロセスが切り替わっていれば、そのペアは前のプロセスと一緒に接続が失われたプレイヤーのものであり、書き戻さない。

pop 時点・書き戻し時点のどちらも gatewayInstanceKey が未保存 (Upstash のエビクションなど) なら、その間に登録が一度も来ていないとみなし書き戻す。登録は必ず gatewayInstanceKey を書き込むため、この間に別 gateway プロセスからの登録があれば実在の識別子が保持されており、通常どおり不一致として拒否される。

### `enqueueScript` による gatewayInstanceID のリセット判定

gatewayInstanceKey の保持値が ARGV[4] と異なる場合はキューをリセットする。GET が nil (未保存) の場合も異なる値として扱う。Upstash のメモリ上限エビクションで gatewayInstanceKey だけが失われても queue の中身は残り得るため、値の有無で分岐しない。比較・削除・識別子の更新・登録を 1 スクリプトでアトミックに行い、複数ラウンドトリップに分けたときの競合窓を作らない。

---

## Pub/Sub トピック

subscriber (gateway側の購読状況) はこのリポジトリからは分からないため、変更時はgateway側の購読設定も確認する。

### `match-made` (論理 channel)

| 項目 | 値 |
|---|---|
| 論理 channel | `match-made` (asyncapi.yaml の MatchMade channel address) |
| 物理 topic 解決 | env `MATCH_MADE_TOPIC` (Terraform / k8s ConfigMap が SSoT) |
| 送出の待ち時間の上限 | 5 秒 (起動処理が渡す定数)。上限に達すると期限切れのエラーで失敗し、呼び出し元がペアを書き戻す |
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
- 送出に待ち時間の上限を設けるのは、送出先に接続できないまま応答が返らないと、定期処理が止まりマッチングが進まなくなるため
- `match_id` は `mch_` + ULID。ULID のタイムスタンプ部は発行時刻。gateway 側の dedup キーとして使う (Exactly-Once が破れた場合の保険)
- `players` は常に 2 要素。順序は pop 順 (先に enqueue した方が index 0)
- `name` / `level` は enqueue 時に受け取った player summary snapshot をそのまま同梱する (matchmaking は account を呼ばない)

### スキーマ変更手順

1. `data/asyncapi.yaml` の `MatchMadeEvent` schema を編集
2. `scripts/generate_types.sh` で `packages/api-matchmaking/asyncapi_gen.go` を再生成
3. gateway 側で `go get` するバージョンを上げる (`packages/api-matchmaking` のタグは `publish.yaml` で発行)

`*_gen.go` を直接編集しない。spec 変更後に再生成を忘れると CI の codegen drift 検出で落ちる。

---

## サーキットブレーカー

定期処理 (1 秒ごとの pop から publish までの 1 回の処理) を止める仕組み。状態はヘルスチェック (`/internal/v1/health`) の `circuit` に現れ、open のとき 503 を返す。

| 項目 | 規則 |
|---|---|
| 数える失敗 | pop の連続失敗と publish の連続失敗を **別々に** 数える。イベントの組み立て失敗も publish の失敗に数える |
| 開く条件 | どちらかの連続失敗が `MATCHMAKING_CIRCUIT_THRESHOLD` 回に達する |
| 失敗が戻る条件 | pop が成功する (ペアが無い空の場合を含む) と pop の連続失敗が 0 に戻る。publish が成功すると publish の連続失敗が 0 に戻る |
| 閉じる条件 | pop の連続失敗と publish の連続失敗のどちらもが、閾値未満になる |
| 開いている間 | 開いてから `MATCHMAKING_CIRCUIT_COOLDOWN_SEC` が経つまで pop を止める。経ったあとは、定期処理ごとに pop を試す |
| 再び開く | 再試行の pop や publish が閾値に達した失敗になると、開いたまま、その失敗の時点から再試行までの時間を数え直す |

失敗を別々に数えるのは、キューが空の定期処理 (pop は成功し publish は呼ばれない) が publish の失敗の数を消さないため。pop の失敗も数えるのは、Redis の障害をヘルスチェックの `circuit` に反映し、障害中の定期処理を止めるため。

ヘルスチェックの 503 は k8s の readiness と liveness の両方に使われるため、Redis の障害中は Pod の再起動が繰り返されうる。

## 永続化の境界とリカバリ

| 障害 | キューの扱い |
|---|---|
| matchmaking Pod 再起動 | **失われない** (キュー状態は Redis 側) |
| Upstash Redis 障害 | enqueue/cancel/pop 全てが 503。pop の連続失敗が閾値に達するとサーキット open で traffic ドレイン。復旧後は pop の成功でサーキットが閉じ、キュー状態がそのまま継続 |
| Pub/Sub 障害 | publish 失敗 → re-enqueue で元の `joinedAt` のまま復元。プレイヤーはキュー先頭に戻る (pop 時点から gateway プロセスが切り替わっていた場合を除く)。応答が返らない場合は送出の待ち時間の上限 (5 秒) で失敗にする |
| graceful shutdown 中の in-flight pop | ctx キャンセル検出後、2 秒の背景コンテキストで最終 re-enqueue 1 回。失敗時のみ `LOST pair` ログ |

キューの永続化は Upstash Redis に委譲しており、matchmaking 側ではスナップショット・バックアップを取らない。Upstash 側の可用性・耐久性保証がキュー全体の SLA 上限となる。
