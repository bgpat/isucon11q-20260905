#!/bin/sh

set -eu

work_dir=${ISUCON_WORK_DIR:-/tmp/isucon-work}
current_dir="$work_dir/current"

usage() {
	cat <<'EOF'
使い方:
  ops/benchmark-assets.sh prepare
  ops/benchmark-assets.sh archive <label>
  ops/benchmark-assets.sh time <label> -- <command> [args...]
  ops/benchmark-assets.sh timing-report

prepare:
  既存の current を削除せず退避し、固定名の収集先を作成する。
archive:
  current の計測成果物を <label>-assets.tar.gz にまとめる。
time:
  コマンドの開始・終了・経過秒・終了コードを固定名 timing.tsv に追記する。
  コマンドや引数は記録しない。Linux /proc/uptime と flock を使用する。
timing-report:
  完了済み処理の一覧と、全体経過・区間の和集合・重複・区間外時間を表示する。
EOF
}

time_command() {
	phase=$1
	shift
	case "$phase" in
		''|*[!A-Za-z0-9._-]*)
			echo "label は英数字、'.'、'_'、'-' のみ指定してください" >&2
			exit 2
			;;
	esac
	[ "${1:-}" = -- ] && [ "$#" -gt 1 ] || { usage >&2; exit 2; }
	shift
	mkdir -p "$work_dir"
	# 計測はローカルで行う。壁時計の補正でelapsedが逆転しないようuptimeを使う。
	read -r boot_id < /proc/sys/kernel/random/boot_id
	read -r start_mono unused < /proc/uptime
	start_utc=$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)
	if "$@"; then
		status=0
	else
		status=$?
	fi
	read -r end_mono unused < /proc/uptime
	end_utc=$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)
	elapsed=$(awk -v start="$start_mono" -v end="$end_mono" 'BEGIN { printf "%.2f", end-start }')
	# 並列コマンドも1レコード単位で排他する。内容・引数・出力はTSVへ保存しない。
	(
		flock 9
		printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
			"$phase" "$start_utc" "$end_utc" "$start_mono" "$end_mono" "$elapsed" "$status" "$boot_id" >> "$work_dir/timing.tsv"
	) 9> "$work_dir/timing.lock"
	return "$status"
}

timing_report() {
	if [ ! -s "$work_dir/timing.tsv" ]; then
		echo "完了済みの計測記録はありません。"
		return
	fi
	# 開始順に並べ、区間の和集合を走査する。重複を合計時間から分離する。
	(
		flock -s 9
		sort -t "$(printf '\t')" -k4,4n "$work_dir/timing.tsv"
	) 9> "$work_dir/timing.lock" | awk -F '\t' '
		BEGIN {
			print "|処理|開始 UTC|終了 UTC|経過秒|終了コード|"
			print "|---|---|---|---:|---:|"
		}
		NF != 8 { bad=1; next }
		{
			if (NR==1) { first=$4; left=$4; right=$5; boot=$8 }
			if ($8!=boot || $5<$4) { bad=1; next }
			printf "|%s|%s|%s|%.2f|%s|\n", $1,$2,$3,$6,$7
			sum+=$5-$4
			if ($4>right) { covered+=right-left; left=$4; right=$5 }
			else if ($5>right) right=$5
		}
		END {
			if (bad) {
				print "不正レコードまたは複数ホスト/bootの混在があるため集計できません。" > "/dev/stderr"
				exit 1
			}
			covered+=right-left
			wall=right-first
			printf "\n全体経過: %.2f秒 / 各処理合計: %.2f秒 / 区間の和集合: %.2f秒 / 重複分: %.2f秒 / 区間外: %.2f秒\n", wall,sum,covered,sum-covered,wall-covered
			print "経過はLinux uptime（0.01秒単位）。並列・親子処理の時間は重複するため単純合算しません。"
			print "完了レコードだけを集計します。ツール起動前の承認待ちや未計測処理は測定できず、区間外時間の原因も特定しません。"
		}'
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
	time)
		shift
		[ "$#" -ge 3 ] || { usage >&2; exit 2; }
		time_command "$@"
		;;
	timing-report)
		[ "$#" -eq 1 ] || { usage >&2; exit 2; }
		timing_report
		;;
	*)
		usage >&2
		exit 2
		;;
esac
