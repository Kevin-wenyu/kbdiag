test_check_exit_ok() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE check")
  # 正常环境下 exit 0 或 1（archiver 失败是已知 WARN）
  [[ "$code" -le 1 ]] && _pass || _fail "exit code $code > 1 on healthy node"
}

test_check_has_ok_or_warn() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE check")
  assert_contains "$out" "[OK]"
}

test_check_quiet_hides_ok() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -q check")
  assert_not_contains "$out" "[OK]"
  assert_not_contains "$out" "[INFO]"
}

test_check_verbose_shows_detail() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -v check")
  # verbose 应显示连接用户明细
  assert_contains "$out" "esrep"
}

test_check_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json check")
  assert_json_valid "$out"
  assert_contains "$out" '"command":"check"'
  assert_contains "$out" '"items":'
}

test_check_xid_item_present() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE check")
  assert_contains "$out" "XID"
}

test_check_buffer_hit_present() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE check")
  assert_contains "$out" "Buffer hit"
}

test_check_standby_has_replication_lag() {
  local out; out=$(ssh_node2 "$KBDIAG_REMOTE check")
  assert_contains "$out" "Replication lag"
}

test_check_oldest_txn_present() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE check")
  assert_contains "$out" "oldest"
}
