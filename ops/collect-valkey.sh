#!/bin/sh
# Valkey のスナップショット取得と統計リセット。LOAD 中には実行しない。
# valkey.conf は変更しない。MONITOR は使わない。
#
# 使い方（Valkey が listen しているホストで）:
#   /ops/collect-valkey.sh snapshot > /tmp/isucon-work/current/valkey-before.txt
#   /ops/collect-valkey.sh reset
#   # ベンチ
#   /ops/collect-valkey.sh snapshot > /tmp/isucon-work/current/valkey-after.txt
set -eu

VALKEY_CLI=${VALKEY_CLI:-/usr/local/bin/valkey-cli}
VALKEY_HOST=${VALKEY_HOST:-127.0.0.1}
VALKEY_PORT=${VALKEY_PORT:-6379}

cli() {
	"$VALKEY_CLI" -h "$VALKEY_HOST" -p "$VALKEY_PORT" "$@"
}

ensure_cli() {
	if [ ! -x "$VALKEY_CLI" ]; then
		echo "valkey-cli がありません: $VALKEY_CLI" >&2
		exit 1
	fi
	if ! cli ping | grep -qx PONG; then
		echo "Valkey に接続できません: ${VALKEY_HOST}:${VALKEY_PORT}" >&2
		exit 1
	fi
}

snapshot() {
	ensure_cli
	echo "# valkey-snapshot"
	echo "# collected_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	echo "# host=$(hostname -s 2>/dev/null || hostname)"
	echo "# addr=${VALKEY_HOST}:${VALKEY_PORT}"
	echo "# slowlog_len=$(cli --raw SLOWLOG LEN)"
	echo
	echo "=== INFO all ==="
	cli INFO all
	echo
	echo "=== SLOWLOG GET 128 ==="
	cli SLOWLOG GET 128
	echo
	echo "=== MEMORY STATS ==="
	cli MEMORY STATS
	echo
	echo "=== ZCARD cond:z:* ==="
	cli --raw --scan --pattern 'cond:z:*' | while IFS= read -r key; do
		[ -n "$key" ] || continue
		printf '%s %s\n' "$key" "$(cli --raw ZCARD "$key")"
	done
	echo "# latest_keys=$(cli --raw --scan --pattern 'cond:latest:*' | awk 'END { print NR }')"
}

reset_stats() {
	ensure_cli
	cli CONFIG RESETSTAT >/dev/null
	cli SLOWLOG RESET >/dev/null
	echo "CONFIG RESETSTAT と SLOWLOG RESET を実行した: ${VALKEY_HOST}:${VALKEY_PORT}"
}

case "${1:-}" in
	snapshot)
		[ "$#" -eq 1 ] || { echo "使い方: $0 snapshot" >&2; exit 2; }
		snapshot
		;;
	reset)
		[ "$#" -eq 1 ] || { echo "使い方: $0 reset" >&2; exit 2; }
		reset_stats
		;;
	*)
		echo "使い方: $0 snapshot|reset" >&2
		exit 2
		;;
esac
