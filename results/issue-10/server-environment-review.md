# Issue #10 調査結果: サーバー構成とマシンスペック

実行日時: 2026-09-06 (Asia/Tokyo)

確認した commit: `5f2fd22edff3e95c9e98cd350892b4a403c88081`

## 結論

サーバーへの SSH 接続先、サーバー役割、hostname、OS、CPU、メモリ、ディスク、ネットワーク、listen port、実行サービスの実測値は、ローカル追跡ファイルと既存 `results/` にありません。ユーザー提供の接続先もこの作業範囲にはありません。よって数値・役割・構成は記録しません。

## ローカルで確認できた運用定義

- README は各競技サーバー上で `/` を Git 管理し、初回セットアップ時に remote を設定して `make` を実行する手順を示しています。
- `Makefile` は `hostname` を使って `/files/hosts/$(HOSTNAME)` と `/files/pubkey/$(HOSTNAME)` を扱います (`Makefile:1,80-83,120-124`)。
- `Makefile` の監視対象は Prometheus、node_exporter、process-exporter、Grafana です (`Makefile:126-169`)。
- Prometheus の scrape 対象ファイルは `/etc/prometheus/hosts.json` ですが、その内容はローカルにはありません (`etc/prometheus/prometheus.yml:4-8`)。
- 実機の OS、ランタイム、ミドルウェア、アプリ配置、DB 配置、ネットワークは未確認です。

## 未実測項目

`results/env/` 自体が存在しないため、次の項目は未取得です。

- サーバーごとの hostname、役割、接続先
- OS / kernel
- CPU model、socket/core/thread、CPU 数
- メモリ、ディスク、ネットワーク、ルーティング
- nginx / MySQL または MariaDB / 言語ランタイムのバージョン
- listen port、systemd サービス、プロセス、`/etc/hosts`

## 判定

ローカルだけで完了できる確認は、追跡済みのセットアップ定義の棚卸しまでです。実測成果物は生成していません。サーバー接続情報と実測許可が提供された後に `tasks/check-environment.md` の手順で取得する必要があります。

アプリ・設定・サーバー状態は変更していません。
