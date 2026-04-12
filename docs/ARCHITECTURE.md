# Matchmaking サービス設計

このドキュメントはマッチメイキングサービスの内部動作を説明する。サービスの概要・エンドポイント・環境変数は [README.md](../README.md) を参照。

## マッチングループ

バックグラウンド goroutine が 1 秒間隔で tick し、以下を繰り返す:

1. Lua スクリプトで Redis Sorted Set (`matchmaking:queue`) からペアをアトミックに pop (`ZPOPMIN` ベース、2 人未満なら no-op)
2. `mch_<ULID>` 形式のマッチ ID を生成
3. `MatchMadeEvent` を Cloud Pub/Sub に publish
4. publish 失敗時は re-enqueue (後述)

キューのメンバー形式は `<playerID>:<deckID>`、スコアは `joinedAt` ミリ秒。冪等 enqueue は同一 playerID の既存エントリを削除してから ZADD する。

## Pub/Sub 契約

| 項目 | 値 |
|---|---|
| トピック | `matchmaking-events` |
| Subscription (gateway 側) | `matchmaking-events-gateway` |
| DLQ | `matchmaking-events-dlq` |
| 配信保証 | Exactly-Once |
| ペイロード型 | `MatchMadeEvent` — `packages/api-matchmaking/events_gen.go` を参照 |

メッセージ属性に `matchId` を含める。Gateway subscriber はこの値でインメモリ重複排除を行う。battle 側のゲーム作成が同一 `matchId` に対して冪等であるため、Pub/Sub の Exactly-Once 保証が破れても安全。

## サーキットブレーカー

連続する Pub/Sub publish 失敗に対してサーキットブレーカーを持つ。

1. `MATCHMAKING_CIRCUIT_THRESHOLD` 回連続で失敗すると circuit が open になる
2. tick() は Redis からの pop を停止し、プレイヤーがキューから失われるのを防ぐ
3. `/internal/v1/health` が 503 を返し、k8s がトラフィックをドレインする
4. `MATCHMAKING_CIRCUIT_COOLDOWN_SEC` 経過後に trial tick を許可する
5. 成功すればカウンターリセット、失敗すれば circuit 再 open

## Re-enqueue リトライ

publish 失敗時、pop 済みペアを元の `joinedAt` スコアでキューに再投入し FIFO 順序を保持する。

- 100ms から始まる指数バックオフで最大 5 回リトライ
- `ctx.Done()` 発生時は 2 秒の背景コンテキストで最善努力の re-enqueue を 1 回実行する (graceful shutdown 中にプレイヤーを暗黙 drop しない)
- 5 回すべて失敗した場合は `LOST pair` をログに記録する。単一のロストペアでサービスは落とさない

## Graceful drain

`matcher.Run` は各 `tick()` を WaitGroup で包む。`ctx.Done()` で ticker を停止し、in-flight tick の完了を `MATCHMAKING_DRAIN_TIMEOUT_SEC` まで待つ。デッドライン超過時は tick コンテキストをキャンセルして強制終了する。`main()` は matcher 完了後に Pub/Sub publisher を close する。
