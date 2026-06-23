# test_wait.sh — tests for "kbdiag wait" command

test_wait_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE wait")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "wait exited $code"
}

test_wait_shows_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE wait")
  assert_contains "$out" "Wait event"
}

test_wait_runs_with_slow_query() {
  start_slow_query_bg
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE wait")
  stop_slow_query_bg
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "wait with slow query exited $code"
}

test_wait_verbose_runs() {
  start_slow_query_bg
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE -v wait")
  stop_slow_query_bg
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "wait -v exited $code"
}

test_wait_produces_output() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE wait")
  local lines; lines=$(echo "$out" | grep -v '^$' | wc -l | tr -d ' ')
  [[ "${lines:-0}" -ge 1 ]] && _pass || _fail "wait produced no output"
}

# ── header verification ──────────────────────────────────────────────────────

test_wait_has_column_header_or_empty_ok() {
  # When idle there may be no waits → "No wait events". With load, header appears.
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE wait" 2>&1)
  echo "$out" | grep -qE 'wait_event|No wait events' && _pass \
    || _fail "wait missing both 'wait_event' header and 'No wait events' message"
}
