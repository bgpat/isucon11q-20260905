#!/bin/sh

set -eu

work_dir=${ISUCON_WORK_DIR:-/tmp/isucon-work}
current_dir="$work_dir/current"

usage() {
	cat <<'EOF'
使い方:
  ops/benchmark-assets.sh prepare
  ops/benchmark-assets.sh archive <label>

prepare:
  既存の current を削除せず退避し、固定名の収集先を作成する。
archive:
  current の計測成果物を <label>-assets.tar.gz にまとめる。
EOF
}

prepare() {
	mkdir -p "$work_dir"
	if [ -e "$current_dir" ] || [ -L "$current_dir" ]; then
		timestamp=$(date -u +%Y%m%dT%H%M%SZ)
		backup_dir="$work_dir/archive-before-$timestamp"
		if [ -e "$backup_dir" ] || [ -L "$backup_dir" ]; then
			echo "退避先が既に存在します: $backup_dir" >&2
			exit 1
		fi
		mv "$current_dir" "$backup_dir"
	fi
	mkdir -m 700 "$current_dir"
}

archive() {
	label=$1
	case "$label" in
		''|*[!A-Za-z0-9._-]*)
			echo "label は英数字、'.'、'_'、'-' のみ指定してください: $label" >&2
			exit 1
		;;
	esac

	archive_path="$work_dir/${label}-assets.tar.gz"
	if [ -e "$archive_path" ] || [ -L "$archive_path" ]; then
		echo "archive が既に存在します: $archive_path" >&2
		exit 1
	fi

	files='bench.log db-before.txt db-after.txt cpu.pprof slow.log.gz access.log.gz error.log journal.log prometheus-db.json prometheus-cpu.json'
	set --
	for file in $files; do
		if [ -e "$current_dir/$file" ]; then
			set -- "$@" "$file"
		fi
	done
	if [ "$#" -eq 0 ]; then
		echo "current に収集成果物がありません: $current_dir" >&2
		exit 1
	fi
	tar -czf "$archive_path" -C "$current_dir" "$@"
	printf '%s\n' "$archive_path"
}

case "${1:-}" in
	prepare)
		[ "$#" -eq 1 ] || { usage >&2; exit 2; }
		prepare
		;;
	archive)
		[ "$#" -eq 2 ] || { usage >&2; exit 2; }
		archive "$2"
		;;
	*)
		usage >&2
		exit 2
		;;
esac
