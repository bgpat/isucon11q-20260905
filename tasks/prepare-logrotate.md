---
title: "ログローテーションを用意する"
labels:
  - phase:early-competition
  - type:observability
  - type:task
depends_on:
  - prepare-observability.md
---

# 目的

計測のたびにログを切り出し、前回結果と混ざらないようにする。

# 前提条件

- ログパスは `prepare-observability.md` で確認した値を使う。
- 手順は Makefile などリポジトリ上のコマンドにまとめる。

# 対象サーバー

アクセスログ、スロークエリ、アプリログがあるサーバー。

# 作業

- アクセスログ、スロークエリログ、アプリケーションログのパスを確認する。
- ベンチマーク前後で切る対象と、退避先を決める。
- Makefile など、毎回同じコマンドで実行できる手順にする。
- 切り出したログを集計や保存に渡せることを確認する。

## 実行例

コマンドは実行例である。問題や環境によって構成、パス、サービス名、接続方法は異なる。Issue に記録したユーザー提供情報と実機の状態に合わせて読み替える。

```bash
SERVERS=(s1 s2 s3)
```

## 1. 切る対象を確定する

```bash
for s in "${SERVERS[@]}"; do
  ssh "$s" 'hostname
    ls -l /var/log/nginx/access.log /var/log/nginx/error.log /var/log/mysql/mysql-slow.log /var/log/mysql/slow.log 2>/dev/null || true'
done
```

## 2. ローカルに毎回同じコマンドを書く

退避先は `/var/log/archive/` など、Git 管理外にする。パスは実機で存在確認したログだけにする。

```makefile
STAMP := $(shell date +%Y%m%d-%H%M%S)
LOG_ARCHIVE := /var/log/archive/$(STAMP)

.PHONY: rotate-logs
rotate-logs:
	mkdir -p $(LOG_ARCHIVE)
	-test -f /var/log/nginx/access.log && mv /var/log/nginx/access.log $(LOG_ARCHIVE)/
	-nginx -s reopen
	-test -f /var/log/mysql/mysql-slow.log && mv /var/log/mysql/mysql-slow.log $(LOG_ARCHIVE)/
	-mysqladmin flush-logs
	ls -l $(LOG_ARCHIVE)
```

## 3. 一度実行して切り出せることを確認する

```bash
# ローカルで commit したあと
for s in "${SERVERS[@]}"; do
  ssh "$s" 'cd / && git pull origin master && make rotate-logs'
done
```

Makefile 反映前の手動確認。

```bash
ssh s1 'bash -s' <<'EOF'
set -eu
STAMP=$(date +%Y%m%d-%H%M%S)
DEST=/var/log/archive/$STAMP
mkdir -p "$DEST"
test -f /var/log/nginx/access.log && mv /var/log/nginx/access.log "$DEST/"
nginx -s reopen
ls -l "$DEST" /var/log/nginx/access.log
EOF
```

# 確認方法

```bash
ssh s1 'ls -lt /var/log/archive | head; sudo tail -n 1 /var/log/nginx/access.log || true'
```

新しいログファイルが空から書き始まること。

# 失敗時の復旧

- nginx がログを開けない: `nginx -s reopen` とディレクトリ権限を確認する。`/var/log/nginx` の所有者を変えてしまったら戻す。
- slow log が切られない: `slow_query_log_file` の実パスを `SHOW VARIABLES` で確認する。

# 完了条件

- 計測ごとにログを分けて保存できる。
- 手順がリポジトリまたは Issue に残っている。

# 作業後の記録

- 実行日時:
- 対象ログ:
- 退避先:
