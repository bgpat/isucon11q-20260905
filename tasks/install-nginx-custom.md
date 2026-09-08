# 現行 nginx を残して 1.30.4 を別バイナリで入れる（#344）

## 目的

Ubuntu の nginx 1.18.0（`/usr/sbin/nginx`、`nginx.service`）は置換せず、
nginx 1.30.4 + OpenSSL 3.5.8 を `/opt/nginx-1.30.4` に用意する。

#185 の「ソースビルドで現行バイナリを置換しない」判断は維持する。
Unix socket 化（#204）は対象外。443 への切り替えとベンチは、投入確認後に別途行う。

## 対象

- 初回は isucon1 のみ。isucon2/3 では実行しない。
- 変更しないもの: `/usr/sbin/nginx`、`/etc/nginx/` の設定、`nginx.service` の enable 状態。

## 前提条件

- 本 Issue の PR が対象サーバーへ `git pull` 済み。
- `nginx.service` が active。
- ビルド中は CPU を使う。ベンチ中には実行しない。

## 実行手順

1. isucon1 で `git pull --ff-only` し、`/etc/systemd/system/nginx-custom.service` があることを確認する。
2. `make install-nginx-custom` を実行する。現行 `nginx.service` は止めない。
3. 確認:
   - `/usr/sbin/nginx -v` が `nginx/1.18.0`
   - `/opt/nginx-1.30.4/sbin/nginx -V` に `1.30.4` と `OpenSSL 3.5.8` があり `--with-debug` が無い
   - `/opt/nginx-1.30.4/sbin/nginx -t` が成功
   - `systemctl is-active nginx` が active
   - `systemctl is-enabled nginx-custom` が disabled
   - `ss -lntp` の 443 が従来の nginx PID のまま

## 確認方法

上記の版表示、設定検査、`nginx.service` が active、`nginx-custom` が inactive / disabled。

## 失敗時の復旧

`/usr/sbin/nginx` は触らない。失敗しても現行 nginx を止めなければ HTTPS は維持される。
途中の `/opt/nginx-1.30.4` は残っていてよい。やり直すときは同じ `make install-nginx-custom`。
誤って `nginx-custom` を起動した場合は `systemctl stop nginx-custom.service` のあと `systemctl start nginx.service`。

## 切り替え（このタスクでは実行しない）

```text
systemctl stop nginx.service
systemctl start nginx-custom.service
```

戻す:

```text
systemctl stop nginx-custom.service
systemctl start nginx.service
```

`nginx-custom.service` は enable しない。サーバー再起動後は現行 Ubuntu nginx が起動する。

## 実行結果

未実施。
