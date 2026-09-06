---
title: "MariaDB 10.3 から 11.4 LTS への更新"
needs-user-action: false
labels:
  - type:operations
depends_on:
  - "#186"
---

# 目的

MariaDB 10.3.39 を Ubuntu 20.04 向け MariaDB Community Server 11.4 LTS に更新する。
対象は isucon1, isucon2, isucon3 であり、必ず1台ずつ検証を終えてから次台へ進む。

# 根拠と前提

- MariaDB公式は Focal 向け11.4パッケージを提供する。
- 公式のメジャー更新手順は、全DBバックアップ、正常停止、
  `innodb_fast_shutdown != 2`、`innodb_force_recovery < 3`、
  option互換性確認、`mariadb-upgrade` を必須とする。
- 既存初期DBバックアップは Release `db-initial-20260906-0343` にある。
  ユーザーが追加バックアップ不要を明示承認した。これは現在データの復旧点ではない。
- MariaDB 11.4 のオプティマイザ変更により性能は不確定である。

# 対象と変更前確認

1. 対象各台が `10.3.39`、`innodb_fast_shutdown=1`、
   `innodb_force_recovery=0` であることを確認する。
2. `/etc/mysql/mariadb.conf.d/50-server.cnf` のSHA-256が各台で一致することを確認し、
   更新前原本をGit commit済みにする。
3. `/etc/apt/sources.list.d/mariadb-11.4.list` と
   `/etc/apt/keyrings/mariadb-keyring.asc` の不存在を各台で確認してから、
   このリポジトリに明示的に追加する。
4. この手順・PR・Issue・既存バックアップReleaseを確認する。

# 1台ごとの実行手順

1. ローカルでPRを作成・マージする。対象サーバーでは `/` で `git pull --ff-only` を行う。
2. `apt-get update` 後、`apt-cache policy` で MariaDB 11.4 を候補として確認する。
   11.4以外、署名検証失敗、または依存関係で削除対象が想定外の場合は停止する。
3. 稼働中のDBを確認し、`mysqladmin shutdown` で正常停止する。
4. APTで MariaDB server/client/core を11.4へ更新する。データディレクトリを削除しない。
5. DBを起動し、`mariadb-upgrade --force` を実行する。
6. `CHECK TABLE`、主要テーブル件数、MariaDB version、systemd状態、HTTPS health check を確認する。
7. isucon1単独更新後にアプリとベンチを実施し、ログをRelease assetへ登録する。
   失敗・性能悪化・整合性異常なら残り2台を更新せず、復旧する。
8. 改善または同等を確認した場合のみisucon2、isucon3を同じ手順で更新する。

# 停止条件

- 公式署名鍵・repository・候補版を検証できない。
- APTがMariaDB以外の重要パッケージ削除を提案する。
- DB停止/起動、`mariadb-upgrade`、整合性、アプリ疎通のいずれかが失敗する。
- ベンチがfail、deduction発生、または直前の比較可能なスコアを下回る。

# 復旧方法

MariaDBはダウングレードを設計上保証しない。障害時は残りサーバーを変更せず、
正常停止して11.4パッケージを削除、10.3パッケージを再導入する。必要なら
Release `db-initial-20260906-0343` の各台 `isucondition` ダンプを展開し、
既存 `/etc/mysql/debian.cnf` を使用して復元する。DB復旧による現在データ喪失を伴うため、
実行前にユーザーへ報告する。

# 作業後の記録

- 実行日時、対象台、commit、実パッケージ版、`mariadb-upgrade`結果
- 整合性・アプリ疎通・ベンチ結果、関連Release asset
- 成功/停止/復旧の判断と関連Issue・PR・Release
