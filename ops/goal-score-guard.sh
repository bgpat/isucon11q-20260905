#!/bin/sh
# benchmark Release の成功スコア記録だけを根拠に、継続目標の達成可否を判定する。
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
releases="$(gh api "repos/${repository}/releases?per_page=100")"
benchmark_releases="$(printf '%s' "$releases" | jq '[.[] | select(.tag_name | startswith("benchmark-"))] | sort_by(.created_at) | reverse')"

if [ "$(printf '%s' "$benchmark_releases" | jq 'length')" -eq 0 ]; then
	printf '%s\n' 'CONTINUE: benchmark Release がありません'
	exit 1
fi

latest="$(printf '%s' "$benchmark_releases" | jq -c 'first')"
latest_tag="$(printf '%s' "$latest" | jq -r .tag_name)"

# 成功した benchmark Release は、リリースタグに score-<整数> を持つものとして記録する。
# fail / aborted Release はスコアがないため、最新であっても達成判定の母集団に含めない。
release="$(printf '%s' "$benchmark_releases" | jq -c '[.[] | select(.tag_name | test("-score-[0-9]+$"))] | first // empty')"

if [ -z "$release" ]; then
	printf '%s\n' "CONTINUE: 最新 benchmark Release ${latest_tag} はスコアなし（fail または中断）。成功スコア付き benchmark Release がありません${issue_context}"
	exit 1
fi

tag="$(printf '%s' "$release" | jq -r .tag_name)"
score="$(printf '%s\n' "$tag" | sed -nE 's/.*-score-([0-9]+)$/\1/p')"

if [ -z "$score" ]; then
	printf '%s\n' "CONTINUE: ${tag} からスコアを読めません${issue_context}"
	exit 1
fi

latest_context=""
if [ "$latest_tag" != "$tag" ]; then
	latest_context=" latest=${latest_tag}(スコアなし)"
fi

if [ "$score" -ge "$target" ]; then
	printf '%s\n' "GOAL_ACHIEVED: ${tag} score=${score} target=${target}${latest_context}${issue_context}"
	exit 0
fi

printf '%s\n' "CONTINUE: ${tag} score=${score} target=${target}${latest_context}${issue_context}"
exit 1
