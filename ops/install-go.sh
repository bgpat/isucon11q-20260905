#!/usr/bin/env bash
# Go の公式配布物を検証して更新する。root で / 上の Git 作業ツリーから実行する。
set -euo pipefail

GO_VERSION="1.27.1"
GO_OS_ARCH="linux-amd64"
GO_ARCHIVE="go${GO_VERSION}.${GO_OS_ARCH}.tar.gz"
GO_SHA256="63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445"
GO_URL="https://go.dev/dl/${GO_ARCHIVE}"
GO_ROOT="/home/isucon/local/go"

if [[ "${EUID}" -ne 0 ]]; then
  echo "root で実行してください" >&2
  exit 1
fi

rollback() {
  local backup="$1"
  local stamp failed

  if [[ ! -d "${backup}" || "$(basename "${backup}")" != go.backup-* ]]; then
    echo "退避済みGoディレクトリを指定してください: ${backup}" >&2
    exit 1
  fi
  if [[ ! -d "${GO_ROOT}" ]]; then
    echo "現在のGoディレクトリがありません: ${GO_ROOT}" >&2
    exit 1
  fi

  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  failed="${GO_ROOT}.failed-${stamp}"
  mv "${GO_ROOT}" "${failed}"
  mv "${backup}" "${GO_ROOT}"
  echo "ロールバック完了: $("${GO_ROOT}/bin/go" version)"
  echo "更新失敗時のGoは退避済み: ${failed}"
}

install() {
  local workdir owner stamp backup extracted

  if [[ ! -d "${GO_ROOT}" || ! -x "${GO_ROOT}/bin/go" ]]; then
    echo "既存Goがありません: ${GO_ROOT}" >&2
    exit 1
  fi

  owner="$(stat -c '%u:%g' "${GO_ROOT}")"
  workdir="$(mktemp -d /home/isucon/local/.go-install.XXXXXX)"
  trap 'rm -rf "${workdir}"' EXIT

  curl --fail --location --proto '=https' --tlsv1.2 --output "${workdir}/${GO_ARCHIVE}" "${GO_URL}"
  printf '%s  %s\n' "${GO_SHA256}" "${workdir}/${GO_ARCHIVE}" | sha256sum --check --status
  tar -C "${workdir}" -xzf "${workdir}/${GO_ARCHIVE}"
  extracted="${workdir}/go"
  test "$("${extracted}/bin/go" version)" = "go version go${GO_VERSION} linux/amd64"
  chown -R "${owner}" "${extracted}"

  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  backup="${GO_ROOT}.backup-${stamp}"
  mv "${GO_ROOT}" "${backup}"
  mv "${extracted}" "${GO_ROOT}"
  trap - EXIT
  rm -rf "${workdir}"

  echo "Go更新完了: $("${GO_ROOT}/bin/go" version)"
  echo "旧版は退避済み: ${backup}"
  echo "ロールバック: $0 --rollback ${backup}"
}

case "${1:-}" in
  "") install ;;
  --rollback)
    if [[ $# -ne 2 ]]; then
      echo "使用方法: $0 --rollback /home/isucon/local/go.backup-<timestamp>" >&2
      exit 1
    fi
    rollback "$2"
    ;;
  *)
    echo "使用方法: $0 [--rollback /home/isucon/local/go.backup-<timestamp>]" >&2
    exit 1
    ;;
esac
