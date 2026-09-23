# shellcheck shell=bash disable=SC2034
# Shared helpers for fault-injection scripts. Sourced by e2e/inject/*.sh, not run directly.
# Scripts run on the Mac and drive the Lima VMs; every injected session is tagged
# application_name=kbdiag_inj_<tag> so it can be found and removed.
set -euo pipefail

NODE="${KB_TEST_NODE:-kes-node1}"
STANDBY_NODE="${KB_STANDBY_NODE:-kes-node2}"

# Helpers predefined for every remote script (runs as kingbase on the VM).
REMOTE_HELPERS=$(cat <<'EOF'
set -euo pipefail
PIDDIR=$HOME/.kbdiag_inj
mkdir -p "$PIDDIR"
# One statement per call: ksql -c runs multiple statements in one transaction block.
sql() { ksql -d test -U system -p 54321 -v ON_ERROR_STOP=1 -Atqc "$1"; }
# hold <tag> <statements>: open a tagged session, run the statements, keep it open.
hold() {
  local tag=$1 stmts=$2
  setsid bash -c '{ printf "%s\n" "$1"; exec sleep 3600; } | ksql "dbname=test user=system port=54321 application_name=kbdiag_inj_$2" -q' \
    _ "$stmts" "$tag" </dev/null >/dev/null 2>&1 &
  echo $! >"$PIDDIR/$tag.pid"
}
# release <tag>: kill the holder's process group and any backend still carrying the tag.
release() {
  local tag=$1
  if [ -f "$PIDDIR/$tag.pid" ]; then
    kill -- "-$(cat "$PIDDIR/$tag.pid")" 2>/dev/null || true
    rm -f "$PIDDIR/$tag.pid"
  fi
  sql "select count(pg_terminate_backend(pid)) from sys_stat_activity where application_name='kbdiag_inj_$tag'" >/dev/null
}
# wait_for <seconds> <sql returning a boolean>; ksql prints booleans as t/f.
wait_for() {
  local deadline=$((SECONDS + $1))
  while [ "$SECONDS" -lt "$deadline" ]; do
    # Leading "(" keeps macOS bash 3.2 from ending the enclosing $( ) at this ")".
    case "$(sql "$2" | tr -d '[:space:]')" in (t | true) return 0 ;; esac
    sleep 0.5
  done
  echo "timeout waiting for: $2" >&2
  return 1
}
EOF
)

# remote [node]: run the bash script on stdin on the node as kingbase, with helpers loaded.
remote() {
  { printf '%s\n' "$REMOTE_HELPERS"; cat; } | limactl shell "${1:-$NODE}" sudo -iu kingbase bash -s
}

usage() {
  echo "usage: KB_TEST_NODE=<lima-node> $0 up|down" >&2
  exit 64
}
