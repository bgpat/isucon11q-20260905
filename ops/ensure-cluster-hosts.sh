#!/usr/bin/env bash
# files/hosts/* の hostname を /etc/hosts に足す。既存の名前は変更しない。
set -euo pipefail

hosts_dir="/files/hosts"
if [[ ! -d "${hosts_dir}" ]]; then
  echo "hosts dir missing: ${hosts_dir}" >&2
  exit 1
fi

shopt -s nullglob
for f in "${hosts_dir}"/ip-*; do
  while read -r name ip _; do
    [[ -z "${name:-}" || "${name}" == \#* ]] && continue
    [[ -z "${ip:-}" ]] && continue
    if grep -qE "(^|[[:space:]])${name}([[:space:]]|$)" /etc/hosts; then
      continue
    fi
    echo "${ip} ${name}" >> /etc/hosts
  done < "${f}"
done
