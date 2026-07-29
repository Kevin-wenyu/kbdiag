test_instances_exit_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE instances")
  assert_exit_code 0 "$code"
}

test_instances_single_shows_count() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE instances")
  assert_contains "$out" "1 kingbase instance detected"
}

test_instances_table_has_headers() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE instances")
  assert_contains "$out" "PID"
  assert_contains "$out" "PORT"
  assert_contains "$out" "DATA_DIR"
}

test_instances_table_has_real_port() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE instances")
  assert_contains "$out" "54321"
}

test_instances_quiet_hides_table() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -q instances")
  assert_not_contains "$out" "PID"
}

test_instances_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json instances")
  assert_json_valid "$out"
}

test_instances_json_has_instance_count() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json instances")
  assert_contains "$out" '"name":"instance_count"'
}

# End-to-end multi-instance scenario: initdb + start a second throwaway
# postmaster under the same OS user, confirm both `instances` and `status`
# surface the ambiguity, then always tear the second instance back down —
# cleanup runs unconditionally (run_tests.sh disables `set -e` around each
# test function) so a mid-test assertion failure can't leak the process.
test_instances_and_status_detect_multiple() {
  local bin="/home/kingbase/cluster/install/kingbase/bin"
  local dd="/tmp/kbdiag_test_instance2"

  ssh_node1 "rm -rf $dd && $bin/initdb -D $dd -U system >/dev/null 2>&1"
  ssh_node1 "$bin/sys_ctl -D $dd -o '-p 54322' -l $dd.log start >/dev/null 2>&1"
  sleep 2

  local out; out=$(ssh_node1 "$KBDIAG_REMOTE instances")
  assert_contains "$out" "2 kingbase instances detected"
  assert_contains "$out" "54322"

  local status_out; status_out=$(ssh_node1 "$KBDIAG_REMOTE status")
  assert_contains "$status_out" "Detected 2 kingbase processes"
  assert_contains "$status_out" "kbdiag instances"

  ssh_node1 "$bin/sys_ctl -D $dd stop >/dev/null 2>&1"
  ssh_node1 "rm -rf $dd $dd.log"

  local cleanup_out; cleanup_out=$(ssh_node1 "$KBDIAG_REMOTE instances")
  assert_contains "$cleanup_out" "1 kingbase instance detected"
}
