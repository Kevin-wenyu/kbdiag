cmd_locks() {
  hdr "Waiting locks"

  local cnt
  cnt=$(ksql_q "SELECT count(*) FROM sys_locks WHERE NOT granted;" | tr -d '[:space:]')

  if [[ "$cnt" -eq 0 ]]; then
    ok "No waiting locks"
    return 0
  fi

  warn "$cnt waiting lock(s)"
  ksql_q "
    SELECT
      w.pid        AS wait_pid,
      w.usename    AS wait_user,
      left(w.query, 60) AS wait_query,
      b.pid        AS block_pid,
      b.usename    AS block_user,
      left(b.query, 60) AS block_query
    FROM sys_stat_activity w
    JOIN sys_locks lw ON lw.pid = w.pid AND NOT lw.granted
    JOIN sys_locks lb ON lb.locktype = lw.locktype
                      AND lb.relation IS NOT DISTINCT FROM lw.relation
                      AND lb.granted
    JOIN sys_stat_activity b ON b.pid = lb.pid
    LIMIT 20;
  " | column -t -s '|' || true
}
