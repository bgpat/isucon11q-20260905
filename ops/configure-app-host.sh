#!/usr/bin/env bash
# hostname に応じて Valkey / MariaDB / systemd の接続先を合わせる。
set -euo pipefail

ISUCON1_HOSTNAME="ip-192-168-0-11"
ISUCON2_HOSTNAME="ip-192-168-0-12"
DROPIN_DIR="/etc/systemd/system/isucondition.go.service.d"
DROPIN="${DROPIN_DIR}/host.conf"

host="$(hostname -s 2>/dev/null || hostname)"

mkdir -p "${DROPIN_DIR}"

if [[ "${host}" == "${ISUCON2_HOSTNAME}" ]]; then
  cat > "${DROPIN}" <<EOF
[Service]
Environment=MYSQL_HOST=${ISUCON1_HOSTNAME}
Environment=VALKEY_ADDR=127.0.0.1:6379
EOF
  priv="$(hostname -I | awk '{print $1}')"
  if [[ -z "${priv}" ]]; then
    echo "私有 IP を取得できません" >&2
    exit 1
  fi
  sed -i -E "s/^bind .*/bind 127.0.0.1 ${priv}/" /etc/valkey/valkey.conf
  systemctl enable valkey.service
  systemctl restart valkey.service
  systemctl is-active --quiet valkey.service
elif [[ "${host}" == "${ISUCON1_HOSTNAME}" ]]; then
  cat > "${DROPIN}" <<EOF
[Service]
Environment=VALKEY_ADDR=${ISUCON2_HOSTNAME}:6379
EOF
  systemctl disable --now valkey.service 2>/dev/null || true
  if ! ss -ltn | grep -q ':3306'; then
    echo "MariaDB が 3306 で待っていません" >&2
    exit 1
  fi
  if ss -ltn | grep -q '127.0.0.1:3306'; then
    if ! ss -ltn | grep -qE '0\.0\.0\.0:3306|\*:3306'; then
      systemctl restart mysql.service
      systemctl is-active --quiet mysql.service
    fi
  fi
  mysql --defaults-file=/dev/null -u root -e "GRANT ALL PRIVILEGES ON isucondition.* TO 'isucon'@'%' IDENTIFIED BY 'isucon'; FLUSH PRIVILEGES;"
else
  rm -f "${DROPIN}"
  systemctl disable --now valkey.service 2>/dev/null || true
fi
