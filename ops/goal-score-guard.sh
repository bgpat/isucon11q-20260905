#!/bin/sh
# 最新 benchmark Release の記録だけを根拠に、継続目標の達成可否を判定する。
set -eu

issue_number=""
if [ "${1:-}" = "--issue" ] && [ -n "${2:-}" ]; then
	issue_number="$2"
	shift 2
fi
if [ "$#" -ne 0 ]; then
	printf '%s\n' 'usage: goal-score-guard.sh [--issue ISSUE_NUMBER]'
	exit 2
fi

target="${GOAL_SCORE_TARGET:-1000000}"
issue_context=""
if [ -n "$issue_number" ]; then
	issue="$(gh issue view "$issue_number" --json number,state,title,url,body)"
	issue_target="$(printf '%s' "$issue" | jq -r '.body' | sed -nE 's/.*goal-score:[[:space:]]*([0-9,]+).*/\1/p' | head -n 1 | tr -d ',')"
	if [ -z "$issue_target" ]; then
		printf '%s\n' "CONTINUE: Issue #${issue_number} に goal-score: <整数> がありません"
		exit 1
	fi
	target="$issue_target"
	issue_context=" issue=#${issue_number} state=$(printf '%s' "$issue" | jq -r .state)"
fi

repository="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
release="$(gh api "repos/${repository}/releases?per_page=100" | jq -r '[.[] | select(.tag_name | startswith("benchmark-"))] | sort_by(.created_at) | last')"

if [ "$release" = "null" ]; then
	printf '%s\n' 'CONTINUE: benchmark Release がありません'
	exit 1
fi

tag="$(printf '%s' "$release" | jq -r .tag_name)"
text="$(printf '%s' "$release" | jq -r '.body + " " + .name')"
score="$(printf '%s\n' "$text" | sed -nE 's/.*スコア[^0-9]*([0-9,]+).*/\1/p' | head -n 1 | tr -d ',')"

if [ -z "$score" ]; then
	printf '%s\n' "CONTINUE: ${tag} からスコアを読めません"
	exit 1
fi

if [ "$score" -ge "$target" ]; then
	printf '%s\n' "GOAL_ACHIEVED: ${tag} score=${score} target=${target}${issue_context}"
	exit 0
fi

printf '%s\n' "CONTINUE: ${tag} score=${score} target=${target}${issue_context}"
exit 1
