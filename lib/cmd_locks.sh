_locks_wait() {
  hdr "Waiting locks"

  local cnt
  cnt=$(ksql_q "SELECT count(*) FROM sys_locks WHERE NOT granted;" | tr -d '[:space:]')

  if [[ "${cnt:-0}" -eq 0 ]]; then
    ok "No waiting locks"
    return 0
  fi

  warn "$cnt waiting lock(s)"
  ksql_q "
    SELECT w.pid AS wait_pid, w.usename, left(w.query,60) AS wait_query,
           b.pid AS block_pid, b.usename AS block_user,
           lw.locktype, lw.mode AS wait_mode,
           date_trunc('second', now()-w.query_start)::text AS waiting_for
    FROM sys_stat_activity w
    JOIN sys_locks lw ON lw.pid=w.pid AND NOT lw.granted
    JOIN sys_locks lb ON lb.locktype=lw.locktype
                      AND lb.relation IS NOT DISTINCT FROM lw.relation
                      AND lb.granted
    JOIN sys_stat_activity b ON b.pid=lb.pid
    LIMIT ${TOP_N};" \
    | column -t -s '|' || true
}

_locks_hold() {
  hdr "Lock-holding sessions"

  ksql_q "
    SELECT l.pid, a.usename, a.application_name,
           l.locktype, l.relation::regclass AS object,
           l.mode, date_trunc('second', now()-a.xact_start)::text AS held_for,
           left(a.query, 60) AS query
    FROM sys_locks l
    JOIN sys_stat_activity a ON a.pid = l.pid
    WHERE l.granted = true
      AND l.pid <> sys_backend_pid()
      AND l.locktype = 'relation'
    ORDER BY a.xact_start
    LIMIT ${TOP_N};" \
    | column -t -s '|' || true
}

_locks_deadlock() {
  hdr "Deadlock statistics"

  local cnt
  cnt=$(ksql_q "SELECT count(*) FROM sys_stat_database WHERE deadlocks > 0;" | tr -d '[:space:]')

  if [[ "${cnt:-0}" -eq 0 ]]; then
    ok "No deadlocks recorded"
    return 0
  fi

  warn "$cnt database(s) with deadlocks"
  ksql_q "
    SELECT datname, deadlocks
    FROM sys_stat_database
    WHERE deadlocks > 0
    ORDER BY deadlocks DESC;" \
    | column -t -s '|' || true
}

cmd_locks() {
  local subcmd="${1:-}"
  shift 2>/dev/null || true
  case "$subcmd" in
    hold)     _locks_hold     ;;
    deadlock) _locks_deadlock ;;
    wait|"")  _locks_wait     ;;
    *)
      echo "Unknown locks subcommand: $subcmd (wait|hold|deadlock)" >&2
      return 1
      ;;
  esac
}
