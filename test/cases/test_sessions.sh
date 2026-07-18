# test_sessions.sh — tests for "kbdiag sessions" command

test_sessions_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE sessions")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "sessions exited $code"
}

test_sessions_verdict_has_values() {
  # output contract: verdict lines must carry measured values, never bare conclusions
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE sessions" 2>&1)
  echo "$out" | grep -qE "Connections: [0-9]+/[0-9]+ \([0-9]+%" \
    && _pass || _fail "sessions verdict line missing measured values"
}

test_sessions_verdict_idle_txn_present() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE sessions" 2>&1)
  assert_contains "$out" "Idle-in-transaction"
}

test_sessions_has_column_header() {
  # Force at least one non-idle session, then check the data table renders.
  start_slow_query_bg
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE sessions" 2>&1)
  stop_slow_query_bg
  echo "$out" | grep -q "DURATION" && _pass || _fail "sessions missing DURATION column header"
}

test_sessions_verbose_detail_columns() {
  # output contract: -v expands to full columns
  start_slow_query_bg
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -v sessions" 2>&1)
  stop_slow_query_bg
  echo "$out" | grep -q "XACT_S" && _pass || _fail "sessions -v missing XACT_S detail column"
}

test_sessions_exit_code_flag() {
  # KB_WARN_CONN=0 deterministically forces a WARN (connections are never 0%)
  local code
  code=$(ssh_node1_exit "KB_WARN_CONN=0 $KBDIAG_REMOTE sessions")
  [[ "${code:-1}" -eq 0 ]] || { _fail "sessions without --exit-code should exit 0 on WARN, got $code"; return; }
  code=$(ssh_node1_exit "KB_WARN_CONN=0 $KBDIAG_REMOTE --exit-code sessions")
  [[ "${code:-0}" -eq 1 ]] && _pass || _fail "sessions --exit-code should exit 1 on WARN, got $code"
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
