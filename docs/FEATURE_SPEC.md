# Matchmaking 機能仕様書

このドキュメントは matchmaking サービスがビジネス要件として満たすべき振る舞いを定義する。実装方法ではなく **何を保証するか** を記述する。テストはこの仕様に従っていることを確認する観点で書く。

関連ドキュメント:
- 内部動作・配線・本番運用設定: [ARCHITECTURE.md](ARCHITECTURE.md)
- REST 契約: [../data/openapi.yaml](../data/openapi.yaml)
- Pub/Sub 契約: [../data/asyncapi.yaml](../data/asyncapi.yaml)
- キュー / イベントスキーマ: [DATA_DESIGN.md](DATA_DESIGN.md)

---

## サービス責務

matchmaking は以下の機能ドメインを所有する。

| 機能 | 主要な責務 |
|---|---|
| キュー登録 (`Enqueue`) | プレイヤーをマッチ待機キューに追加 |
| キュー取消 (`Cancel`) | プレイヤーをキューから除外 |
| キューサイズ参照 | 現在のキュー長を返す |
| マッチ成立 | キュー先頭 2 名をペアリングし `match_made` を publish |
| ヘルス公開 | サーキットブレーカー状態を含む稼働状況を返す |

matchmaking は **Redis Sorted Set をキューの唯一の真実とし**、他サービスへの状態同期は Pub/Sub publish のみで行う。battle / account を直接呼び出さない。

---

## キュー登録 (`Enqueue`)

**入力**: `playerID` (UUID v4 文字列。`X-Internal-Auth` JWT の sub クレームから解決し body には含めない), `deckID` (int64), `name` / `level` (enqueue 時点の player summary snapshot), `gatewayInstanceID` (gateway プロセスが起動時に生成する識別子)
**出力**: 202 Accepted（ボディなし） / 400 / 401 / 503

### 仕様

1. `deckID` / `name` / `gatewayInstanceID` のバリデーション (0・空は 400)。`X-Internal-Auth` の検証失敗は 401
2. `gatewayInstanceID` が直前の Enqueue と異なる場合、別プロセスへの切り替わりとみなし `matchmaking:queue` を空にする (「gatewayInstanceID によるキューリセット」)
3. Redis Sorted Set `matchmaking:queue` に対し、**同一 playerID の既存メンバーを全削除してから** 新しいメンバー `<playerID>:<deckID>:<level>:<base64(name)>` をスコア `now().UnixMilli()` で ZADD
4. 2〜3 は Lua スクリプトで 1 ラウンドトリップ・アトミックに実行
5. Redis 到達不能は 503 (インメモリフォールバックは持たない)

### 冪等性契約

- 同一 playerID の再 enqueue は **常に「最後の enqueue 時刻で待機開始」** に上書きされる
- 別 deckID で re-enqueue したケースでも最新の `(deckID, joinedAt)` で上書きされる
- プレイヤー視点で「2 重待機」になることはない (同一 playerID の行は常に 1 行以下)

> **注意**: 業務的には冪等ではない。再 enqueue すると FIFO 位置が後退する。gateway 側で WS ハートビート等により無駄な再 enqueue を抑止する前提。

---

## gatewayInstanceID によるキューリセット

キュー登録の取消 (`Cancel`) は gateway プロセスから送られる。gateway プロセスが待機タイムアウト・切断検知・明示的なキャンセル操作のいずれかで取消を送る前にプロセスごと消えると、取消が届かず待機不在のプレイヤーがキューに残り続ける。

### 仕様

1. gateway は起動時に生成した識別子 (`gateway_instance_id`) を Enqueue のたびに送る
2. matchmaking は最後に受け取った `gateway_instance_id` を保持する
3. 保持している識別子と異なる値を受け取ったとき、別プロセスへの切り替わりとみなしキュー全体を空にしてから登録する。識別子を保持していない場合も異なる値として扱いリセットする
4. リセットで削除した件数を記録する

### 成り立つ理由

gateway は同時に 1 プロセスしか動かないため、全ての接続は 1 プロセスに集まる。起動直後の gateway は接続を 1 つも持たない。したがって別プロセスからの最初の登録が来た時点で、キューに残っている全エントリは前のプロセスと一緒に死んだプレイヤーのものである。

---

## キュー取消 (`Cancel`)

**入力**: `playerID` (`X-Internal-Auth` JWT の sub クレームから解決)
**出力**: 200 OK / 401 / 404 / 503

### 仕様

1. `matchmaking:queue` から `<playerID>:*` に一致するメンバーを全削除
2. 削除件数 0 なら 404、1 件以上なら 200
3. 削除処理は Lua で atomic に実行

### 冪等性契約

- 同一 playerID の cancel を連続で呼んでも副作用は増えない (2 回目以降は 404)
- マッチ成立直後や WS 切断時の best-effort cancel で 404 が返ることは正常系に含まれる

---

## マッチ成立 (`match_made`)

バックグラウンドループが 1 秒間隔で tick し、キュー先頭 2 名をアトミックに pop してペアリングし `MatchMadeEvent` を `match-made` 論理チャネル (物理 topic は env `MATCH_MADE_TOPIC`) に publish する。

### ペアリング順序契約 (FIFO)

