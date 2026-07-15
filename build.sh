#!/usr/bin/env bash
set -euo pipefail
OUT=dist/kbdiag
mkdir -p dist

KBDIAG_VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")

{
  echo '#!/usr/bin/env bash'
  echo '# kbdiag — KingbaseES CLI diagnostic tool (built by build.sh, do not edit directly)'
  echo 'set -euo pipefail'
  echo "KBDIAG_VERSION=\"${KBDIAG_VERSION}\""
  echo '[[ -f ~/.kbdiagrc ]] && source ~/.kbdiagrc || true'
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
            lib/cmd_advisor.sh \
            lib/cmd_stmt.sh \
            lib/cmd_diagnose.sh \
            lib/cmd_license.sh \
            lib/cmd_backup.sh \
            lib/cmd_explain.sh \
            lib/cmd_jobs.sh \
            lib/cmd_partition.sh \
            lib/cmd_update.sh; do
    [[ -f "$f" ]] || continue
    echo "# --- $f ---"
    grep -v '^#!' "$f" || true
    echo ''
  done

  cat << 'DISPATCH'
# ─── dispatch ────────────────────────────────────────────────────────────────
require_kingbase_user "$@"

parse_global_args "$@"
set -- "${ARGS[@]+"${ARGS[@]}"}"

CMD="${1:-}"
SUBCMD="${2:-}"
shift 2 2>/dev/null || true
CMD_ARGS=("$@")

case "$CMD" in
  status)      cmd_status ;;
  cluster)     cmd_cluster "$SUBCMD" || exit $? ;;
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
  backup)      cmd_backup ;;
  # shift 2 above is a no-op when only the command name was given, leaving the
  # command name itself in CMD_ARGS — only forward args when SUBCMD exists.
  explain)     cmd_explain ${SUBCMD:+"$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}"} ;;
  jobs)        cmd_jobs ;;
  partition)   cmd_partition ;;
  logs)        cmd_logs ${SUBCMD:+"$SUBCMD"} "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  remote)      cmd_remote "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  kill)        cmd_kill "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  idx)         cmd_idx "$SUBCMD" ;;
  colstat)     cmd_colstat "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  advisor)     cmd_advisor "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" || exit $? ;;
  stmt)       cmd_stmt "$SUBCMD" ;;
  diagnose)   cmd_diagnose ${SUBCMD:+"$SUBCMD"} "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  license)     cmd_license ;;
  update)      cmd_update ;;
  all)
    cmd_status; cmd_cluster; cmd_replication; cmd_sessions
    cmd_locks ""; cmd_check || true; cmd_perf "" ""; cmd_space ""
    ;;
  --version|-V)
    echo "kbdiag ${KBDIAG_VERSION}"
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

[OPS] Quick fact lookup — one command, one deterministic answer:
  status              Instance process, connectivity, role, uptime
  license             License validity and expiry
  cluster [ready]     Repmgr cluster topology; 'ready' = failover readiness
                      checklist, exit 0=OK 1=WARN 2=FAIL
  replication         Replication lag / standby connections
  check               15-item health check, exit 0=OK 1=WARN 2=FAIL
  space [frag]        Disk, tables, WAL, archive, fragmentation
  backup              Backup and WAL archiving readiness (archiver, sys_rman, slots)
  params [pattern]    Instance parameters
  update              Update kbdiag to the latest version from GitHub

[DBA] Single-dimension deep query — also used to verify a ROOT-CAUSE finding:
  sessions            Non-idle sessions
  locks [hold|wait|deadlock]   Lock analysis
  perf [slow|bloat|vacuum|wait|io|wal|top]   Performance diagnostics
  sql [pid|all]       SQL text + EXPLAIN for a session
  stmt [queryid]      SQL history statistics — Top N by mean/total/IO/calls (AWR-style)
  explain <queryid|"SQL">  Plan analysis: EXPLAIN + red flags (seq scan, nested loop, sort)
  wait                Wait event distribution
  progress            Long-running operation progress
  jobs                Scheduler job health (kdb_schedule: broken jobs, failed runs)
  partition           Partition table health (missing DEFAULT, size skew, orphan partitions)
  stat                Throughput metrics (TPS, buffer hit rate)
  obj <schema.table>  Object deep-dive (size, indexes, constraints)
  colstat <schema.table> [--col <col>]  Column statistics deep-dive (n_distinct, MCV, correlation)
  temp                Temp file and sort spill analysis
  idx [unused|dup|bloat|missing]  Index health analysis
  kill [--terminate] [pid|--long N|--idle-txn N] [--dry-run] [--force]  Cancel or terminate queries
  conf [diff]         Configuration restart-pending status / cross-node comparison
  audit               Security and compliance checks
  logs                Log file analysis (slow queries, errors)

[ROOT-CAUSE] Multi-dimension correlation — full chain: symptom -> evidence -> cause -> fix:
  diagnose [--full]   Root-cause diagnostic report (fast <15s; --full ~90s)
  advisor [index|vacuum|params|analyze] [--fix]  Consolidated DBA recommendations

[UTIL]
  watch <N> <cmd>     Repeat any command every N seconds
  remote <nodes> <cmd>  Multi-node batch diagnostics
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
