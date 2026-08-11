# overload-party-matchmaking

マッチメイキングキューを管理する内部マイクロサービス。キューに入ったプレイヤーをペアリングし、`match_made` イベントを Cloud Pub/Sub に publish する。

エンドポイント・Pub/Sub 契約は [data/openapi.yaml](data/openapi.yaml) と [data/asyncapi.yaml](data/asyncapi.yaml) を参照。サービス構成全体の図は [common のシステム構成図](https://github.com/kenyamaneko/overload-party-common#システム構成図) を参照。環境変数は [docs/SETUP.md](docs/SETUP.md) を参照。

[テスト観点カタログ](https://kenyamaneko.github.io/overload-party-matchmaking/): テスト名から生成した、テスト済みの観点の一覧。
