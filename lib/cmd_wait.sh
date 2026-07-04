# shellcheck shell=bash

# Shared with cmd_perf.sh's _perf_wait and cmd_stat.sh's _stat_section_wait_events —
# edit only here to change what counts as "currently waiting".
_WAIT_EVENT_WHERE="wait_event IS NOT NULL AND state <> 'idle'"

cmd_wait() {
  hdr "Wait event analysis"

  local cnt
  cnt=$(ksql_q "SELECT count(*) FROM sys_stat_activity WHERE $_WAIT_EVENT_WHERE;" | tr -d '[:space:]')

  if [[ "${cnt:-0}" -eq 0 ]]; then
    ok "No wait events currently"
    return 0
  fi

  info "Wait event distribution ($cnt session(s) waiting):"
  ksql_qh "
    SELECT wait_event_type, wait_event, count(*) AS sessions,
           round(avg(EXTRACT(EPOCH FROM (now()-query_start))),1) AS avg_wait_sec
    FROM sys_stat_activity
    WHERE $_WAIT_EVENT_WHERE
    GROUP BY wait_event_type, wait_event
    ORDER BY sessions DESC LIMIT $TOP_N;" | column -t -s '|' || true

  if [[ -n "$VERBOSE" ]]; then
    echo ""
    info "Session details by wait event:"
    ksql_qh "
      SELECT wait_event_type, wait_event, pid,
             left(query, 80) AS query
      FROM sys_stat_activity
      WHERE $_WAIT_EVENT_WHERE
      ORDER BY wait_event_type, wait_event, pid
      LIMIT $TOP_N;" | column -t -s '|' || true
  fi
}
