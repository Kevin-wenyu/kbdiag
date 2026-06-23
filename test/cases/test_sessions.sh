# test_sessions.sh — tests for "kbdiag sessions" command

test_sessions_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE sessions")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "sessions exited $code"
}

test_sessions_shows_header_text() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE sessions" 2>&1)
  assert_contains "$out" "Active sessions"
}

test_sessions_has_column_header() {
  # Force at least one non-idle session, then check usename column header appears.
  start_slow_query_bg
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE sessions" 2>&1)
  stop_slow_query_bg
  echo "$out" | grep -qi "usename" && _pass || _fail "sessions missing 'usename' column header"
}

# ── sessions 补强 ────────────────────────────────────────────

test_sessions_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json sessions")
  assert_json_valid "$out"
}

test_sessions_idle_in_txn_flag() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE sessions --idle-txn 1")
  assert_exit_code 0 "${code:-0}"
}

test_sessions_long_flag() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE sessions --long 1")
  assert_exit_code 0 "${code:-0}"
}

test_sessions_long_shows_query() {
  start_slow_query_bg
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE sessions --long 0" 2>&1)
  stop_slow_query_bg
  assert_contains "$out" "pg_sleep"
}

test_sessions_no_crash_on_node2() {
  local code; code=$(ssh_node2_exit "$KBDIAG_REMOTE sessions")
  assert_exit_code 0 "${code:-0}"
}
