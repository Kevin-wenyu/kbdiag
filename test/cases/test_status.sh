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

test_status_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json status" 2>/dev/null)
  assert_json_valid "$out"
}

test_status_json_has_license_item() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json status" 2>/dev/null)
  assert_contains "$out" '"name":"license"'
}
