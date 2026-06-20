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
            lib/cmd_check.sh lib/cmd_perf.sh lib/cmd_space.sh \
            lib/cmd_sql.sh lib/cmd_wait.sh lib/cmd_progress.sh \
            lib/cmd_params.sh lib/cmd_stat.sh lib/cmd_obj.sh \
            lib/cmd_temp.sh lib/cmd_watch.sh lib/cmd_conf.sh \
            lib/cmd_audit.sh lib/cmd_logs.sh lib/cmd_remote.sh; do
    [[ -f "$f" ]] || continue
    echo "# --- $f ---"
    grep -v '^#!' "$f" || true
    echo ''
  done

  cat << 'DISPATCH'
# ─── dispatch ────────────────────────────────────────────────────────────────
require_kingbase_user

parse_global_args "$@"
set -- "${ARGS[@]+"${ARGS[@]}"}"

CMD="${1:-}"
SUBCMD="${2:-}"
shift 2 2>/dev/null || true
CMD_ARGS=("$@")

case "$CMD" in
  status)      cmd_status ;;
  cluster)     cmd_cluster ;;
  replication) cmd_replication ;;
  sessions)    cmd_sessions ;;
  locks)       cmd_locks "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  check)       cmd_check || exit $? ;;
  perf)        cmd_perf "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  space)       cmd_space "$SUBCMD" ;;
  sql)         cmd_sql "$SUBCMD" ;;
  wait)        cmd_wait ;;
  progress)    cmd_progress ;;
  params)      cmd_params "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  stat)        cmd_stat "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  obj)         cmd_obj "$SUBCMD" ;;
  temp)        cmd_temp ;;
  watch)       cmd_watch "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  conf)        cmd_conf "$SUBCMD" ;;
  audit)       cmd_audit ;;
  logs)        cmd_logs "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  remote)      cmd_remote "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  all)
    cmd_status; cmd_cluster; cmd_replication; cmd_sessions
    cmd_locks ""; cmd_check || true; cmd_perf "" ""; cmd_space ""
    ;;
  -h|--help|help|"")
    cat <<'USAGE'
Usage: kbdiag [-v] [-q] [--no-color] [-n N] [--format text|json|csv] [--timeout S] <command> [subcommand]

Commands:
  status       Instance process, connectivity, role, uptime
  cluster      Repmgr cluster topology (skipped if standalone)
  replication  Standby connections (primary) or lag (standby)
  sessions     Non-idle sessions
  locks        Waiting lock chains
  check        Health threshold checks (exit 0=OK 1=WARN 2=FAIL)
  perf         Performance: [slow|bloat|vacuum|index] (default: all)
  space        Storage: disk, tables, WAL, archive
  sql          SQL utilities
  wait         Wait event analysis
  progress     Long-running operation progress
  params       GUC parameter management
  stat         pg_stat snapshots
  obj          Object DDL inspection
  temp         Temp file usage
  watch        Continuous monitoring
  conf         Config file management
  audit        Audit log review
  logs         Log analysis
  remote       Remote node diagnostics
  all          Run status/cluster/replication/sessions/locks/check/perf/space

Flags:
  -v, --verbose        Show full underlying data (default: summary only)
  -q, --quiet          Show only WARN/FAIL output
  --no-color           Disable color output
  -n N, --top N        Limit results to top N rows (default: 10)
  --format text|json|csv  Output format (default: text)
  --timeout S          Query timeout in seconds (default: 10)

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
