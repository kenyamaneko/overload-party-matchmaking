# overload-party-matchmaking

カードゲーム Overload Party のマッチメイキングキュー管理を担うマイクロサービス。

## 技術スタック

| レイヤー | 技術 |
|---|---|
| 言語 | Go |
| フレームワーク | Gin |
| データストア | Upstash Redis |
| シークレット管理 | Secret Manager |
| 同期通信 | REST |
| 非同期通信 | Cloud Pub/Sub |

## ドキュメント

| ドキュメント | 内容 |
|---|---|
| [セットアップ](docs/SETUP.md) | 環境変数の設定 |
| [API仕様書](data/openapi.yaml) | REST API のエンドポイント定義 |
| [Pub/Sub仕様書](data/asyncapi.yaml) | Pub/Sub イベントの定義 |
| [データ設計書](docs/DATA_DESIGN.md) | Redis キーとメッセージフォーマットの定義 |
| [ADR](https://github.com/kenyamaneko/overload-party-common/tree/main/docs/adr)（commonリポジトリ） | 設計判断の背景・理由・結果 |
| [システム構成図](https://github.com/kenyamaneko/overload-party-common#システム構成図)（commonリポジトリ） | Overload Party 全体の構成図 |
| [テスト観点カタログ](https://kenyamaneko.github.io/overload-party-matchmaking/) | テスト名から自動生成した、テスト済みの観点一覧 |
