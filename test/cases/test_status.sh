test_node1_is_primary() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE status")
  assert_contains "$out" "Role: PRIMARY"
  assert_contains "$out" "[OK]"
}

test_node2_is_standby() {
  local out; out=$(ssh_node2 "$KBDIAG_REMOTE status")
  assert_contains "$out" "Role: STANDBY"
}

test_status_process_running() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE status")
  assert_contains "$out" "kingbase process running"
}

test_status_exit_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE status")
  assert_exit_code 0 "$code"
}

test_status_quiet_hides_ok() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -q status")
  assert_not_contains "$out" "[OK]"
}
