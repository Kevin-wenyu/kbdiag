#!/usr/bin/env bash
set -euo pipefail
OUT=dist/kbdiag
mkdir -p dist

{
  echo '#!/usr/bin/env bash'
  echo '# kbdiag — KingbaseES CLI diagnostic tool (built by build.sh, do not edit directly)'
  echo 'set -euo pipefail'
  echo ''
  # strip shebang and set from lib files
  for f in lib/core.sh lib/cmd_status.sh lib/cmd_cluster.sh \
            lib/cmd_replication.sh lib/cmd_sessions.sh lib/cmd_locks.sh \
            lib/cmd_check.sh lib/cmd_perf.sh lib/cmd_space.sh; do
    [[ -f "$f" ]] || continue
    echo "# --- $f ---"
    grep -v '^#!' "$f" || true
    echo ''
  done

  cat << 'DISPATCH'
# ─── dispatch ────────────────────────────────────────────────────────────────
require_kingbase_user

# parse -v / --verbose anywhere in args
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
  check)       cmd_check || exit $? ;;
  perf)        cmd_perf "$SUBCMD" ;;
  space)       cmd_space       ;;
  all)
    cmd_status; cmd_cluster; cmd_replication
    cmd_sessions; cmd_locks; cmd_check || true; cmd_perf ""; cmd_space
    ;;
  -h|--help|help|"")
    cat <<'USAGE'
Usage: kbdiag [-v] <command> [subcommand]

Commands:
  status       Instance process, connectivity, role, uptime
  cluster      Repmgr cluster topology (skipped if standalone)
  replication  Standby connections (primary) or lag (standby)
  sessions     Non-idle sessions
  locks        Waiting lock chains
  check        Health threshold checks (exit 0=OK 1=WARN 2=FAIL)
  perf         Performance: [slow|bloat|vacuum|index] (default: all)
  space        Storage: disk, tables, WAL, archive
  all          Run all checks above

Flags:
  -v, --verbose    Show full underlying data (default: summary only)

Connection overrides:
  KB_PORT KB_BIN_DIR KB_DATA_DIR KB_REPMGR_CONF KB_SUPERUSER KB_DB

Threshold overrides:
  KB_WARN_CONN KB_FAIL_CONN   connection % (default 70/90)
  KB_WARN_LAG  KB_FAIL_LAG   replication lag seconds (default 30/300)
  KB_WARN_TXN  KB_FAIL_TXN   long transaction seconds (default 300/1800)
  KB_WARN_DEAD_TUPLES         dead tuple count (default 100000)
  KB_SLOW_THRESHOLD           slow query seconds (default 5)
USAGE
    ;;
  *)
    echo "Unknown command: $CMD" >&2; exit 1 ;;
esac
DISPATCH
} > "$OUT"

chmod +x "$OUT"
echo "Built: $OUT"
