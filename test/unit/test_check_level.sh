#!/usr/bin/env bash
# Unit test for lib/cmd_check.sh's _check_level (CONTRIBUTING.md §4 example):
# severity is a max-taking merge (0=ok, 1=warn, 2=fail), order-independent.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FAIL=0

# shellcheck source=/dev/null
source "$ROOT_DIR/lib/core.sh"
# shellcheck source=/dev/null
source "$ROOT_DIR/lib/cmd_check.sh"

check_eq() {
  local label="$1" actual="$2" expected="$3"
  if [[ "$actual" == "$expected" ]]; then
    echo "ok: $label"
  else
    echo "FAIL: $label — expected '$expected', got '$actual'"
    FAIL=1
  fi
}

check_eq "warn_then_fail" "$(_check_level 1 2)" "2"
check_eq "fail_then_warn" "$(_check_level 2 1)" "2"
check_eq "ok_then_warn"   "$(_check_level 0 1)" "1"
check_eq "warn_then_ok"   "$(_check_level 1 0)" "1"
check_eq "ok_then_ok"     "$(_check_level 0 0)" "0"
check_eq "fail_then_fail" "$(_check_level 2 2)" "2"

exit "$FAIL"
