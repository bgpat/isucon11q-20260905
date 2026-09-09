#!/bin/sh
set -eu

priv=$(hostname -I | awk '{print $1}')
join=$(
	for f in /files/hosts/ip-192-168-0-*; do
		test -f "$f" || continue
		awk '{print "    - "$2":7946"}' "$f"
	done | sort -u
)
if [ -z "$join" ]; then
	join='    - 127.0.0.1:7946'
fi

awk -v join="$join" -v addr="$priv" '
	/^__TEMPO_JOIN_MEMBERS__$/ { print join; next }
	{ gsub("__TEMPO_INSTANCE_ADDR__", addr); print }
' /files/tempo/config.yml > /etc/tempo/config.yml
