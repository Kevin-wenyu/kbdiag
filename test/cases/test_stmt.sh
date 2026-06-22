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

test_stmt_queryid_exit_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE stmt 0000000000000000")
  assert_exit_code 0 "$code"
}

test_stmt_queryid_has_explain_template() {
  local qid
  qid=$(ssh_node1 "/home/kingbase/cluster/install/kingbase/bin/ksql -p 54321 -U system test -AXtc \"SELECT queryid::text FROM sys_stat_statements LIMIT 1;\"" 2>/dev/null | tr -d '[:space:]')
  [[ -z "$qid" ]] && { echo "  [SKIP] sys_stat_statements empty"; _pass; return; }
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stmt $qid")
  assert_contains "$out" "验证路径"
  assert_contains "$out" "EXPLAIN"
}

test_stmt_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json stmt")
  assert_json_valid "$out"
  assert_contains "$out" '"command":"stmt"'
}

test_stmt_no_extension_graceful() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json stmt" || true)
  assert_not_contains "$out" ": line "
  assert_not_contains "$out" "command not found"
}

test_stmt_no_ext_shows_hint() {
  # 如果扩展不可用，输出应包含 shared_preload_libraries
  # 如果扩展可用，此测试直接 skip
  local avail
  avail=$(ssh_node1 "/home/kingbase/cluster/install/kingbase/bin/ksql -p 54321 -U system test -AXtc \"SELECT count(*) FROM sys_catalog.sys_class WHERE relname='sys_stat_statements';\"" 2>/dev/null | tr -d '[:space:]')
  [[ "${avail:-0}" -gt 0 ]] && { _pass; return; }  # extension present, skip
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stmt" || true)
  assert_contains "$out" "shared_preload_libraries"
}
