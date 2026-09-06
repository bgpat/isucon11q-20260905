---
title: "MariaDB 10.3 から 11.4 LTS への更新"
needs-user-action: false
labels:
  - type:operations
depends_on:
  - "#186"
---

# 目的

MariaDB 10.3.39 を MariaDB Community Server 11.4 LTS に更新できるか検証する。
2026-09-06時点でUbuntu 20.04/Focal向けの公式11.4 APT repositoryは利用できないことを確認したため、
このOSのまま更新は実施しない。

# 根拠と前提

- MariaDB公式 `mariadb_repo_setup --mariadb-server-version=mariadb-11.4
  --os-type=ubuntu --os-version=focal --write-to-stdout` は、11.4がFocalでは
  利用不可であると明示して終了した。
- 実機で一時的に設定した公式repositoryに対する `apt-get update` も
  `focal Release 404` で失敗した。設定はGit経由で削除する。
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
3. この手順・PR・Issue・既存バックアップReleaseを確認する。

# 1台ごとの実行手順

OSをUbuntu 22.04以降など、公式11.4 APT repositoryが対応する版へ安全に更新し、
そのOS更新のバックアップ・復旧・競技規約確認を完了した後にのみ、上記のメジャー更新を
再計画する。現行20.04上ではこの手順を実行しない。

# 停止条件

- 公式setup scriptが対象OS/版のrepositoryを生成できない。
- APTがMariaDB以外の重要パッケージ削除を提案する。
- DB停止/起動、`mariadb-upgrade`、整合性、アプリ疎通のいずれかが失敗する。
- ベンチがfail、deduction発生、または直前の比較可能なスコアを下回る。

# 復旧方法

MariaDBはダウングレードを設計上保証しない。障害時は残りサーバーを変更せず、
正常停止して11.4パッケージを削除、10.3パッケージを再導入する。必要なら
Release `db-initial-20260906-0343` の各台 `isucondition` ダンプを展開し、
既存 `/etc/mysql/debian.cnf` を使用して復元する。DB復旧による現在データ喪失を伴うため、
実行前にユーザーへ報告する。

# 実行結果（2026-09-06）

- 対象: isucon1のみ（更新前確認・repository検証）。isucon2/3は未変更。
- DB停止、パッケージ更新、`mariadb-upgrade`、データ変更は実行していない。
- 停止理由: MariaDB公式setup scriptがFocalの11.4を利用不可と判定し、APT repositoryも
  Release 404となったため。
- APT repositoryの追加はGit経由で撤回し、isucon1を正常なUbuntu APT状態へ戻す。

# 作業後の記録

- 実行日時、対象台、commit、実パッケージ版、`mariadb-upgrade`結果
- 整合性・アプリ疎通・ベンチ結果、関連Release asset
- 成功/停止/復旧の判断と関連Issue・PR・Release
