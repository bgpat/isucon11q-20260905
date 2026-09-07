#!/usr/bin/env bash
# hostname に応じて Valkey / MariaDB / systemd の接続先を合わせる。
set -euo pipefail

ISUCON1_HOSTNAME="ip-192-168-0-11"
ISUCON2_HOSTNAME="ip-192-168-0-12"
DROPIN_DIR="/etc/systemd/system/isucondition.go.service.d"
DROPIN="${DROPIN_DIR}/host.conf"
HOST_ENV="${DROPIN_DIR}/host.env"
ENV_SH="/home/isucon/env.sh"

host="$(hostname -s 2>/dev/null || hostname)"

mkdir -p "${DROPIN_DIR}"

# systemd 245 では EnvironmentFile（env.sh）が drop-in の Environment= より後に
# 適用される。MYSQL_HOST は env.sh 自体と、env.sh より後に読む host.env の両方を合わせる。
set_mysql_host() {
  local value="$1"
  if [[ ! -f "${ENV_SH}" ]]; then
    return 0
  fi
  if grep -qE '^MYSQL_HOST=' "${ENV_SH}"; then
    sed -i -E "s/^MYSQL_HOST=.*/MYSQL_HOST=\"${value}\"/" "${ENV_SH}"
  else
    printf 'MYSQL_HOST="%s"\n' "${value}" >> "${ENV_SH}"
  fi
}

write_host_dropin() {
  cat > "${HOST_ENV}" <<EOF
$1
EOF
  cat > "${DROPIN}" <<EOF
[Service]
EnvironmentFile=${HOST_ENV}
EOF
}

if [[ "${host}" == "${ISUCON2_HOSTNAME}" ]]; then
  set_mysql_host "${ISUCON1_HOSTNAME}"
  write_host_dropin "MYSQL_HOST=${ISUCON1_HOSTNAME}
VALKEY_ADDR=127.0.0.1:6379"
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
  set_mysql_host "127.0.0.1"
  write_host_dropin "VALKEY_ADDR=${ISUCON2_HOSTNAME}:6379"
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
  set_mysql_host "127.0.0.1"
  rm -f "${DROPIN}" "${HOST_ENV}"
  systemctl disable --now valkey.service 2>/dev/null || true
fi
