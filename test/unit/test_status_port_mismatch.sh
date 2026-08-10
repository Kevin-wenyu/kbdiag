#!/usr/bin/env bash
# Regression test for the multi-instance port-mismatch bug: cmd_status
# matched the running process's data dir against KB_DATA_DIR but never
# cross-checked KB_PORT against the process's actual LISTEN port, so a
# host with several kingbase instances got a bare "Cannot connect to DB
# on port <configured>" with no indication of what the real port was.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FAIL=0

# shellcheck source=/dev/null
source "$ROOT_DIR/lib/core.sh"
# shellcheck source=/dev/null
source "$ROOT_DIR/lib/cmd_status.sh"
# shellcheck source=/dev/null
source "$ROOT_DIR/lib/cmd_instances.sh"

# Fake a single matched process whose real data dir matches KB_DATA_DIR
# (so cmd_status reports "process ok") but whose actual LISTEN port
# (54325, via a fake `ss -tlnp` snapshot) differs from configured KB_PORT.
pgrep() { echo "4242"; }
ps() { echo "/opt/kingbase/bin/kingbase -D /data/kingbase_i2/data"; }
ss() { echo 'LISTEN 0 128 127.0.0.1:54325 0.0.0.0:* users:(("kingbase",pid=4242,fd=6))'; }
ksql_q() { echo "connection failed" >&2; return 1; }

KB_DATA_DIR="/data/kingbase_i2/data"
KB_PORT="54321"

check_contains() {
  local label="$1" haystack="$2" needle="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    echo "ok: $label"
  else
    echo "FAIL: $label — expected to find '$needle'"
    echo "--- actual output ---"
    echo "$haystack"
    echo "---------------------"
    FAIL=1
  fi
}

OUTPUT_FMT="text"
out=$(cmd_status || true)
check_contains "text mentions configured port"   "$out" "Cannot connect to DB on port 54321"
check_contains "text mentions detected pid"       "$out" "pid 4242 is actually listening on port 54325"
check_contains "text suggests the fix"            "$out" "set KB_PORT=54325"

OUTPUT_FMT="json"
out=$(cmd_status || true)
check_contains "json flags db_connect as fail"    "$out" '"name":"db_connect","status":"fail"'
check_contains "json detail carries mismatch"     "$out" "configured KB_PORT=54321 does not match actual port 54325 for pid 4242"

exit "$FAIL"
