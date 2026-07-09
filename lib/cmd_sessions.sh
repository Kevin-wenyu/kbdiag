# shellcheck shell=bash
cmd_sessions() {
  hdr "Active sessions"

  local cnt
  cnt=$(ksql_q "SELECT count(*) FROM sys_stat_activity WHERE state <> 'idle' AND pid <> sys_backend_pid();" | tr -d '[:space:]')
  info "Non-idle sessions: $cnt"

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_begin "sessions"
    json_item "non_idle_sessions" "ok" "${cnt:-0}" ""
    local rows
    # translate() strips newlines and pipes from query text so the row
    # survives IFS='|' parsing and stays a legal JSON string.
    rows=$(ksql_q "
      SELECT pid, usename, application_name,
             coalesce(client_addr::text, 'local'), state,
             date_trunc('second', now() - query_start)::text,
             translate(left(query, 80), E'\n\r|', '   ')
      FROM sys_stat_activity
      WHERE state <> 'idle' AND pid <> sys_backend_pid()
      ORDER BY query_start;")
    while IFS='|' read -r pid user app addr state dur query; do
      [[ -z "$pid" ]] && continue
      json_row "pid=${pid// /}" "user=$user" "application=$app" \
        "client_addr=$addr" "state=$state" "duration=$dur" "query=$query"
    done <<< "$rows"
    json_end
    return 0
  fi

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
