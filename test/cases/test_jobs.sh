# test_jobs.sh — tests for "kbdiag jobs" command

test_jobs_exits_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE jobs")
  assert_exit_code 0 "${code:-0}"
}

test_jobs_shows_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE jobs")
  assert_contains "$out" "Scheduler Jobs"
}

test_jobs_handles_empty_or_lists() {
  # Either no jobs defined (test env default), extension missing, or a job list
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE jobs")
  if echo "$out" | grep -qE 'No scheduler jobs defined|not installed|job\(s\) defined'; then
    _pass
  else
    _fail "unexpected jobs output: $(echo "$out" | head -3)"
  fi
}

test_jobs_works_on_standby() {
  local code; code=$(ssh_node2_exit "$KBDIAG_REMOTE jobs")
  assert_exit_code 0 "${code:-0}"
}
