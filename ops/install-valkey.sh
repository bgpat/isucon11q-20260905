#!/usr/bin/env bash
# Valkey を公式ソースから固定版で入れる。root で / 上の Git 作業ツリーから実行する。
# バイナリを入れたホストでは systemd の enable/start も行う。bind は設定ファイルの 0.0.0.0 のまま。
set -euo pipefail

VALKEY_VERSION="9.0.6"
VALKEY_ARCHIVE="valkey-${VALKEY_VERSION}.tar.gz"
VALKEY_SHA256="38f1e83faf382637d6e49503903187acb3ceed624e96d7b1d32b8d3d52768414"
VALKEY_URL="https://github.com/valkey-io/valkey/archive/refs/tags/${VALKEY_VERSION}.tar.gz"
VALKEY_PREFIX="/usr/local"
VALKEY_CONF="/etc/valkey/valkey.conf"
VALKEY_DATA="/var/lib/valkey"
VALKEY_USER="valkey"

if [[ "${EUID}" -ne 0 ]]; then
  echo "root で実行してください" >&2
  exit 1
fi

install_build_deps() {
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq build-essential pkg-config
}

ensure_user() {
  if ! id -u "${VALKEY_USER}" >/dev/null 2>&1; then
    useradd --system --home "${VALKEY_DATA}" --shell /usr/sbin/nologin "${VALKEY_USER}"
  fi
  mkdir -p "${VALKEY_DATA}"
  chown "${VALKEY_USER}:${VALKEY_USER}" "${VALKEY_DATA}"
}

install_binary() {
  local workdir extracted

  workdir="$(mktemp -d /tmp/valkey-install.XXXXXX)"
  trap 'rm -rf "${workdir}"' EXIT

  curl --fail --location --proto '=https' --tlsv1.2 --output "${workdir}/${VALKEY_ARCHIVE}" "${VALKEY_URL}"
  printf '%s  %s\n' "${VALKEY_SHA256}" "${workdir}/${VALKEY_ARCHIVE}" | sha256sum --check --status
  tar -C "${workdir}" -xzf "${workdir}/${VALKEY_ARCHIVE}"
  extracted="${workdir}/valkey-${VALKEY_VERSION}"
  test -d "${extracted}"

  make -C "${extracted}" -j"$(nproc)" BUILD_TLS=no
  install -m 0755 "${extracted}/src/valkey-server" "${VALKEY_PREFIX}/bin/valkey-server"
  install -m 0755 "${extracted}/src/valkey-cli" "${VALKEY_PREFIX}/bin/valkey-cli"
  install -m 0755 "${extracted}/src/valkey-benchmark" "${VALKEY_PREFIX}/bin/valkey-benchmark" || true

  "${VALKEY_PREFIX}/bin/valkey-server" --version | grep -F "v=${VALKEY_VERSION}" >/dev/null

  trap - EXIT
  rm -rf "${workdir}"
}

install_config() {
  mkdir -p /etc/valkey
  if [[ -f /etc/valkey/valkey.conf ]] && [[ ! -f /etc/valkey/valkey.conf.bak ]]; then
    cp -a /etc/valkey/valkey.conf /etc/valkey/valkey.conf.bak
  fi
  if [[ ! -f "${VALKEY_CONF}" ]]; then
    echo "設定ファイルがありません: ${VALKEY_CONF}" >&2
    exit 1
  fi
  chown root:root "${VALKEY_CONF}"
  chmod 0644 "${VALKEY_CONF}"
}

install_unit() {
  if [[ -f /etc/systemd/system/valkey.service ]]; then
    :
  elif [[ -f /files/systemd/valkey.service ]]; then
    cp -f /files/systemd/valkey.service /etc/systemd/system/valkey.service
  else
    echo "unit がありません: /etc/systemd/system/valkey.service" >&2
    exit 1
  fi
  systemctl daemon-reload
}

configure_service() {
  systemctl enable valkey.service
  systemctl restart valkey.service
  local i
  for i in 1 2 3 4 5 6 7 8 9 10; do
    if "${VALKEY_PREFIX}/bin/valkey-cli" -h 127.0.0.1 -p 6379 ping 2>/dev/null | grep -qx PONG; then
      break
    fi
    sleep 0.5
  done
  systemctl is-active --quiet valkey.service
  "${VALKEY_PREFIX}/bin/valkey-cli" -h 127.0.0.1 -p 6379 ping | grep -qx PONG
  echo "Valkey を起動しました ($(hostname -s)): $("${VALKEY_PREFIX}/bin/valkey-server" --version)"
}

install_build_deps
ensure_user
install_binary
install_config
install_unit
configure_service
