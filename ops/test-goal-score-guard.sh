#!/bin/sh
# goal-score-guard.sh の Release 判定をネットワークなしで確認する。
set -eu

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

cat >"$tmp_dir/gh" <<'EOF'
#!/bin/sh
set -eu

if [ "$1" = "issue" ]; then
	printf '%s\n' '{"number":166,"state":"OPEN","title":"目標","url":"https://example.test/issues/166","body":"goal-score: 1000000"}'
	exit 0
fi
if [ "$1" = "repo" ]; then
	printf '%s\n' 'example/isucon'
	exit 0
fi
if [ "$1" = "api" ]; then
	case "${FAKE_SCENARIO:?}" in
	latest_fail)
		printf '%s\n' '[{"tag_name":"benchmark-new-fail","created_at":"2026-09-06T02:00:00Z","name":"fail","body":"失敗"},{"tag_name":"benchmark-old-score-74249","created_at":"2026-09-06T01:00:00Z","name":"74,249点","body":"スコア 74,249"}]'
		;;
	no_success)
		printf '%s\n' '[{"tag_name":"benchmark-new-fail","created_at":"2026-09-06T02:00:00Z","name":"fail","body":"失敗"}]'
		;;
	latest_success)
		printf '%s\n' '[{"tag_name":"benchmark-new-score-74249","created_at":"2026-09-06T02:00:00Z","name":"74,249点","body":"スコア 74,249"}]'
		;;
	esac
	exit 0
fi
exit 2
EOF
chmod +x "$tmp_dir/gh"

run_case() {
	name=$1
	expected_status=$2
	expected_text=$3
	shift 3
	set +e
	output=$(PATH="$tmp_dir:$PATH" "$@")
	status=$?
	set -e
	if [ "$status" != "$expected_status" ]; then
		printf '%s\n' "FAIL ${name}: status=${status}, expected=${expected_status}" >&2
		exit 1
	fi
	if ! printf '%s\n' "$output" | grep -F -- "$expected_text" >/dev/null; then
		printf '%s\n' "FAIL ${name}: output=${output}" >&2
		exit 1
	fi
	printf '%s\n' "PASS ${name}: ${output}"
}

run_case latest_fail_uses_prior_success 1 'score=74249 target=1000000 latest=benchmark-new-fail(スコアなし)' env FAKE_SCENARIO=latest_fail sh "$root_dir/ops/goal-score-guard.sh" --issue 166
run_case no_success_score 1 '成功スコア付き benchmark Release がありません' env FAKE_SCENARIO=no_success sh "$root_dir/ops/goal-score-guard.sh" --issue 166
run_case score_below_target 1 'CONTINUE: benchmark-new-score-74249 score=74249 target=1000000' env FAKE_SCENARIO=latest_success sh "$root_dir/ops/goal-score-guard.sh" --issue 166
run_case score_reaches_target 0 'GOAL_ACHIEVED: benchmark-new-score-74249 score=74249 target=70000' env FAKE_SCENARIO=latest_success GOAL_SCORE_TARGET=70000 sh "$root_dir/ops/goal-score-guard.sh"
