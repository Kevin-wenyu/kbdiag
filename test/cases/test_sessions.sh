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
