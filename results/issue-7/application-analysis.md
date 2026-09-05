# Issue #7 調査結果: アプリケーション構成

実行日時: 2026-09-06 (Asia/Tokyo)

確認した commit: `5f2fd22edff3e95c9e98cd350892b4a403c88081`

## 結論

このローカル作業ツリーには競技アプリケーションのソース、DB 定義、実機取得結果がありません。したがって、エントリポイント、ルーティング、リクエスト経路、DB/API 呼び出し、認証・セッション、状態遷移は確認できません。公開解答や未確認の構成は補っていません。

確認できたのは、競技サーバーへ Git で同期し、`make monitoring` で監視を起動する運用リポジトリの構成です。

## 確認できた事実

- `git ls-files` は 48 ファイルで、`home/isucon`、`webapp`、`app/` 配下のアプリケーションソースはありません。
- `Makefile` は `hostname` に基づくホスト固有ファイル、SSH、Git、Prometheus、node_exporter、process-exporter、Grafana のセットアップを定義しています。
- `make monitoring` は Prometheus、node_exporter、process-exporter、Grafana を依存先として起動します (`Makefile:126-127`)。
- Prometheus は `/etc/prometheus/hosts.json` を file service discovery として参照し、scrape/evaluation interval は 1 秒です (`etc/prometheus/prometheus.yml:1-8`)。
- Grafana は Makefile 上で HTTP ポート 3999、匿名 Admin を指定して起動されます (`Makefile:159-162`)。
- アプリケーションのプロセス、プロキシ、DB、キャッシュ、キュー、外部 API、認証・セッション、状態エンティティは、ローカル追跡ファイルからは確認できません。

## 図示範囲

`application-architecture.json` / HTML は、根拠がある運用リポジトリ、競技サーバー、監視プロセスだけを含めます。未確認のアプリケーションをノードとして追加していません。

Archify の workflow / sequence / dataflow / lifecycle は、根拠となるアプリソースや実測がないため省略します。

Archify validate は 9/9 checks、composition は pass (errors 0 / warnings 0) でした。visual-check は自動ブラウザ検証 pass で、1440×900 light のスクリーンショットを目視確認しました。visual-review の対象はこの運用構成図に限ります。

## 追加確認が必要な項目

- `initialize-servers.md` 相当の実施記録と、競技サーバー上で Git 管理されたアプリソース
- `check-environment.md` の実測結果 (`results/env/`)
- 実行中のプロキシ、アプリ、DB、外部連携、代表リクエスト

## 変更していないもの

アプリケーションコード、設定ファイル、Makefile、Prometheus 設定は変更していません。