- `joinedAt` が古い順に 2 名ずつ取り出す (Sorted Set の score = `joinedAt` ms、`ZPOPMIN` で先頭から pop)
- MMR・デッキ相性・Bot 混入などは **現時点では考慮しない**。純粋な FIFO
- 奇数人数の余りプレイヤーは次 tick まで待機
- publish 失敗で re-enqueue する際は **元の `joinedAt` でスコアを復元** する。後から来たプレイヤーに先に抜かれない

### マッチ ID

- 形式: `mch_<ULID>`
- ULID のタイムスタンプ部は発行時刻を反映する (同一 tick 内なら辞書順 = 時系列順)

### 配信保証 (gateway との契約)

- **契約**: Exactly-Once (Cloud Pub/Sub Exactly-Once delivery を採用)
- **破れた場合の保険**: gateway の全 Pod が競合コンシューマとして subscribe し、payload の `match_id` によるインメモリ重複排除を行う。battle 側のゲーム作成が同一 `match_id` に対して冪等であるため、万一 Exactly-Once が破れても二重ゲーム作成は発生しない
- publish 成功 = acknowledge。publish 前にキューから取り出した時点では acknowledge 扱いにしない (re-enqueue で巻き戻せる状態に保つ)

### publish 失敗時のリカバリ契約

publish 失敗時、matchmaking は **プレイヤーをキューから暗黙に drop しない** (pop 時点から gateway プロセスが切り替わっていた場合を除く):

1. 元の `joinedAt` スコアで即座に re-enqueue を試行 (指数バックオフ最大 5 回、初期 100ms)
2. pop 時点で保持していた `gateway_instance_id` が現在の保持値と異なる場合、書き戻さず Warn ログを記録してリトライを終える。前のプロセスと一緒に接続が失われたプレイヤーであり、待機に戻しても救えないため
3. リトライ途中で ctx キャンセル (shutdown) を検出したら、2 秒の背景コンテキストで最終 1 回だけ re-enqueue を試みる (この最終試行にも 2 と同じ判定を適用する)
4. 5 回すべて失敗した場合のみ `LOST pair` をログに記録。単一のロストペアでサービスは落とさない
5. 連続 publish 失敗が閾値 (`MATCHMAKING_CIRCUIT_THRESHOLD`) を超えるとサーキットブレーカーが open し、tick は pop 自体を停止する

### ペイロード

- `event_type`: 定数 `"match_made"` (discriminator)
- `match_id`: 「マッチ ID」の形式
- `players`: 2 要素の配列。順序は queue pop 順 (古く enqueue した方が index 0)

スキーマ詳細は [`data/asyncapi.yaml`](../data/asyncapi.yaml) を SSoT とする。変更後は `scripts/generate_types.sh` で `packages/api-matchmaking/asyncapi_gen.go` を再生成する。

---

## ヘルスチェック (`/internal/v1/health`)

**入力**: なし
**出力**: JSON

| 状態 | HTTP | body |
|---|---|---|
| 正常 | 200 | `{"status":"ok","circuit":"closed"}` |
| サーキット open | 503 | `{"status":"degraded","circuit":"open"}` |

### サーキット open の意味

- Pub/Sub publish が `MATCHMAKING_CIRCUIT_THRESHOLD` 回連続で失敗した状態
- マッチングループは **pop を止める** (プレイヤーをキューに残したまま待機)
- k8s Service のヘルスチェックが 503 で traffic をドレインすることを前提とした設計
- `MATCHMAKING_CIRCUIT_COOLDOWN_SEC` 経過後、1 回の trial tick で成功すれば自動復旧

### Redis 障害との関係

Redis 到達不能は個々のエンドポイントで 503 を返すが、ヘルスチェック自体は Redis を触らない。ヘルスチェックが 503 を返すのはサーキット open 時のみ。Redis 障害時の traffic ドレインは個々の 503 応答に依存する。

---

## Graceful shutdown

SIGTERM 受信時の契約:

1. HTTP サーバは新規リクエスト受付を止める
2. マッチングループは ticker を止め、in-flight tick の完了を `MATCHMAKING_DRAIN_TIMEOUT_SEC` まで待つ
3. ドレイン中に publish 失敗が起きた場合、`ctx.Done()` 後も背景コンテキストで最終 re-enqueue を 1 回試行する (「publish 失敗時のリカバリ契約」)
4. ドレインタイムアウト超過時は tick を強制キャンセル。この場合 pop 済みで publish 前のペアがロストするリスクがあり、`LOST pair` がログされる
5. Pub/Sub publisher は matcher 完了後に close する (publish 中の close を避ける)

キュー自体の状態は Redis 側にあるため、matchmaking Pod の再起動でキューは失われない。

---

## エラー応答の契約

サービス層は HTTP ステータスを知らない。handler と認証 middleware で以下に分類する:

| 原因 | HTTP |
|---|---|
| リクエスト JSON 不正、必須フィールド欠落 | 400 |
| `X-Internal-Auth` の検証失敗 (`Enqueue` / `Cancel`) | 401 |
| `Cancel` 対象がキューに存在しない | 404 |
| Redis 到達不能、その他インフラ障害 | 503 |
| サーキットブレーカー open (health のみ) | 503 |

gateway は 5xx と 4xx を区別し、5xx は WS 上でユーザーにリトライを促す。Cancel の 404 は WS 切断タイミングと tick の競合で正常系でも発生しうる (例: マッチ成立直後の cancel) ため、gateway 側でエラーとして扱わない前提。
