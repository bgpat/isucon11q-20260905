#!/usr/bin/env bash
# Ubuntu の nginx は残し、1.30.4 + OpenSSL 3.5.8 を /opt に入れる（kTLS なし）。
# root で / 上の Git 作業ツリーから実行する。現行 nginx.service は止めない。
set -euo pipefail

NGINX_VERSION="1.30.4"
NGINX_ARCHIVE="nginx-${NGINX_VERSION}.tar.gz"
NGINX_SHA256="4261dc90e9e47c1c4041276e9aaa3d48ebe2e664f728e14fa95ae6c67d57a08b"
NGINX_URL="https://nginx.org/download/${NGINX_ARCHIVE}"

OPENSSL_VERSION="3.5.8"
OPENSSL_ARCHIVE="openssl-${OPENSSL_VERSION}.tar.gz"
OPENSSL_SHA256="a8f84a39918ec6415ce765d9b429d313ba97b8143169c172e734b9514464f5b2"
OPENSSL_URL="https://github.com/openssl/openssl/releases/download/openssl-${OPENSSL_VERSION}/${OPENSSL_ARCHIVE}"

NGINX_SBIN="/opt/nginx-${NGINX_VERSION}/sbin/nginx"
CUSTOM_CONF="/etc/nginx-custom/nginx.conf"
UNIT_PATH="/etc/systemd/system/nginx-custom.service"
UBUNTU_NGINX="/usr/sbin/nginx"

if [[ "${EUID}" -ne 0 ]]; then
  echo "root で実行してください" >&2
  exit 1
fi

if [[ ! -x "${UBUNTU_NGINX}" ]]; then
  echo "現行 nginx がありません: ${UBUNTU_NGINX}" >&2
  exit 1
fi

if [[ ! -f "${UNIT_PATH}" ]]; then
  echo "unit がありません: ${UNIT_PATH}" >&2
  echo "先に git pull して nginx-custom.service を入れてください" >&2
  exit 1
fi

install_build_deps() {
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
    build-essential pkg-config libpcre3-dev zlib1g-dev linux-libc-dev
}

install_binary() {
  local workdir nginx_src openssl_src jobs

  workdir="$(mktemp -d /tmp/nginx-custom-build.XXXXXX)"
  trap 'rm -rf "${workdir}"' EXIT

  curl --fail --location --proto '=https' --tlsv1.2 --output "${workdir}/${NGINX_ARCHIVE}" "${NGINX_URL}"
  printf '%s  %s\n' "${NGINX_SHA256}" "${workdir}/${NGINX_ARCHIVE}" | sha256sum --check --status
  tar -C "${workdir}" -xzf "${workdir}/${NGINX_ARCHIVE}"
  nginx_src="${workdir}/nginx-${NGINX_VERSION}"
  test -d "${nginx_src}"

  curl --fail --location --proto '=https' --tlsv1.2 --output "${workdir}/${OPENSSL_ARCHIVE}" "${OPENSSL_URL}"
  printf '%s  %s\n' "${OPENSSL_SHA256}" "${workdir}/${OPENSSL_ARCHIVE}" | sha256sum --check --status
  tar -C "${workdir}" -xzf "${workdir}/${OPENSSL_ARCHIVE}"
  openssl_src="${workdir}/openssl-${OPENSSL_VERSION}"
  test -d "${openssl_src}"

  mkdir -p "/opt/nginx-${NGINX_VERSION}/sbin"
  jobs="$(nproc)"
  (
    cd "${nginx_src}"
    ./configure \
      --prefix="/opt/nginx-${NGINX_VERSION}" \
      --sbin-path="${NGINX_SBIN}" \
      --modules-path="/opt/nginx-${NGINX_VERSION}/modules" \
      --conf-path="${CUSTOM_CONF}" \
      --error-log-path=/var/log/nginx/error.log \
      --http-log-path=/var/log/nginx/access.log \
      --pid-path=/run/nginx.pid \
      --lock-path=/var/lock/nginx-custom.lock \
      --http-client-body-temp-path=/var/lib/nginx/body \
      --http-fastcgi-temp-path=/var/lib/nginx/fastcgi \
      --http-proxy-temp-path=/var/lib/nginx/proxy \
      --http-scgi-temp-path=/var/lib/nginx/scgi \
      --http-uwsgi-temp-path=/var/lib/nginx/uwsgi \
      --with-compat \
      --with-pcre-jit \
      --with-file-aio \
      --with-http_ssl_module \
      --with-http_v2_module \
      --with-http_realip_module \
      --with-http_stub_status_module \
      --with-http_gzip_static_module \
      --with-http_gunzip_module \
      --with-http_auth_request_module \
      --with-threads \
      --with-openssl="${openssl_src}" \
      --with-openssl-opt='enable-tls1_3 enable-ec_nistp_64_gcc_128 no-ssl3 no-tls1 no-tls1_1 no-dtls no-comp no-idea no-mdc2 no-rc5 no-weak-ssl-ciphers no-tests' \
      --with-cc-opt='-O3 -march=native -mtune=native' \
      --with-ld-opt='-Wl,-O1'
    nice -n 10 make -j"${jobs}"
    nice -n 10 make install
  )

  test -x "${NGINX_SBIN}"
  "${NGINX_SBIN}" -v 2>&1 | grep -F "nginx/${NGINX_VERSION}" >/dev/null
  "${NGINX_SBIN}" -V 2>&1 | grep -F "OpenSSL ${OPENSSL_VERSION}" >/dev/null
  if "${NGINX_SBIN}" -V 2>&1 | grep -F enable-ktls >/dev/null; then
    echo "カスタム nginx に enable-ktls が入っている" >&2
    exit 1
  fi
  if "${NGINX_SBIN}" -V 2>&1 | grep -F -- '--with-debug' >/dev/null; then
    echo "カスタム nginx に --with-debug が入っている" >&2
    exit 1
  fi

  trap - EXIT
  rm -rf "${workdir}"
}

