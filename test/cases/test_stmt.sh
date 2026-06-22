test_stmt_exit_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE stmt")
  assert_exit_code 0 "$code"
}

test_stmt_has_four_sections() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stmt")
  assert_contains "$out" "平均耗时"
  assert_contains "$out" "总耗时"
  assert_contains "$out" "IO"
  assert_contains "$out" "调用频率"
}

test_stmt_has_stats_reset() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stmt")
  assert_contains "$out" "数据覆盖"
}
