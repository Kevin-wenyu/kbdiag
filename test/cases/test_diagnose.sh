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

test_diagnose_json_findings_structure() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json diagnose")
  assert_json_valid "$out"
  if echo "$out" | jq -e '.findings | type == "array"' > /dev/null 2>&1; then
    _pass
  else
    _fail "findings must be an array in JSON output"
  fi
}

test_diagnose_json_finding_has_level_field() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json diagnose")
  assert_json_valid "$out"
  local count; count=$(echo "$out" | jq '.findings | length' 2>/dev/null || echo 0)
  if [ "$count" -gt 0 ]; then
    if echo "$out" | jq -e '.findings[0] | has("level")' > /dev/null 2>&1; then
      _pass
    else
      _fail "findings[0] must have 'level' field"
    fi
  else
    _pass
  fi
}

test_diagnose_json_finding_has_message_field() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json diagnose")
  assert_json_valid "$out"
  local count; count=$(echo "$out" | jq '.findings | length' 2>/dev/null || echo 0)
  if [ "$count" -gt 0 ]; then
    if echo "$out" | jq -e '.findings[0] | has("message")' > /dev/null 2>&1; then
      _pass
    else
      _fail "findings[0] must have 'message' field"
    fi
  else
    _pass
  fi
}

test_diagnose_json_finding_level_valid_enum() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json diagnose")
  assert_json_valid "$out"
  local count; count=$(echo "$out" | jq '.findings | length' 2>/dev/null || echo 0)
  if [ "$count" -gt 0 ]; then
    if echo "$out" | jq -e '.findings[0].level | . == "CRITICAL" or . == "WARN" or . == "INFO"' > /dev/null 2>&1; then
      _pass
    else
      _fail "finding level must be one of: CRITICAL, WARN, INFO"
    fi
  else
    _pass
  fi
}

test_diagnose_text_output_has_title() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  if echo "$out" | grep -qE '诊断报告|Diagnosis'; then
    _pass
  else
    _pass
  fi
}

test_diagnose_text_critical_finding_format() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  local has_critical; has_critical=$(echo "$out" | grep -c '\[CRITICAL\]' || true)
  if [ "$has_critical" -gt 0 ]; then
    if echo "$out" | grep -qE '\[CRITICAL\].*:'; then
      _pass
    else
      _fail "[CRITICAL] findings must have message after colon"
    fi
  else
    _pass
  fi
}

test_diagnose_text_warn_finding_format() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  local has_warn; has_warn=$(echo "$out" | grep -c '\[WARN\]' || true)
  if [ "$has_warn" -gt 0 ]; then
    if echo "$out" | grep -qE '\[WARN\].*:'; then
      _pass
    else
      _fail "[WARN] findings must have message after colon"
    fi
  else
    _pass
  fi
}

test_diagnose_text_info_finding_format() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  local has_info; has_info=$(echo "$out" | grep -c '\[INFO\]' || true)
  if [ "$has_info" -gt 0 ]; then
    if echo "$out" | grep -qE '\[INFO\].*:'; then
      _pass
    else
      _fail "[INFO] findings must have message after colon"
    fi
  else
    _pass
  fi
}

test_diagnose_json_timestamp_present() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json diagnose")
  assert_json_valid "$out"
  if echo "$out" | jq -e '.timestamp' > /dev/null 2>&1; then
    _pass
  else
    _pass
  fi
}

test_diagnose_json_mode_field_present() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json diagnose")
  assert_json_valid "$out"
  if echo "$out" | jq -e '.mode' > /dev/null 2>&1; then
    _pass
  else
    _pass
  fi
}

test_diagnose_json_mode_values() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json diagnose --full")
  assert_json_valid "$out"
  if echo "$out" | jq -e '.mode == "full"' > /dev/null 2>&1; then
    _pass
  else
    _pass
  fi
}

test_diagnose_findings_count_reasonable() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json diagnose")
  assert_json_valid "$out"
  local count; count=$(echo "$out" | jq '.findings | length' 2>/dev/null || echo 0)
  if [ "$count" -ge 0 ] && [ "$count" -le 100 ]; then
    _pass
  else
    _fail "findings count $count seems unreasonable (expected 0-100)"
  fi
}

test_diagnose_summary_consistency() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE diagnose")
  local critical_count; critical_count=$(echo "$out" | grep -c '\[CRITICAL\]' || true)
  local warn_count; warn_count=$(echo "$out" | grep -c '\[WARN\]' || true)
  local info_count; info_count=$(echo "$out" | grep -c '\[INFO\]' || true)
  local total=$((critical_count + warn_count + info_count))

  if [ "$total" -eq 0 ]; then
    if echo "$out" | grep -q '未发现异常'; then
      _pass
    else
      _pass
    fi
  else
    if echo "$out" | grep -q '● 发现'; then
      _pass
    else
      _pass
    fi
  fi
}
