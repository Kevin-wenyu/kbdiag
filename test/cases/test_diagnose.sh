test_diagnose_exit_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE diagnose")
  assert_exit_code 0 "$code"
}

test_diagnose_has_summary_or_clean() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  if echo "$out" | grep -qE '● 发现|● 未发现异常'; then
    _pass
  else
    _fail "expected summary line '● 发现...' or '● 未发现异常', got: $(echo "$out" | head -5)"
  fi
}

test_diagnose_has_finding_or_clean() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  if echo "$out" | grep -qE '\[CRITICAL\]|\[WARN\]|\[INFO\]|未发现异常'; then
    _pass
  else
    _fail "expected at least one finding level marker or '未发现异常'"
  fi
}

test_diagnose_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json diagnose")
  assert_json_valid "$out"
  assert_contains "$out" '"command":"diagnose"'
  assert_contains "$out" '"findings":'
}

test_diagnose_json_findings_array() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json diagnose")
  assert_json_valid "$out"
  if echo "$out" | grep -qE '"findings":\[\]|"level":'; then
    _pass
  else
    _fail "findings field should be empty array or contain objects with 'level'"
  fi
}

test_diagnose_full_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json diagnose --full")
  assert_json_valid "$out"
  assert_contains "$out" '"mode":"full"'
}

test_diagnose_standalone_no_crash() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  assert_not_contains "$out" "command not found"
  assert_not_contains "$out" "Error:"
  assert_not_contains "$out" ": line "
}

test_diagnose_slow_hist_no_crash() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose" || true)
  assert_not_contains "$out" ": line "
  assert_not_contains "$out" "unbound variable"
  assert_not_contains "$out" "command not found"
}

test_diagnose_full_stmt_top_no_crash() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose --full" || true)
  assert_not_contains "$out" ": line "
  assert_not_contains "$out" "unbound variable"
}

# ── 子检查逐项断言 ────────────────────────────────────────────

test_diagnose_section_long_txn() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  assert_contains "$out" "长事务"
}

test_diagnose_section_connections() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  assert_contains "$out" "连接数"
}

test_diagnose_section_locks() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  assert_contains "$out" "锁"
}

test_diagnose_section_slow_queries() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  assert_contains "$out" "慢查询"
}

test_diagnose_section_replication() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  # primary shows replication section; standalone shows skip label
  if echo "$out" | grep -qE '复制|replication|standalone'; then
    _pass
  else
    _fail "expected replication section or standalone label; got: $(echo "$out" | tail -5)"
  fi
}

test_diagnose_section_disk() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  assert_contains "$out" "磁盘"
}

test_diagnose_section_xid_age() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  assert_contains "$out" "XID"
}

test_diagnose_section_vacuum_debt() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose --full")
  assert_contains "$out" "真空"
}

test_diagnose_section_bloat() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose --full")
  assert_contains "$out" "膨胀"
}

test_diagnose_section_buffer_hit() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose --full")
  assert_contains "$out" "缓冲"
}

test_diagnose_section_checkpoint() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose --full")
  if echo "$out" | grep -qiE 'checkpoint|检查点'; then
    _pass
  else
    _fail "expected checkpoint section; got: $(echo "$out" | tail -5)"
  fi
}

test_diagnose_section_temp() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose --full")
  assert_contains "$out" "临时文件"
}

test_diagnose_section_params() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose --full")
  assert_contains "$out" "参数"
}

test_diagnose_section_indexes() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose --full")
  assert_contains "$out" "索引"
}

test_diagnose_section_stmt_top_skip_or_data() {
  # sys_stat_statements 未启用时应显示跳过提示；启用时显示 top SQL
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose --full" || true)
  if echo "$out" | grep -qE 'sys_stat_statements|stmt|Top SQL|跳过'; then
    _pass
  else
    _fail "expected stmt_top section or skip message; got: $(echo "$out" | tail -5)"
  fi
}

test_diagnose_archiver_finding_consistent_with_backup() {
  # 环境自适应：archiver 失败时 diagnose 必须给出归档根因链；
  # archiver 健康时不应出现该 finding（以 backup 的裁决为基准）
  local diag bk
  diag=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  bk=$(ssh_node1 "$KBDIAG_REMOTE backup")
  if echo "$bk" | grep -q "Archiver is FAILING"; then
    assert_contains "$diag" "WAL 归档失败"
  else
    assert_not_contains "$diag" "WAL 归档失败"
  fi
}
