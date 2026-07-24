# shellcheck shell=bash
cmd_status() {
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "status"
  hdr "Instance status"

  local pid
  pid=$(pgrep -U kingbase -f "/kingbase -D " | head -1 || true)
  if [[ -n "$pid" ]]; then
    local actual_data_dir
    actual_data_dir=$(ps -o args= -p "$pid" 2>/dev/null | sed -n 's/.*-D[[:space:]]\+\([^[:space:]]*\).*/\1/p')
    if [[ -n "$actual_data_dir" && "$actual_data_dir" != "$KB_DATA_DIR" ]]; then
      warn "kingbase process running (pid $pid), but actual data dir ($actual_data_dir) differs from configured KB_DATA_DIR ($KB_DATA_DIR) — set KB_DATA_DIR to avoid inaccurate downstream checks"
      json_item "process" "warn" "$pid" "data_dir mismatch: configured=$KB_DATA_DIR actual=$actual_data_dir"
    else
      ok "kingbase process running (pid $pid)"
      json_item "process" "ok" "$pid" ""
    fi
  else
    fail "kingbase process not found"
    json_item "process" "fail" "" ""
    [[ "$OUTPUT_FMT" == "json" ]] && json_end
    return 1
  fi

  local ver
  if ver=$(ksql_q "SELECT version();" 2>&1); then
    ok "DB connectable — $ver"
    json_item "db_connect" "ok" "" "$ver"
  else
    fail "Cannot connect to DB on port $KB_PORT"
    json_item "db_connect" "fail" "" "port $KB_PORT"
    [[ "$OUTPUT_FMT" == "json" ]] && json_end
    return 1
  fi

  local _exit=0

  local is_standby
  is_standby=$(ksql_q "SELECT pg_is_in_recovery()::text;" | tr -d '[:space:]')
  if [[ "$is_standby" == "false" ]]; then
    ok "Role: PRIMARY"
    json_item "role" "ok" "PRIMARY" ""
  else
    ok "Role: STANDBY"
    json_item "role" "ok" "STANDBY" ""
  fi

  local uptime
  uptime=$(ksql_q "SELECT date_trunc('second', now() - pg_postmaster_start_time())::text;")
  info "Uptime: $uptime"
  json_item "uptime" "ok" "$uptime" ""

  local lic_days
  lic_days=$(ksql_q "SELECT get_license_validdays();" | tr -d '[:space:]')
  lic_days="${lic_days:-0}"
  if [[ "$lic_days" == "-2" ]]; then
    ok "License: 永久授权（正式）"
    json_item "license" "ok" "-2" "perpetual"
  elif [[ "$lic_days" -gt "$KB_WARN_LICENSE_DAYS" ]]; then
    ok "License: 剩余 ${lic_days} 天"
    json_item "license" "ok" "$lic_days" "days remaining"
  elif [[ "$lic_days" -gt 0 ]]; then
    warn "License: 剩余 ${lic_days} 天到期"
    json_item "license" "warn" "$lic_days" "days remaining, below KB_WARN_LICENSE_DAYS=$KB_WARN_LICENSE_DAYS"
    [[ "$_exit" -lt 1 ]] && _exit=1
  else
    fail "License: 已过期"
    json_item "license" "fail" "$lic_days" "expired"
    _exit=2
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}
