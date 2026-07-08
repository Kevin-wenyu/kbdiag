#!/usr/bin/env bash
# perf_check.sh — wall-clock budget check for kbdiag commands.
#
# Runs each command once on node1 (timed inside the VM to exclude SSH
# overhead) and fails if any command exceeds its budget. Budgets are
# deliberately loose (~3x observed baseline) — this catches order-of-
# magnitude regressions (a new N+1 query loop, a missing LIMIT), not
# millisecond noise.
#
# Usage: bash test/perf_check.sh            # check against budgets
#        bash test/perf_check.sh --table    # also print a markdown table
set -uo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 1
# shellcheck source=test/lib/ssh_helpers.sh
source test/lib/ssh_helpers.sh

MD_TABLE=0
[[ "${1:-}" == "--table" ]] && MD_TABLE=1

echo "==> Deploying dist/kbdiag to node1..."
deploy_kbdiag

# cmd|budget_seconds — commands needing args, mutating state (kill),
# network (update), or looping (watch) are excluded.
BUDGETS="
status|10
license|10
cluster|15
replication|10
check|30
space|15
backup|15
params|10
sessions|10
locks|10
perf|30
wait|10
progress|10
stat|20
stmt|15
temp|10
idx|20
conf|10
audit|20
logs|30
diagnose|30
advisor|30
"

# Time one command inside the VM; echoes elapsed milliseconds.
# </dev/null keeps ssh from draining the heredoc that feeds the caller's loop.
time_cmd_ms() {
  ssh_node1 "s=\$(date +%s%N); $KBDIAG_REMOTE $1 >/dev/null 2>&1; e=\$(date +%s%N); echo \$(( (e-s)/1000000 ))" </dev/null
}

FAILED=0
ROWS=""
printf "%-14s %10s %10s  %s\n" "command" "ms" "budget_s" "result"
while IFS='|' read -r cmd budget; do
  [[ -z "$cmd" ]] && continue
  ms=$(time_cmd_ms "$cmd" | tr -d '[:space:]')
  if [[ -z "$ms" ]]; then
    printf "%-14s %10s %10s  %s\n" "$cmd" "n/a" "$budget" "FAIL (no timing)"
    FAILED=1
    continue
  fi
  budget_ms=$(( budget * 1000 ))
  if (( ms > budget_ms )); then
    printf "%-14s %10s %10s  %s\n" "$cmd" "$ms" "$budget" "FAIL"
    FAILED=1
  else
    printf "%-14s %10s %10s  %s\n" "$cmd" "$ms" "$budget" "ok"
  fi
  ROWS+="| \`$cmd\` | ${ms} | ${budget} |"$'\n'
done <<< "$BUDGETS"

if (( MD_TABLE )); then
  echo ""
  echo "| command | measured (ms) | budget (s) |"
  echo "|---------|--------------:|-----------:|"
  printf "%s" "$ROWS"
fi

if (( FAILED )); then
  echo ""
  echo "PERF CHECK FAILED — at least one command exceeded its budget"
  exit 1
fi
echo ""
echo "PERF CHECK PASSED"
