cmd_status() {
  hdr "Instance status"

  local pid
  pid=$(pgrep -U kingbase -f "kingbase.*-D $KB_DATA_DIR" | head -1 || true)
  if [[ -n "$pid" ]]; then
    ok "kingbase process running (pid $pid)"
  else
    fail "kingbase process not found"
    return 1
  fi

  local ver
  if ver=$(ksql_q "SELECT version();" 2>&1); then
    ok "DB connectable — $ver"
  else
    fail "Cannot connect to DB on port $KB_PORT"
    return 1
  fi

  local is_standby
  is_standby=$(ksql_q "SELECT pg_is_in_recovery()::text;" | tr -d '[:space:]')
  if [[ "$is_standby" == "false" ]]; then
    ok "Role: PRIMARY"
  else
    ok "Role: STANDBY"
  fi

  local uptime
  uptime=$(ksql_q "SELECT date_trunc('second', now() - pg_postmaster_start_time())::text;")
  info "Uptime: $uptime"
}
