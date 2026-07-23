#!/usr/bin/env bash
# Unit test for lib/core.sh's parse_global_args (CONTRIBUTING.md §4 example):
# global flags must be extracted regardless of position, leaving positional
# args (the actual command + subcommand) in ARGS.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FAIL=0

# shellcheck source=/dev/null
source "$ROOT_DIR/lib/core.sh"

check_eq() {
  local label="$1" actual="$2" expected="$3"
  if [[ "$actual" == "$expected" ]]; then
    echo "ok: $label"
  else
    echo "FAIL: $label — expected '$expected', got '$actual'"
    FAIL=1
  fi
}

TOP_N=""; OUTPUT_FMT="text"; VERBOSE=0; EXIT_CODE_MODE=""; QUIET=0
parse_global_args --top 5 --format json status
check_eq "top" "$TOP_N" "5"
check_eq "format" "$OUTPUT_FMT" "json"
check_eq "args_count" "${#ARGS[@]}" "1"
check_eq "args_0" "${ARGS[0]}" "status"

TOP_N=""; OUTPUT_FMT="text"
parse_global_args status check --top=10 --format=json
check_eq "top_eq_form" "$TOP_N" "10"
check_eq "format_eq_form" "$OUTPUT_FMT" "json"
check_eq "args_count_interspersed" "${#ARGS[@]}" "2"
check_eq "args_0_interspersed" "${ARGS[0]}" "status"
check_eq "args_1_interspersed" "${ARGS[1]}" "check"

VERBOSE=0; EXIT_CODE_MODE=""; QUIET=0
parse_global_args -v --exit-code -q status
check_eq "verbose" "$VERBOSE" "1"
check_eq "exit_code_mode" "$EXIT_CODE_MODE" "1"
check_eq "quiet" "$QUIET" "1"

exit "$FAIL"
