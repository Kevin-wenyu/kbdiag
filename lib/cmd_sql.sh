# shellcheck shell=bash
cmd_sql() {
  local target="${1:-all}"
  hdr "SQL analysis"

  if [[ "$target" == "all" ]]; then
    # All non-idle sessions summary
    ksql_qh "
      SELECT pid, usename, state,
             date_trunc('second', now()-query_start)::text AS duration,
             wait_event_type, wait_event,
             left(query, 80) AS query
      FROM sys_stat_activity
      WHERE state <> 'idle' AND pid <> sys_backend_pid()
      ORDER BY query_start LIMIT $TOP_N;" | column -t -s '|' || true
    return 0
  fi

  # Single pid deep dive
  local text
  text=$(ksql_q "SELECT query FROM sys_stat_activity WHERE pid = $target;")
  if [[ -z "$text" ]]; then
    warn "No active session with pid=$target"
    return 0
  fi

  info "PID $target full query:"
  echo "$text"
  echo ""

  local wait_info
  wait_info=$(ksql_q "SELECT wait_event_type, wait_event, state FROM sys_stat_activity WHERE pid=$target;")
  info "Wait state: $wait_info"
  echo ""

  if [[ -n "${ANALYZE_MODE:-}" ]]; then
    info "EXPLAIN (ANALYZE, BUFFERS):"
    "$KSQL" -p "$KB_PORT" -U "$KB_SUPERUSER" "$KB_DB" -AXc "EXPLAIN (ANALYZE, BUFFERS) $text" 2>/dev/null || warn "EXPLAIN failed (query may have ended)"
  elif [[ -n "$VERBOSE" ]]; then
    info "EXPLAIN (VERBOSE, COSTS):"
    "$KSQL" -p "$KB_PORT" -U "$KB_SUPERUSER" "$KB_DB" -AXc "EXPLAIN (VERBOSE, COSTS) $text" 2>/dev/null || warn "EXPLAIN failed (query may have ended)"
  else
    info "EXPLAIN:"
    "$KSQL" -p "$KB_PORT" -U "$KB_SUPERUSER" "$KB_DB" -AXc "EXPLAIN $text" 2>/dev/null || warn "EXPLAIN failed (query may have ended)"
  fi
}
