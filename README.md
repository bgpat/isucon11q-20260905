# isucon11q-20260905

## usage

### initialize

#### from local PC

**create a new repository**

- [x] Create a new repository from https://github.com/dbgofy/isucon-boilerplate/generate.

**add members**

- [x] Invite your team members from https://github.com/bgpat/isucon11q-20260905/settings/access.
  - Run [Update SSH Keys](https://github.com/bgpat/isucon11q-20260905/actions/workflows/update-sshkeys.yml).

#### in each competition server 

Run following commands by `root` user.

**generate and register ssh deploy key**

```bash
ssh-keygen -t rsa
cat ~/.ssh/id_rsa.pub
```

Copy the result and register as a [deploy key](https://github.com/bgpat/isucon11q-20260905/settings/keys/new).

**install**

```bash
cd /
git init
git remote add origin git@github.com:bgpat/isucon11q-20260905.git
git fetch origin master
git reset --hard FETCH_HEAD
make
```

Add deploy key and run `git push -u origin master`.

インスタンスを作り直したあとも、未初期化（`/.git` なし）なら上記と同じ手順で `/` を Git 管理へ戻す。詳細と実施結果は `tasks/initialize-servers.md`。

### 現在の競技サーバー（2026-09-10 ポータル出力に合わせて役割を修正）

接続先はユーザー提供の Public IP とローカル SSH 設定を正とする。IP は作り直しで変わる。

| Host | Public IP | hostname | 役割 |
| --- | --- | --- | --- |
| isucon1 | 18.177.215.25 | ip-192-168-0-11 | アプリ / MariaDB / Valkey |
| isucon2 | 3.113.46.84 | ip-192-168-0-12 | アプリ / MariaDB |
| isucon3 | 13.113.66.178 | ip-192-168-0-13 | アプリ / MariaDB |
| bench | 54.92.69.228 | ip-192-168-0-10 | ベンチマーカ / JIA mock |

- SSH Host は `isucon1`〜`isucon3` と `bench`。競技サーバーの初回セットアップ時だけ `isucon`、初期化後は `root`。IdentityFile はリポジトリ外の `~/.ssh/id_ed25519`
- 競技サーバー（isucon1〜3）の `/` が Git 作業ツリー。以後の変更はローカルで行い、サーバーでは `git pull` する
- `bench` は競技アプリを動かさない。#371 で isucon3 と誤認して初期化したため、#379 でアプリと nginx-custom を止めた
- Go 1.27.1（`make upgrade-go`）。Valkey 9.0.6（`make install-valkey`）。Valkey の起動と enable は isucon1 のみ
- DB 初期データは Git に入れず、Release [db-initial-20260906-0343](https://github.com/bgpat/isucon11q-20260905/releases/tag/db-initial-20260906-0343) に保存済み（#9）。作り直し後もこの Release を使い、同じ初期データを再取得しない
- 再セットアップ: #305 #362 #371 #377。役割修正: #379。接続設定: #19。ランタイム: #38。文書化: #341

### run monitoring servers

```bash
make monitoring
```

Access to 3999 port from your browser.

Grafana OSS 13.2.1 と Tempo 2.6.1 も同じ `make monitoring` で nohup 起動する。systemd には入れないので、サーバー再起動後は自動では立ち上がらない。Tempo は全競技サーバーが同一クラスタ（distributor + ingester + querier + query-frontend）に参加する。memberlist と ingester アドレスは `/files/hosts/*`（各サーバーの hostname と IP）から `/ops/render-tempo-config.sh` が生成する。Grafana はどのサーバーで起動しても Explore の Tempo（ローカル `:3200`）から ring 全体の trace をまとめて見られる。アプリは `ENABLE_TRACING=1`（`/etc/isucondition-observability.env`）のとき OTLP（`OTEL_EXPORTER_OTLP_*`）でローカル distributor へ送る。提出用計測では `ENABLE_TRACING=0` にしてアプリを再起動する（#12）。

### edit config file

```bash
vim /etc/nginx/nginx.conf
```

### track source files by git

```bash
cd /home/isucon
git add -f *.go
```

### デプロイ・計測・後処理の所要時間

保存済み baseline/candidate の後処理は使い捨てスクリプトを作らず、次の定型コマンドを使う。
各入力は `bench.log`、`timing.json`（commit/load/end）、`slow.log.gz`、`cpu.pprof`、
`prometheus-cpu.json`、`db-before.txt`、`db-after.txt` を含む収集ディレクトリ。

```sh
sh ops/benchmark-assets.sh time report -- python3 ops/benchmark-report.py --baseline /tmp/isucon-work/baseline --candidate /tmp/isucon-work/candidate --pr 233 --issue 226 --tag benchmark-TIMESTAMP-score-SCORE
```

外部投稿なしで `/tmp/isucon-work/report/` に `report.md`、`metrics.json`、`assets.tar.gz`、
`processing.json` を生成する。入力ディレクトリは変更しない。出力は固定名で更新する。
SQL/pprof/CPU を並列集計し、SQL/pprof top20、CPU・DB差分、比較commitと窓を自動記録する。
SQL は LOAD 次整数秒から59秒（実際の終了秒まで）の同長窓で比較する。
CPU は instance ごとに区別し、移動rateの積分による推定値として表示する。
`processing.json` に並列集計全体・本文生成・圧縮の経過秒、既存 `timing.tsv` に全体とpprofを記録する。
ベンチのLOAD END/sendResult完走と最終scoreの算術整合を検査する。CPU系列はLOAD窓±1秒を
超える場合に停止し、本文にも保存範囲を表示する。pprofは時刻・期間・総CPU sampleを掲載する。
機械集計は仮説・採否を決めない。レビュー後 `--hypothesis '仮説' --decision '判断と根拠'` を指定する。
`--publish` を付けた場合だけ計測commitをtargetにRelease作成とasset添付、PR/Issueコメントを行う。
既存Releaseを上書きせずエラーで停止する。公開が途中失敗した場合は作成済みReleaseを確認し、
未完了のupload/commentのみ再実行する。再公開のためにタグを変えて同一計測を重複登録しない。
公開前は `processing.json` のstatusがprepared、Releaseとコメント完了後だけcompleteとなる。

承認画面で毎回異なるphase名がコマンド先頭に現れないよう、上記の外側timeラッパーは省略して
`python3 ops/benchmark-report.py ...` を固定entrypointとして実行してもよい。処理別の内部計測は残る。
公開時の許可は、この固定スクリプトの内容をレビューしたうえでスクリプトまでのprefixに対して
申請できる。これは自動許可を保証せず、内部コマンドにも実行環境の承認規則が適用される。

```sh
python3 -m unittest discover -s ops -p test_benchmark_report.py
```

各処理をローカルの `time` サブコマンドで囲み、開始・終了・経過秒・終了コードを記録する。
出力は固定名 `/tmp/isucon-work/timing.tsv` で、コマンド内容や認証情報は記録しない。
既存記録は追記で残す。新しい比較単位では、既存TSVを計測成果物として退避してから開始する。
`ISUCON_WORK_DIR` で保存ディレクトリを変更できる。`prepare` は timing.tsv を消さない。

```sh
sh ops/benchmark-assets.sh time deploy-isucon1 -- ssh root@isucon1 'make -C / deploy-app'
sh ops/benchmark-assets.sh time archive -- sh ops/benchmark-assets.sh archive candidate
sh ops/benchmark-assets.sh time release-upload -- gh release upload TAG /tmp/isucon-work/candidate-assets.tar.gz
sh ops/benchmark-assets.sh timing-report
```

`timing.tsv` は `current/` の外にあるため、`archive` のアーカイブには含まれない。
後処理完了後に別のRelease assetとして添付する。次の計測を始める前に添付を終える。

```sh
gh release upload TAG /tmp/isucon-work/timing.tsv
```

同期、デプロイ確認、ベンチ、profile取得、ログ取得・圧縮、SQL/pprof分析、Release作成・upload、
PR/Issue記録、merge、master同期を別の処理名で計測する。独立した処理は同じラッパーで並列実行できる。
既存の後処理スクリプト全体を `time postprocess -- sh ...` で囲むと、その全体経過も記録できる。

一覧は各処理の開始・終了をUTCで表示する。経過はLinux `/proc/uptime` の単調増加値を使い、
壁時計の時刻補正とは分ける。0.01秒単位で、並列・親子区間が重なる時間は合計から分離する。
全体経過は最初の開始から最後の終了、区間外はその範囲内でどの処理も計測していない時間である。
ツール起動前の承認待ちはラッパーから観測できず、区間外時間を承認待ちとは断定しない。
コマンド内部の待機は経過時間に含む。強制終了などで記録が残らなかった処理は集計できない。
同じTSVへ別ホストや再起動前後の記録を混在させない。ログ・レポートはGitへ追加しない。
