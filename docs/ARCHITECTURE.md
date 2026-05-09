# Matchmaking サービス設計

本ドキュメントは **コードを読んでも一見しては分からない設計意図** だけを残す。実装詳細 (フロー順序・環境変数の一覧・エラー → HTTP ステータス変換) は各ファイルの実装とコメントを一次情報とする。

サービス概要・起動手順・環境変数は [../README.md](../README.md)、REST 契約は [../data/openapi.yaml](../data/openapi.yaml)、Pub/Sub 契約は [../data/asyncapi.yaml](../data/asyncapi.yaml)、ビジネス仕様は [FEATURE_SPEC.md](FEATURE_SPEC.md)、キュー / イベントスキーマは [DATA_DESIGN.md](DATA_DESIGN.md) を参照。

## 責務境界 (state の SSoT と呼び出し関係)

matchmaking は **「待機キューの所有」** だけを責務とし、それ以外のものは持たない。DB は持たず、永続状態はすべて Redis と Pub/Sub の中にある。

| 状態 | SSoT | matchmaking の扱い |
|---|---|---|
| 待機キュー | Upstash Redis Sorted Set `matchmaking:queue` | 唯一の所有者。enqueue / cancel / pop を Lua でアトミックに実行 |
| マッチ成立イベント | Cloud Pub/Sub `match_made` (物理 topic は env `MATCH_MADE_TOPIC` で解決) | 発行のみ。publish 後は subscriber (gateway) の責務 |
| battle の state | battle サービス | **直接呼ばない**。gateway が `match_made` を購読し、battle への RPC を担う |
| プレイヤー情報 | account サービス | matchmaking は playerID を不透明な string として扱う |

gateway が唯一の呼び出し元 (ClusterIP のみ、クライアント認証なし) で、battle は matchmaking から見えない。「キューに入れる / ペアを作って通知する」以外の機能をこのサービスに足さない。

## インメモリフォールバックを持たない

Redis に到達できなければ即 503 で fail する。過去にインメモリキューのフォールバックを持たせないと決めた理由:

- 複数 Pod 構成では Pod 間でキューが分断され、Pod A と Pod B に 1 人ずつ乗ったままペアリングが永遠に発生しない
- Redis 障害中だけ受け付けたプレイヤーを Redis 復旧後に移す経路が無く、プレイヤーが見えないキューに取り残される
- 503 を返せば k8s がトラフィックをドレインでき、gateway 側でユーザーにリトライを促せる

フォールバック再導入禁止は `CLAUDE.md` にも明記してある。

## マッチングループ

バックグラウンド goroutine が 1 秒間隔で tick し、Lua スクリプトで `ZPOPMIN` ベースのアトミック pop (2 人未満なら no-op) → `mch_<ULID>` 形式のマッチ ID 生成 → `MatchMadeEvent` を Pub/Sub に publish → 失敗時は re-enqueue、を繰り返す。

冪等 enqueue は同一 playerID の既存エントリを削除してから ZADD する (Lua で 1 スクリプト内、詳細は [DATA_DESIGN.md](DATA_DESIGN.md))。

## サーキットブレーカー

連続する Pub/Sub publish 失敗に対してサーキットブレーカーを持つ。

1. `MATCHMAKING_CIRCUIT_THRESHOLD` 回連続で失敗すると circuit が open になる
2. tick は Redis からの pop を停止し、プレイヤーがキューから失われるのを防ぐ
3. `/internal/v1/health` が 503 を返し、k8s がトラフィックをドレインする
4. `MATCHMAKING_CIRCUIT_COOLDOWN_SEC` 経過後に trial tick を許可する
5. 成功すればカウンターリセット、失敗すれば circuit 再 open

pop を止める理由は、publish が壊れている間に pop を続けるとプレイヤーがキューから取り出された上で publish も re-enqueue も失敗する窓が生まれるため。open 中は「取り出さない」方が安全。

## Re-enqueue リトライ

publish 失敗時、pop 済みペアを **元の `JoinedAt` スコア** でキューに再投入し FIFO 順序を保持する。後から来たプレイヤーに先に抜かれない契約を守るため、新しいタイムスタンプで再投入してはいけない。

- 100ms から始まる指数バックオフで最大 5 回リトライ
- `ctx.Done()` 発生時は 2 秒の背景コンテキストで最善努力の re-enqueue を 1 回実行する (graceful shutdown 中にプレイヤーを暗黙 drop しない)
- 5 回すべて失敗した場合は `LOST pair` をログに記録する。単一のロストペアでサービスは落とさない

## Graceful drain

`matcher.Run` は各 `tick()` を WaitGroup で包む。`ctx.Done()` で ticker を停止し、in-flight tick の完了を `MATCHMAKING_DRAIN_TIMEOUT_SEC` まで待つ。デッドライン超過時は tick コンテキストをキャンセルして強制終了する。`main()` は matcher 完了後に Pub/Sub publisher を close する (publish 中の tick が残っている状態で publisher を閉じない)。

## 運用

### 環境変数 / Secret Manager

環境変数の一覧と必須条件は [../README.md](../README.md) と [internal/config/config.go](../internal/config/config.go) が SSoT (起動時に検証、欠ければ即 fail、暗黙フォールバック禁止)。

運用上の注意点のみ:

- **`APP_ENV=production`** では Upstash Redis の endpoint / password を Secret Manager から起動時に取得する。k8s マニフェストにシークレットは載せない
- 参照する secret ID は `matchmaking-upstash-redis-endpoint` / `matchmaking-upstash-redis-password`。Workload Identity で bind された Service Account に `roles/secretmanager.secretAccessor` が必要
- **`MATCHMAKING_CIRCUIT_THRESHOLD` / `MATCHMAKING_CIRCUIT_COOLDOWN_SEC` / `MATCHMAKING_DRAIN_TIMEOUT_SEC`** は ConfigMap 経由。負荷試験やインシデント時にコード変更なしで試行錯誤できるよう env で持つ

### Pub/Sub トピックと subscriber

| 論理 channel | 物理 topic 解決 | 発行契機 | subscriber |
|---|---|---|---|
| `match-made` (asyncapi.yaml) | env `MATCH_MADE_TOPIC` (Terraform / ConfigMap 経由) | マッチ成立時 (publish 成功 = acknowledge) | gateway (競合コンシューマとして全 Pod で subscribe) |

subscriber 列はこのリポジトリからは導けないので、変更時は gateway 側の購読状況も確認すること。publish 失敗時の再試行挙動と Exactly-Once の扱いは [FEATURE_SPEC.md](FEATURE_SPEC.md) 参照。
