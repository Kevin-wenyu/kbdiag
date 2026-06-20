# test_logs.sh — tests for "kbdiag logs" command

# Helper: find a log file on node1
_find_log_file() {
  local log_dir="${KB_DATA_DIR:-/home/kingbase/cluster/install/kingbase/data}/log"
  ssh_node1 "ls ${log_dir}/*.log 2>/dev/null | head -1 || true"
}

# ── basic run ─────────────────────────────────────────────────────────────────

test_logs_exits_zero_no_logfile() {
  # When no log file can be found (or file flag points to nonexistent), must exit 0
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE logs --file /nonexistent/no.log")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "logs with missing --file should exit 0, got $code"
}

test_logs_warns_when_no_logfile() {
  # When pointing to nonexistent file, should emit a WARN message
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE logs --file /nonexistent/no.log")
  assert_contains "$out" "WARN"
}

test_logs_exits_zero_auto_detect() {
  # Auto-detect mode: whether or not a log file exists, must exit 0
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE logs")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "logs auto-detect should exit 0, got $code"
}

# ── with log file ─────────────────────────────────────────────────────────────

test_logs_with_file_flag_exits_zero() {
  # Find a real log file; if none found, skip (pass with note)
  local log_file; log_file=$(_find_log_file)
  if [[ -z "$log_file" ]]; then
    _pass  # No log file available — graceful skip
    return
  fi
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE logs --file $log_file")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "logs --file $log_file exited $code"
}

test_logs_with_file_has_slow_section() {
  local log_file; log_file=$(_find_log_file)
  if [[ -z "$log_file" ]]; then
    _pass  # No log file available — graceful skip
    return
  fi
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE logs --file $log_file")
  assert_contains "$out" "Slow queries"
}

test_logs_with_file_has_error_section() {
  local log_file; log_file=$(_find_log_file)
  if [[ -z "$log_file" ]]; then
    _pass  # No log file available — graceful skip
    return
  fi
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE logs --file $log_file")
  assert_contains "$out" "ERROR/FATAL"
}

test_logs_with_file_has_top_errors_section() {
  local log_file; log_file=$(_find_log_file)
  if [[ -z "$log_file" ]]; then
    _pass  # No log file available — graceful skip
    return
  fi
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE logs --file $log_file")
  assert_contains "$out" "Top"
}

# ── --since flag ──────────────────────────────────────────────────────────────

test_logs_since_flag_exits_zero() {
  local log_file; log_file=$(_find_log_file)
  if [[ -z "$log_file" ]]; then
    _pass  # No log file available — graceful skip
    return
  fi
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE logs --file $log_file --since 60m")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "logs --since 60m exited $code"
}

test_logs_since_hours_exits_zero() {
  local log_file; log_file=$(_find_log_file)
  if [[ -z "$log_file" ]]; then
    _pass
    return
  fi
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE logs --file $log_file --since 1h")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "logs --since 1h exited $code"
}

# ── --top N flag ──────────────────────────────────────────────────────────────

test_logs_top_n_exits_zero() {
  local log_file; log_file=$(_find_log_file)
  if [[ -z "$log_file" ]]; then
    _pass
    return
  fi
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE --top 5 logs --file $log_file")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "logs --top 5 exited $code"
}
