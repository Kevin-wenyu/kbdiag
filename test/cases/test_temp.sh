# test_temp.sh — tests for "kbdiag temp" command

test_temp_exits_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE temp")
  assert_exit_code 0 "${code:-0}"
}

test_temp_shows_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE temp")
  assert_contains "$out" "Temp file usage"
}

test_temp_shows_cumulative_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE temp")
  assert_contains "$out" "Cumulative temp file stats"
}

test_temp_shows_sessions_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE temp")
  assert_contains "$out" "Current sessions"
}

test_temp_shows_settings_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE temp")
  assert_contains "$out" "Related settings"
}

test_temp_shows_work_mem_setting() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE temp")
  assert_contains "$out" "work_mem"
}

test_temp_quiet_flag() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE -q temp")
  assert_exit_code 0 "${code:-0}"
}

test_temp_has_settings_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE temp")
  # The "Related settings" table always renders (3 rows), so header must appear
  assert_contains "$out" "name"
}
