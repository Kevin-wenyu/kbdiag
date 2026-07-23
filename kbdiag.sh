#!/usr/bin/env bash
# kbdiag — development entry point (sources lib/* directly)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Mirrors build.sh's KBDIAG_VERSION computation — cmd_update.sh and --version
# reference it unconditionally, and this entry point runs under `set -u`.
KBDIAG_VERSION=$(git -C "$SCRIPT_DIR" describe --tags --always 2>/dev/null || echo "dev")
KBDIAG_VERSION="${KBDIAG_VERSION%%-g*} ($(date +%Y-%m-%d))"
[[ -f ~/.kbdiagrc ]] && source ~/.kbdiagrc || true

for _f in \
  "$SCRIPT_DIR"/lib/core.sh \
  "$SCRIPT_DIR"/lib/cmd_status.sh \
  "$SCRIPT_DIR"/lib/cmd_cluster.sh \
  "$SCRIPT_DIR"/lib/cmd_replication.sh \
  "$SCRIPT_DIR"/lib/cmd_sessions.sh \
  "$SCRIPT_DIR"/lib/cmd_locks.sh \
  "$SCRIPT_DIR"/lib/cmd_check.sh \
  "$SCRIPT_DIR"/lib/cmd_perf.sh \
  "$SCRIPT_DIR"/lib/cmd_space.sh \
  "$SCRIPT_DIR"/lib/cmd_sql.sh \
  "$SCRIPT_DIR"/lib/cmd_wait.sh \
  "$SCRIPT_DIR"/lib/cmd_progress.sh \
  "$SCRIPT_DIR"/lib/cmd_params.sh \
  "$SCRIPT_DIR"/lib/cmd_stat.sh \
  "$SCRIPT_DIR"/lib/cmd_obj.sh \
  "$SCRIPT_DIR"/lib/cmd_temp.sh \
  "$SCRIPT_DIR"/lib/cmd_watch.sh \
  "$SCRIPT_DIR"/lib/cmd_conf.sh \
  "$SCRIPT_DIR"/lib/cmd_audit.sh \
  "$SCRIPT_DIR"/lib/cmd_logs.sh \
  "$SCRIPT_DIR"/lib/cmd_remote.sh \
  "$SCRIPT_DIR"/lib/cmd_kill.sh \
  "$SCRIPT_DIR"/lib/cmd_idx.sh \
  "$SCRIPT_DIR"/lib/cmd_colstat.sh \
  "$SCRIPT_DIR"/lib/cmd_advisor.sh \
  "$SCRIPT_DIR"/lib/cmd_diagnose.sh \
  "$SCRIPT_DIR"/lib/cmd_license.sh \
  "$SCRIPT_DIR"/lib/cmd_backup.sh \
  "$SCRIPT_DIR"/lib/cmd_explain.sh \
  "$SCRIPT_DIR"/lib/cmd_jobs.sh \
  "$SCRIPT_DIR"/lib/cmd_partition.sh \
  "$SCRIPT_DIR"/lib/cmd_report.sh \
  "$SCRIPT_DIR"/lib/cmd_snapshot.sh \
  "$SCRIPT_DIR"/lib/cmd_stmt.sh \
  "$SCRIPT_DIR"/lib/cmd_update.sh; do
  [[ -f "$_f" ]] && source "$_f"
done

require_kingbase_user

parse_global_args "$@"
set -- "${ARGS[@]+"${ARGS[@]}"}"

CMD="${1:-}"
SUBCMD="${2:-}"
shift 2 2>/dev/null || true
CMD_ARGS=("$@")

case "$CMD" in
  status)      cmd_status ;;
  cluster)     cmd_cluster "$SUBCMD" || exit $? ;;
  report)      cmd_report "$SUBCMD" || exit $? ;;
  snapshot)    cmd_snapshot "$SUBCMD" || exit $? ;;
  backup)      cmd_backup ;;
  # shift 2 above is a no-op when only the command name was given, leaving the
  # command name itself in CMD_ARGS — only forward args when SUBCMD exists.
  explain)     cmd_explain ${SUBCMD:+"$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}"} ;;
  jobs)        cmd_jobs ;;
  partition)   cmd_partition ;;
  stmt)        cmd_stmt "$SUBCMD" ;;
  update)      cmd_update ;;
  replication) cmd_replication ;;
  sessions)    cmd_sessions ;;
  locks)       cmd_locks "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  check)       cmd_check "$SUBCMD" || exit $? ;;
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
  advisor)     cmd_advisor "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" || exit $? ;;
  diagnose)   cmd_diagnose ${SUBCMD:+"$SUBCMD"} "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  license)     cmd_license ;;
  all)
    cmd_status; cmd_cluster; cmd_replication; cmd_sessions
    cmd_locks ""; cmd_check || true; cmd_perf "" ""; cmd_space ""
    ;;
  --version|-V) echo "kbdiag ${KBDIAG_VERSION}" ;;
  -h|--help|help|"") bash "$SCRIPT_DIR/dist/kbdiag" --help ;;
  *) echo "Unknown command: $CMD" >&2; exit 1 ;;
esac
