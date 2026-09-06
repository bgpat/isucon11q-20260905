#!/bin/sh
set -eu

# Stop hooks receive one JSON object on stdin. It is intentionally discarded:
# the target Issue is selected in the versioned configuration below.
cat >/dev/null

repo_root=$(git rev-parse --show-toplevel)
goal_config="$repo_root/.codex/goal-guard.json"
issue_number=$(sed -n 's/.*"issue"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$goal_config")

if [ -z "$issue_number" ]; then
  printf '%s\n' '{"decision":"block","reason":".codex/goal-guard.json の issue 番号が不正です。修正してから作業を継続してください。"}'
  exit 0
fi

if sh "$repo_root/ops/goal-score-guard.sh" --issue "$issue_number" >/dev/null 2>&1; then
  printf '%s\n' '{}'
  exit 0
fi

printf '%s\n' "{\"decision\":\"block\",\"reason\":\"目標 Issue #$issue_number は未達です。final を返さず、直ちに計測→分析→Issue→実装→PR→比較計測を継続してください。\"}"
