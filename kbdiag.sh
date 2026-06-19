#!/usr/bin/env bash
# kbdiag — development entry point (sources lib/* directly)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

for _f in "$SCRIPT_DIR"/lib/core.sh \
           "$SCRIPT_DIR"/lib/cmd_status.sh \
           "$SCRIPT_DIR"/lib/cmd_cluster.sh \
           "$SCRIPT_DIR"/lib/cmd_replication.sh \
           "$SCRIPT_DIR"/lib/cmd_sessions.sh \
           "$SCRIPT_DIR"/lib/cmd_locks.sh \
           "$SCRIPT_DIR"/lib/cmd_check.sh \
           "$SCRIPT_DIR"/lib/cmd_perf.sh \
           "$SCRIPT_DIR"/lib/cmd_space.sh; do
  [[ -f "$_f" ]] && source "$_f"
done

require_kingbase_user

ARGS=()
for arg in "$@"; do
  case "$arg" in
    -v|--verbose) VERBOSE=1 ;;
    *) ARGS+=("$arg") ;;
  esac
done
set -- "${ARGS[@]+"${ARGS[@]}"}"

CMD="${1:-}"
SUBCMD="${2:-}"

case "$CMD" in
  status)      cmd_status      ;;
  cluster)     cmd_cluster     ;;
  replication) cmd_replication ;;
  sessions)    cmd_sessions    ;;
  locks)       cmd_locks       ;;
  check)       cmd_check       ;;
  perf)        cmd_perf "$SUBCMD" ;;
  space)       cmd_space       ;;
  all)
    cmd_status; cmd_cluster; cmd_replication
    cmd_sessions; cmd_locks; cmd_check || true; cmd_perf ""; cmd_space
    ;;
  -h|--help|help|"") bash "$SCRIPT_DIR/dist/kbdiag" --help ;;
  *) echo "Unknown command: $CMD" >&2; exit 1 ;;
esac
