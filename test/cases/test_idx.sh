# test_idx.sh — tests for "kbdiag idx"

test_idx_all_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE idx")
  assert_exit_code 0 "$code"
}

test_idx_all_has_headers() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE idx")
  assert_contains "$out" "Unused"
}

test_idx_unused_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE idx unused")
  assert_exit_code 0 "$code"
}

test_idx_unused_shows_ok_when_none() {
  # System may or may not have unused indexes; just check it doesn't crash
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE idx unused")
  # Should contain either [OK] or [WARN]
  echo "$out" | grep -qE '\[OK\]|\[WARN\]' && _pass || _fail "no status line in: $out"
}

test_idx_dup_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE idx dup")
  assert_exit_code 0 "$code"
}

test_idx_bloat_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE idx bloat")
  assert_exit_code 0 "$code"
}

test_idx_missing_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE idx missing")
  assert_exit_code 0 "$code"
}

test_idx_top_n() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --top 2 idx unused")
  # Result rows (excluding header and blank lines) <= 2
  local rows; rows=$(echo "$out" | grep -v '^$' | grep -v '==>' | grep -v '\[OK\]' | grep -v '\[WARN\]' | wc -l | tr -d ' ')
  [[ "${rows:-0}" -le 2 ]] && _pass || _fail "expected <=2 rows with --top 2, got $rows"
}

test_idx_quiet_hides_ok() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -q idx unused")
  assert_not_contains "$out" "[OK]"
}

test_idx_unused_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE idx unused" 2>&1)
  # Either "No unused indexes" OK message, or table with header
  echo "$out" | grep -qE 'indexrelname|No unused' && _pass || _fail "missing 'indexrelname' header or 'No unused' message"
}

test_idx_bloat_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE idx bloat" 2>&1)
  echo "$out" | grep -qE 'schemaname|No highly bloated' && _pass || _fail "missing 'schemaname' header or 'No highly bloated' message"
}

test_idx_missing_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE idx missing" 2>&1)
  echo "$out" | grep -qE 'fk_col|No missing FK' && _pass || _fail "missing 'fk_col' header or 'No missing FK' message"
}

test_idx_unknown_sub_fails() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE idx badsubcmd 2>/dev/null" 2>/dev/null || true)
  [[ "${code:-0}" -ne 0 ]] && _pass || _fail "expected non-zero exit for unknown subcommand"
}

test_idx_default_exit_zero_even_with_warn() {
  # Query-layer default: exit 0 regardless of verdict, unless --exit-code is passed
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE idx bloat")
  assert_exit_code 0 "$code"
}

test_idx_exit_code_flag_reflects_verdict() {
  local out code
  out=$(ssh_node1 "$KBDIAG_REMOTE idx bloat")
  code=$(ssh_node1_exit "$KBDIAG_REMOTE --exit-code idx bloat")
  if echo "$out" | grep -q '\[WARN\]'; then
    assert_exit_code 1 "$code"
  else
    assert_exit_code 0 "$code"
  fi
}

test_idx_verbose_shows_full_list() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -v idx bloat")
  assert_not_contains "$out" "use -v for full list"
}

test_idx_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json idx")
  assert_json_valid "$out"
  assert_contains "$out" '"command":"idx"'
}

test_idx_json_no_table_leakage() {
  # Raw column -t table text must not appear in JSON output
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json idx")
  assert_not_contains "$out" "indexrelname  "
}

test_idx_json_rows_use_raw_bytes() {
  # index_size must be raw numeric bytes, not "13 MB"-style pretty text
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json idx")
  assert_not_contains "$out" "index_size\":\""
}
