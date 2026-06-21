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
            lib/cmd_audit.sh lib/cmd_logs.sh lib/cmd_remote.sh \
            lib/cmd_kill.sh \
            lib/cmd_idx.sh \
            lib/cmd_colstat.sh \
            lib/cmd_advisor.sh; do
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
  logs)        cmd_logs ${SUBCMD:+"$SUBCMD"} "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  remote)      cmd_remote "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  kill)        cmd_kill "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  idx)         cmd_idx "$SUBCMD" ;;
  colstat)     cmd_colstat "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  all)
    cmd_status; cmd_cluster; cmd_replication; cmd_sessions
    cmd_locks ""; cmd_check || true; cmd_perf "" ""; cmd_space ""
    ;;
  -h|--help|help|"")
    cat <<'USAGE'
Usage: kbdiag [global-flags] <command> [subcommand] [command-flags]

Global flags:
  -v, --verbose       Show full underlying data
  -q, --quiet         Only show WARN/FAIL (suppress OK/INFO)
  -n N, --top N       Limit result rows (default: 10)
  --format text|json  Output format (default: text)
  --no-color          Disable ANSI colors
  --timeout N         DB query timeout seconds (default: 10)

[OPS] Commands (no DBA context required):
  status              Instance process, connectivity, role, uptime
  cluster             Repmgr cluster topology
  replication         Replication lag / standby connections
  check               14-item health check, exit 0=OK 1=WARN 2=FAIL
  space [frag]        Disk, tables, WAL, archive, fragmentation

[DBA] Commands (requires interpretation):
  sessions            Non-idle sessions
  locks [hold|wait|deadlock]   Lock analysis
  perf [slow|bloat|vacuum|wait|io|wal|top]   Performance diagnostics
  sql [pid|all]       SQL text + EXPLAIN for a session
  wait                Wait event distribution
  progress            Long-running operation progress
  params [pattern]    Instance parameters
  stat                Throughput metrics (TPS, buffer hit rate)
  obj <schema.table>  Object deep-dive (size, indexes, constraints)
  colstat <schema.table> [--col <col>]  Column statistics deep-dive (n_distinct, MCV, correlation)
  temp                Temp file and sort spill analysis
  watch <N> <cmd>     Repeat any command every N seconds
  conf [diff]         Configuration audit / node comparison
  audit               Security and compliance checks
  logs                Log file analysis (slow queries, errors)
  remote <nodes> <cmd>  Multi-node batch diagnostics

[ALL]
  all                 Run all checks

Connection overrides: KB_PORT KB_BIN_DIR KB_DATA_DIR KB_REPMGR_CONF KB_SUPERUSER KB_DB
Threshold overrides: KB_WARN_CONN KB_FAIL_CONN KB_WARN_LAG KB_FAIL_LAG KB_WARN_XID KB_FAIL_XID ...
USAGE
    ;;
  *)
    echo "Unknown command: $CMD" >&2; exit 1 ;;
esac
DISPATCH
} > "$OUT"

chmod +x "$OUT"
echo "Built: $OUT"
