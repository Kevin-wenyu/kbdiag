#!/usr/bin/env bash
# Pure static check, no DB/VM required: asserts every lib/cmd_*.sh command is
# reachable from BOTH build.sh's dispatch table and kbdiag.sh's dispatch table
# (and kbdiag.sh's source list). This is the guardrail for the R0 drift bug
# where kbdiag.sh (dev entry point) silently fell behind build.sh (release
# packer) — 8 commands were unreachable via `bash kbdiag.sh <cmd>` for an
# unknown period before being caught by manual review.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FAIL=0

# Commands that intentionally have no top-level dispatch entry (aggregate /
# meta pseudo-commands defined directly in the dispatch case, not lib/cmd_*.sh).
SKIP_COMMANDS=()

COMMANDS=()
while IFS= read -r b; do
  COMMANDS+=("${b#cmd_}")
done < <(
  for f in "$ROOT_DIR"/lib/cmd_*.sh; do
    basename "$f" .sh
  done | sort -u
)

is_skipped() {
  local cmd="$1"
  for s in "${SKIP_COMMANDS[@]+"${SKIP_COMMANDS[@]}"}"; do
    [[ "$cmd" == "$s" ]] && return 0
  done
  return 1
}

check_file_contains_dispatch() {
  local file="$1" cmd="$2"
  # Dispatch entries look like `  cmd)` or `  cmd|alias)` at line start
  # (allowing leading whitespace), matching build.sh/kbdiag.sh case syntax.
  grep -qE "^[[:space:]]*${cmd}(\\)|\\|)" "$file"
}

check_file_contains_source() {
  local file="$1" cmd="$2"
  grep -q "cmd_${cmd}.sh" "$file"
}

for cmd in "${COMMANDS[@]}"; do
  is_skipped "$cmd" && continue

  if ! check_file_contains_dispatch "$ROOT_DIR/build.sh" "$cmd"; then
    echo "FAIL: build.sh dispatch is missing command '$cmd'"
    FAIL=1
  fi
  if ! check_file_contains_source "$ROOT_DIR/build.sh" "$cmd"; then
    echo "FAIL: build.sh source list is missing lib/cmd_${cmd}.sh"
    FAIL=1
  fi

  if ! check_file_contains_dispatch "$ROOT_DIR/kbdiag.sh" "$cmd"; then
    echo "FAIL: kbdiag.sh dispatch is missing command '$cmd'"
    FAIL=1
  fi
  if ! check_file_contains_source "$ROOT_DIR/kbdiag.sh" "$cmd"; then
    echo "FAIL: kbdiag.sh source list is missing lib/cmd_${cmd}.sh"
    FAIL=1
  fi
done

if [[ "$FAIL" -eq 0 ]]; then
  echo "PASS: ${#COMMANDS[@]} commands present in build.sh and kbdiag.sh (dispatch + source)"
fi

exit "$FAIL"
