#!/bin/sh
set -eu

hostname_short=$(hostname -s 2>/dev/null || hostname)
hostfile="/files/hosts/${hostname_short}"
if [ -f "$hostfile" ]; then
	addr=$(awk '{print $2; exit}' "$hostfile")
else
	addr=$(hostname -I | awk '{print $1}')
fi

join=$(
	for f in /files/hosts/*; do
		test -f "$f" || continue
		ip=$(awk '{print $2; exit}' "$f")
		test -n "$ip" || continue
		printf '    - %s:7947\n' "$ip"
	done | sort -u
)
if [ -z "$join" ]; then
	join="    - ${addr}:7947"
fi

awk -v join="$join" -v addr="$addr" '
	/^__LOKI_JOIN_MEMBERS__$/ { print join; next }
	{ gsub("__LOKI_INSTANCE_ADDR__", addr); print }
' /files/loki/config.yml > /etc/loki/config.yml
