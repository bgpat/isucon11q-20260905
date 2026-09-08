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
