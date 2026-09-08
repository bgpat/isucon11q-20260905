# 現行 nginx を残して 1.30.4 を別バイナリで入れる（#344 / #353）

## 目的

Ubuntu の nginx 1.18.0（`/usr/sbin/nginx`）は置換せず、
nginx 1.30.4 + OpenSSL 3.5.8 を `/opt/nginx-1.30.4` に用意する。
再起動後の既定は `nginx-custom.service` とする（#353）。

#185 の「ソースビルドで現行バイナリを置換しない」判断は維持する。

## 対象

- `/usr/sbin/nginx` と共有の `/etc/nginx/` サイト設定は残す。
- enable は `nginx-custom`、付属 `nginx.service` は disable。

## 前提条件

- 本 Issue の PR が対象サーバーへ `git pull` 済み。
- ビルド中は CPU を使う。ベンチ中には実行しない。

## 実行手順

1. `git pull --ff-only` し、`nginx-custom.service` と `multi-user.target.wants/nginx-custom.service` があることを確認する。
2. `make install-nginx-custom` を実行する。install 中は 443 を止めない。
3. 確認:
   - `/usr/sbin/nginx -v` が `nginx/1.18.0`
   - `/opt/nginx-1.30.4/sbin/nginx -V` に `1.30.4` と `OpenSSL 3.5.8` があり `--with-debug` と `enable-ktls` が無い
   - `systemctl is-enabled nginx-custom` が enabled
   - `systemctl is-enabled nginx` が disabled

## 確認方法

上記の版表示と enable 状態。再起動後は `nginx-custom` が 443 を持つ。

## 失敗時の復旧

`/usr/sbin/nginx` は触らない。途中の `/opt/nginx-1.30.4` は残っていてよい。
付属に戻すときは `systemctl disable nginx-custom.service` のあと `systemctl enable --now nginx.service`。

## 今すぐ 443 を切り替えるとき

```text
systemctl stop nginx.service
systemctl start nginx-custom.service
```

## 実行結果

実施済み（2026-09-08 09:40–09:43 JST、isucon1）。

isucon1 の HEAD は `perf/issue-325-condition-fetch-limit-master` の `e540dd1` のままにした。実験ブランチを切り替えず、`origin/infra/issue-344-nginx-custom` から install スクリプトと unit だけ取り出した。

| 確認項目 | 結果 |
| --- | --- |
| `/usr/sbin/nginx -v` | `nginx/1.18.0 (Ubuntu)` |
| カスタム `/opt/nginx-1.30.4/sbin/nginx -V` | `nginx/1.30.4`、`OpenSSL 3.5.8 25 Aug 2026`、`--with-debug` なし |
| カスタム `nginx -t` | 成功。`listen ... http2` は deprecated の warn のみ |
| `nginx.service` | active / enabled |
| `nginx-custom.service` | inactive / disabled |
| 443 | ビルド前と同じ PID（25884/111324/111325） |

`make install` が `/etc/nginx/` に `*.default` と `koi-utf` / `koi-win` / `win-utf` を追加した。`nginx.conf` 本体は上書きしていない。Git 作業ツリーは clean のまま。

443 の切り替えは未実施。

isucon2 / isucon3（2026-09-08 09:46–09:49 JST）も同じ手順で投入した。HEAD はいずれも `e540dd1` のまま。現行 1.18.0 は active、カスタム 1.30.4 / OpenSSL 3.5.8 は inactive / disabled。`nginx -t` は deprecated warn のみで成功。

isucon1 ベンチ（現行 1.18.0 のまま、LOAD 09:46:30–09:47:30）: score **7,741,656** pass（7,741,714 − 58、timeout 0）。ERR は unauth 200 が 24、trend 500 が 21、HTTP/2 force closed が 10、broken pipe が 3。mismatch は 0。

## kTLS 再ビルド（2026-09-08 21:00 頃）

3 台とも `/opt/nginx-1.30.4` を `enable-ktls`、`-O3 -march=native`、`--with-file-aio` で組み直した。`ssl_conf_command Options KTLS;` は `/etc/nginx-custom/nginx.conf` のみ。共有の `/etc/nginx/nginx.conf` には書いていない。443 は付属 1.18.0 のまま。`SSL_sendfile` あり。`nginx-custom` は inactive / disabled。

## kTLS なし再ビルド（2026-09-08 22:30 頃、isucon1）

`enable-ktls` / `ssl_conf_command` / `modprobe tls` を外して組み直した。`-O3 -march=native` と `--with-file-aio` は残した。`/opt/nginx-1.30.4/sbin/nginx -V` に `enable-ktls` と `--with-debug` は無い。`/etc/nginx-custom/nginx.conf` は `/etc/nginx/nginx.conf` のコピーで kTLS 行なし。443 は付属 1.18.0 のまま。

## 再起動後の既定（#353）

`multi-user.target.wants/nginx-custom.service` を Git 追跡する。install スクリプトは付属 `nginx.service` を disable し、独自を enable する。443 の即時切替はしない。3 台は 2026-09-09 00:54 に enable 済み。
