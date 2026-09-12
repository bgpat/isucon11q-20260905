# ベンチマーク計測手順

この文書はこのリポジトリ固有の繰り返し手順である。本番と練習で手順を分けない。
共通ルールは `AGENTS.md` に従う。接続先・役割はユーザー提供の情報を正とする。

## 省略

ユーザーがその回だけ明示した項目だけ省略する。
点数比較やベースライン整合を理由にプロファイルやログを外さない。

## 保存先

成果物は `/tmp/isucon-work/` に置く。リポジトリには入れない。

```sh
sh ops/benchmark-assets.sh prepare
```

収集ディレクトリは `/tmp/isucon-work/current/`。比較時は baseline / candidate に退避する。

## アプリの反映

1. ローカルの worktree で変更し、リモートへ push する。
2. 対象の競技サーバーで fetch し、計測する commit を checkout する。
3. 各サーバーで `make -C / deploy-app` する。アプリの build と `isucondition.go` / Valkey の restart まで行う。nginx は reload しない。
4. nginx 設定を変えた場合は、確認したうえで別途 reload する。
5. デプロイ前後の Git commit と対象サーバーを記録する。

## 必須成果物

`ops/benchmark-report.py` が読むファイルを含む。無いものは理由を実測として記録し、推測値で埋めない。

| ファイル | 内容 |
| --- | --- |
| `bench.log` | ベンチマーカの標準出力 |
| `timing.json` | commit、LOAD 開始、終了、終了コード |
| `cpu.pprof` | LOAD 中の CPU profile |
| `slow.log.gz` | MariaDB slow query（計測区間） |
| `access.log.gz` | nginx access（計測区間） |
| `error.log` | nginx error（計測区間） |
| `journal.log` | `isucondition.go` の journal |
| `db-before.txt` / `db-after.txt` | MariaDB `SHOW GLOBAL STATUS` |
| `valkey-before.txt` / `valkey-after.txt` | `make -C / collect-valkey` |
| `prometheus-cpu.json` | process-exporter CPU rate |
| `prometheus-db.json` | MySQL queries rate |
| `state-before.txt` | HEAD、サービス状態、`ENABLE_PPROF`、slow log 変数 |

アプリを複数台で動かす場合は、各ホストの access / error / journal も取る。
isucon2 / isucon3 はファイル名にホスト名を付ける（例: `isucon2-access.log.gz`）。

## 事前

- `/etc/isucondition-observability.env` の `ENABLE_PPROF=1` を確認する。提出用計測で無効化する場合は、その回の省略指示として記録する。
- slow query log が FILE 出力であることを確認する。パスは `/var/log/mysql/mariadb-slow.log`。
- nginx ログは `/var/log/nginx/access.log` と `/var/log/nginx/error.log`。
- 各ホストで上記ログの現在サイズを記録し、ベンチ後に増分だけ取る。
- `db-before.txt`、`valkey-before.txt`、`state-before.txt` を取る。MariaDB の認証はサーバー上の env に従う。パスワードをこの文書やリポジトリに書かない。
- ベンチマーカ側で `jiaapi-mock` が競合する場合は、計測中だけ停止し、終了後に開始する。操作対象はユーザー確認済みの bench ホストとする。

## LOAD 中

LOAD 開始を検出したら、次を並行して始める。

```sh
curl -fsSk 'https://127.0.0.1/debug/pprof/profile?seconds=55'
make -C / reset-valkey-stats
```

CPU profile はアプリを受けているホスト（通常は isucon1）で取る。
`seconds=55` は 60 秒 LOAD に収めるため。LOAD 長が変わる場合は、LOAD 終了より短くする。

## 事後

- `db-after.txt` と `valkey-after.txt` を取る。Valkey は listen しているホストで、LOAD の外に取る。
- 事前に記録した offset から、各ホストの slow / access / error の増分を取る。slow と access は gzip する。
- 各ホストで `journalctl -u isucondition.go` をベンチ開始〜終了で取る。
- isucon1 の Prometheus `:9090` から LOAD 窓の `prometheus-cpu.json` と `prometheus-db.json` を取る。
- `timing.json` に exact commit、LOAD 開始、終了、終了コードを書く。
- 停止した `jiaapi-mock` を開始する。

## 分析と記録

外部投稿なしの集計:

```sh
python3 ops/benchmark-report.py --baseline /tmp/isucon-work/baseline --candidate /tmp/isucon-work/candidate --pr PR番号 --issue Issue番号 --tag TAG
```

公開するときだけ `--publish` を付ける。後処理の時間計測と Release 手順は `README.md` に従う。

ベンチ終了後は、チャットに分析結果を必ず出す。出す内容は `AGENTS.md` の計測ルールと性能分析の記録形式に従う。
`cpu.pprof` があるときは flat / cumulative の上位を出す。無いときは、取れなかった理由を実測として書き、関数別の内訳を補わない。
