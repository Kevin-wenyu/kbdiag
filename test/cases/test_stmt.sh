test_stmt_exit_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE stmt")
  assert_exit_code 0 "$code"
}

test_stmt_has_four_sections() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stmt")
  assert_contains "$out" "by mean execution time"
  assert_contains "$out" "by total execution time"
  assert_contains "$out" "by IO"
  assert_contains "$out" "by call frequency"
}

test_stmt_has_stats_reset() {
  local reset
  reset=$(ssh_node1 "/home/kingbase/cluster/install/kingbase/bin/ksql -p 54321 -U system test -AXtc \"SELECT coalesce(stats_reset::text,'unknown') FROM sys_stat_bgwriter;\"" 2>/dev/null | tr -d '[:space:]')
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stmt")
  assert_contains "$out" "$reset"
}

test_stmt_queryid_exit_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE stmt 0000000000000000")
  assert_exit_code 0 "$code"
}

test_stmt_queryid_has_explain_template() {
  local qid
  qid=$(ssh_node1 "/home/kingbase/cluster/install/kingbase/bin/ksql -p 54321 -U system test -AXtc \"SELECT queryid::text FROM sys_stat_statements LIMIT 1;\"" 2>/dev/null | tr -d '[:space:]')
  [[ -z "$qid" ]] && { echo "  [SKIP] sys_stat_statements empty"; _pass; return; }
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -v stmt $qid")
  assert_contains "$out" "Verification path"
  assert_contains "$out" "EXPLAIN"
}

test_stmt_queryid_default_hides_explain_template() {
  local qid
  qid=$(ssh_node1 "/home/kingbase/cluster/install/kingbase/bin/ksql -p 54321 -U system test -AXtc \"SELECT queryid::text FROM sys_stat_statements LIMIT 1;\"" 2>/dev/null | tr -d '[:space:]')
  [[ -z "$qid" ]] && { echo "  [SKIP] sys_stat_statements empty"; _pass; return; }
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stmt $qid")
  assert_not_contains "$out" "EXPLAIN"
}

test_stmt_not_found_warns() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stmt 999999999")
  assert_contains "$out" "not found"
}

test_stmt_not_found_exit_zero_default() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE stmt 999999999")
  assert_exit_code 0 "$code"
}

test_stmt_not_found_exit_code_flag() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE --exit-code stmt 999999999")
  assert_exit_code 1 "$code"
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
