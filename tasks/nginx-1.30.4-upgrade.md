# nginx 1.30.4 更新判定（#185）

## 目的

nginx 1.30.4 へ安全に更新できるかを確認し、可能な場合だけ 3 台へ反映する。

## 対象

- isucon1、isucon2、isucon3
- nginx のみ。MariaDB、OS の更新は対象外。

## 事前確認（2026-09-06）

| 確認項目 | isucon1 | isucon2 | isucon3 |
| --- | --- | --- | --- |
| OS | Ubuntu 20.04.4 (focal) | Ubuntu 20.04.4 (focal) | Ubuntu 20.04.4 (focal) |
| nginx 実行版 | 1.18.0 (Ubuntu) | 1.18.0 (Ubuntu) | 1.18.0 (Ubuntu) |
| APT候補 | 1.18.0-0ubuntu1.7 | 1.18.0-0ubuntu1.7 | 1.18.0-0ubuntu1.7 |
| 導入パッケージ | nginx/nginx-common/nginx-core | 同左 | 同左 |
| 設定検査 | 成功 | 成功 | 成功 |
| Git作業ツリー | clean | clean | clean |

nginx.org の現行 Linux package 対応表は Ubuntu 22.04、24.04、26.04 のみで、
Ubuntu 20.04 (focal) を含まない。従って、公式現行パッケージを Focal に追加して
1.30.4 へ更新する手順はサポート対象外である。

## 今回の判定

更新しない。未対応OSへのリポジトリ追加、別リリース向けdebの強制導入、ソースからの
置換は、競技中の可逆性と3台同一性を損なうため行わない。

## 将来の実施前提

1. OS を公式対応版（少なくとも Ubuntu 22.04）へ更新する作業を別Issue・別PRで完了する。
2. 各台の `/etc/nginx/nginx.conf`、`/etc/nginx/sites-available/isucondition.conf`、
有効モジュール、パッケージ一覧を更新前にGitへ原本保存済みであることを確認する。
3. nginx.org公式署名鍵の fingerprint を検証し、対象OS用の公式stable repositoryを追加する。
4. 各台を同一版へ更新し、`nginx -t`、reload、HTTPS、認可済みアイコン、ETagによる304を確認する。
5. ベンチマークの完走ログ・計測結果をRelease assetとして登録し、更新前後を比較する。

## ロールバック

更新後に設定検査、reload、HTTPS、アイコン配信、またはベンチで異常が出た場合は、
更新前に記録したUbuntu配布パッケージ版とGit保存済み設定へ戻す。OS更新とnginx更新を
同一変更に混在させないため、OSロールバックはOS更新Issueの手順に従う。

## 実行結果

2026-09-06 に3台で事前確認を実施した。Focal向けの公式現行パッケージが存在しないため、
パッケージ更新・設定変更・再読み込み・ベンチマークは実施していない。根拠は #185 と
#183 に記録する。
