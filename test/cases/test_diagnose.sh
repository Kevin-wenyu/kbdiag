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