install_custom_config() {
  mkdir -p /etc/nginx-custom
  python3 - "${CUSTOM_CONF}" <<'PY'
from pathlib import Path
import sys

dst = Path(sys.argv[1])
src = Path("/etc/nginx/nginx.conf").read_text()
src = src.replace("    ssl_conf_command Options KTLS;\n", "")
if "ssl_conf_command" in src:
    raise SystemExit("ssl_conf_command が残っている")
dst.write_text(src)
PY
  if grep -F "ssl_conf_command" /etc/nginx/nginx.conf >/dev/null; then
    echo "/etc/nginx/nginx.conf に ssl_conf_command がある" >&2
    exit 1
  fi
}

verify_ubuntu_nginx_untouched() {
  local pkg_path

  pkg_path="$(dpkg -L nginx-core | awk '$0=="/usr/sbin/nginx"{print; exit}')"
  if [[ "${pkg_path}" != "${UBUNTU_NGINX}" ]]; then
    echo "nginx-core の /usr/sbin/nginx が見つかりません" >&2
    exit 1
  fi
  "${UBUNTU_NGINX}" -v 2>&1 | grep -F 'nginx/1.18.0' >/dev/null
  "${UBUNTU_NGINX}" -t -q
  if [[ "$(readlink -f "${UBUNTU_NGINX}")" == "$(readlink -f "${NGINX_SBIN}")" ]]; then
    echo "現行 nginx がカスタムバイナリで上書きされています" >&2
    exit 1
  fi
}

reload_unit_without_starting() {
  systemctl daemon-reload
  if systemctl is-enabled nginx-custom.service >/dev/null 2>&1; then
    systemctl disable nginx-custom.service
  fi
  systemctl is-active --quiet nginx.service
}

test_custom_config() {
  "${NGINX_SBIN}" -t -q -c "${CUSTOM_CONF}"
}

install_build_deps
install_binary
install_custom_config
verify_ubuntu_nginx_untouched
reload_unit_without_starting
test_custom_config

echo "カスタム nginx を入れました: $("${NGINX_SBIN}" -v 2>&1)"
echo "現行 nginx は残しています: $("${UBUNTU_NGINX}" -v 2>&1)"
echo "設定: ${CUSTOM_CONF}（kTLS なし）"
echo "nginx.service は active のままです。nginx-custom は起動していません。"
echo "切り替えるとき（このスクリプトでは実行しない）:"
echo "  systemctl stop nginx.service"
echo "  systemctl start nginx-custom.service"
echo "戻すとき:"
echo "  systemctl stop nginx-custom.service"
echo "  systemctl start nginx.service"
