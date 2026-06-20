cmd_wait() {
  hdr "Wait event analysis"

  local cnt
  cnt=$(ksql_q "SELECT count(*) FROM sys_stat_activity WHERE wait_event IS NOT NULL;" | tr -d '[:space:]')

  if [[ "${cnt:-0}" -eq 0 ]]; then
    ok "No wait events currently"
    return 0
  fi

  info "Wait event distribution ($cnt session(s) waiting):"
  ksql_q "
    SELECT wait_event_type, wait_event, count(*) AS sessions,
           round(avg(EXTRACT(EPOCH FROM (now()-query_start))),1) AS avg_wait_sec
    FROM sys_stat_activity
    WHERE wait_event IS NOT NULL
    GROUP BY wait_event_type, wait_event
    ORDER BY sessions DESC LIMIT $TOP_N;" | column -t -s '|' || true

  if [[ -n "$VERBOSE" ]]; then
    echo ""
    info "Session details by wait event:"
    ksql_q "
      SELECT wait_event_type, wait_event, pid,
             left(query, 80) AS query
      FROM sys_stat_activity
      WHERE wait_event IS NOT NULL
      ORDER BY wait_event_type, wait_event, pid
      LIMIT $TOP_N;" | column -t -s '|' || true
  fi
}
