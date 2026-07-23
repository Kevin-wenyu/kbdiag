#!/usr/bin/env bash
# Runs all pure-function unit tests in test/unit/ — no DB, no VM, no network.
# Each test_*.sh is a standalone script that exits 0 on pass, non-zero on fail.
set -euo pipefail

UNIT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FAIL=0
COUNT=0

for t in "$UNIT_DIR"/test_*.sh; do
  [[ -f "$t" ]] || continue
  COUNT=$((COUNT + 1))
  name="$(basename "$t")"
  echo "=== $name ==="
  if bash "$t"; then
    echo "--- $name: PASS"
  else
    echo "--- $name: FAIL"
    FAIL=1
  fi
  echo
done

if [[ "$COUNT" -eq 0 ]]; then
  echo "no unit tests found in $UNIT_DIR"
  exit 1
fi

if [[ "$FAIL" -eq 0 ]]; then
  echo "all unit tests passed"
else
  echo "unit tests FAILED"
fi

exit "$FAIL"
