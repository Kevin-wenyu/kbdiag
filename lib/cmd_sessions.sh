cmd_sessions() {
  hdr "Active sessions"

  local cnt
  cnt=$(ksql_q "SELECT count(*) FROM sys_stat_activity WHERE state <> 'idle' AND pid <> sys_backend_pid();" | tr -d '[:space:]')
  info "Non-idle sessions: $cnt"

  if [[ -n "$VERBOSE" || "$cnt" -gt 0 ]]; then
    ksql_qh "
      SELECT pid, usename, application_name, client_addr, state,
             date_trunc('second', now() - query_start)::text AS duration,
             left(query, 80) AS query
      FROM sys_stat_activity
      WHERE state <> 'idle' AND pid <> sys_backend_pid()
      ORDER BY query_start;
    " | column -t -s '|' || true
  fi
}
