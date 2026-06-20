#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "$SCRIPT_DIR/lib/assert.sh"
source "$SCRIPT_DIR/lib/ssh_helpers.sh"

FILTER="${1:-}"

echo "==> Deploying dist/kbdiag to both nodes..."
deploy_kbdiag
echo ""

TOTAL_RUN=0; TOTAL_PASSED=0; TOTAL_FAILED=0

for case_file in "$SCRIPT_DIR/cases"/test_*.sh; do
  cmd=$(basename "$case_file" .sh | sed 's/^test_//')
  if [[ -n "$FILTER" && "$cmd" != "$FILTER" ]]; then
    continue
  fi

  echo "==> $case_file"

  # Reset counters for this file
  TESTS_RUN=0; TESTS_PASSED=0; TESTS_FAILED=0; FAILURES=""

  # Source and run all test_* functions
  source "$case_file"
  for fn in $(declare -F | awk '{print $3}' | grep '^test_'); do
    CURRENT_TEST="$fn"
    set +e
    $fn
    set -e
  done

  TOTAL_RUN=$(( TOTAL_RUN + TESTS_RUN ))
  TOTAL_PASSED=$(( TOTAL_PASSED + TESTS_PASSED ))
  TOTAL_FAILED=$(( TOTAL_FAILED + TESTS_FAILED ))
  echo ""
done

echo "============================================"
echo "Total: $TOTAL_PASSED passed, $TOTAL_FAILED failed / $TOTAL_RUN run"
[[ $TOTAL_FAILED -eq 0 ]] && exit 0 || exit 1
