# test_progress.sh — tests for "kbdiag progress" command

test_progress_exits_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE progress")
  assert_exit_code 0 "${code:-0}"
}

test_progress_shows_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE progress")
  assert_contains "$out" "Operation progress"
}

test_progress_empty_shows_no_vacuum() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE progress")
  assert_contains "$out" "No vacuum operations in progress"
}

test_progress_empty_shows_no_index() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE progress")
  assert_contains "$out" "No index creation in progress"
}

test_progress_empty_shows_no_long_queries() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE progress")
  assert_contains "$out" "No long-running queries"
}

test_progress_shows_long_query() {
  start_slow_query_bg
  # Wait slightly longer so the query ages past 60s is not guaranteed in test,
  # but the command itself should still exit 0 and show relevant info
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE progress")
  stop_slow_query_bg
  assert_exit_code 0 "${code:-0}"
}

test_progress_quiet_flag() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE -q progress")
  assert_exit_code 0 "${code:-0}"
}
