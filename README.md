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

### run monitoring servers

```bash
make monitoring
```

Access to 3999 port from your browser.

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

各処理をローカルの `time` サブコマンドで囲み、開始・終了・経過秒・終了コードを記録する。
出力は固定名 `/tmp/isucon-work/timing.tsv` で、コマンド内容や認証情報は記録しない。
既存記録は追記で残す。新しい比較単位では、既存TSVを計測成果物として退避してから開始する。
`ISUCON_WORK_DIR` で保存ディレクトリを変更できる。`prepare` は timing.tsv を消さない。

```sh
sh ops/benchmark-assets.sh time deploy-isucon1 -- ssh root@isucon1 'make deploy-app'
sh ops/benchmark-assets.sh time archive -- sh ops/benchmark-assets.sh archive candidate
sh ops/benchmark-assets.sh time release-upload -- gh release upload TAG /tmp/isucon-work/candidate-assets.tar.gz
sh ops/benchmark-assets.sh timing-report
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
