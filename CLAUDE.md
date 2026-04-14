# CLAUDE.md - overload-party-matchmaking

## 行動制約

- エラーは握りつぶさない
- git tag を手動で打たない（CI が自動作成する）
- TODO スタブを追加しない
- クライアント認証を行わない（ClusterIP のみ、gateway が唯一の呼び出し元）
- インメモリキューのフォールバックを再導入しない（Redis に到達できなければ 503）
- publish 成功前にマッチを acknowledge しない（失敗時は元の JoinedAt でキューに再投入）
- `data/models.yaml` 変更時は `python3 scripts/generate_types.py` を実行する
- 秘匿情報は env に直接注入せず Secret Manager から実行時取得する
- 未設定・未知の値は起動時エラー（暗黙フォールバック禁止）
