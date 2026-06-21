#!/usr/bin/env bash
# kbdiag — development entry point (sources lib/* directly)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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
  "$SCRIPT_DIR"/lib/cmd_advisor.sh; do
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
  stat)        cmd_stat "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  obj)         cmd_obj "$SUBCMD" ;;
  temp)        cmd_temp ;;
  watch)       cmd_watch "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  conf)        cmd_conf "$SUBCMD" ;;
  audit)       cmd_audit ;;
  logs)        cmd_logs ${SUBCMD:+"$SUBCMD"} "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  remote)      cmd_remote "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  kill)        cmd_kill "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}" ;;
  idx)         cmd_idx "$SUBCMD" ;;
  all)
    cmd_status; cmd_cluster; cmd_replication; cmd_sessions
    cmd_locks ""; cmd_check || true; cmd_perf "" ""; cmd_space ""
    ;;
  -h|--help|help|"") bash "$SCRIPT_DIR/dist/kbdiag" --help ;;
  *) echo "Unknown command: $CMD" >&2; exit 1 ;;
esac
