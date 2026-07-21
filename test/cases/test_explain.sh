# test_explain.sh — tests for "kbdiag explain" command

test_explain_no_arg_shows_usage() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE explain")
  assert_contains "$out" "Usage"
}

test_explain_no_arg_exits_nonzero() {
  # Missing required argument is a usage error — always non-zero, regardless
  # of --exit-code (matches idx's unknown-subcommand / sql's invalid-pid convention)
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE explain")
  assert_exit_code 1 "${code:-0}"
}

test_explain_sql_shows_plan() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE explain \"SELECT count(*) FROM sys_class\"")
  assert_contains "$out" "Plan:"
}

test_explain_sql_flags_seq_scan() {
  # sys_class's plan depends on catalog size/stats and drifts over a long-lived
  # test DB (index-only scan once the catalog grows enough) — use kbdiag_test's
  # unindexed "data" column instead, which is always a guaranteed seq scan.
  setup_test_table
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE explain \"SELECT count(*) FROM kbdiag_test WHERE data = 'zzz_no_such_value_xyz'\"")
  assert_contains "$out" "sequential scan"
}

test_explain_sql_shows_planner_estimate() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE explain \"SELECT count(*) FROM sys_class\"")
  assert_contains "$out" "Planner estimate"
}

test_explain_invalid_sql_fails_gracefully() {
  local out code
  code=$(ssh_node1_exit "$KBDIAG_REMOTE explain \"SELECT FROM no_such_table_xyz\"")
  assert_exit_code 0 "${code:-0}"
  out=$(ssh_node1 "$KBDIAG_REMOTE explain \"SELECT FROM no_such_table_xyz\"")
  assert_contains "$out" "EXPLAIN failed"
}

test_explain_unknown_queryid_warns() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE explain 999999999")
  # Either stmt extension missing (graceful skip) or queryid-not-found warning
  if echo "$out" | grep -qE 'not found|sys_stat_statements'; then _pass; else _fail "no graceful queryid handling: $(echo "$out" | head -3)"; fi
}

test_explain_dml_is_not_executed() {
  # EXPLAIN (without ANALYZE) must not execute the DML
  ssh_node1 "$KBDIAG_REMOTE explain \"CREATE TABLE kbdiag_explain_probe(i int)\"" >/dev/null 2>&1 || true
  local out; out=$(ssh_node1 "ksql -p 54321 -U system test -AXtc \"SELECT count(*) FROM sys_class WHERE relname='kbdiag_explain_probe';\"")
  # CREATE TABLE isn't EXPLAINable → EXPLAIN failed; table must not exist either way
  assert_contains "$out" "0"
}

test_explain_seq_scan_exit_code_flag() {
  setup_test_table
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE --exit-code explain \"SELECT count(*) FROM kbdiag_test WHERE data = 'zzz_no_such_value_xyz'\"")
  assert_exit_code 1 "$code"
}

test_explain_seq_scan_default_exit_zero() {
  setup_test_table
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE explain \"SELECT count(*) FROM kbdiag_test WHERE data = 'zzz_no_such_value_xyz'\"")
  assert_exit_code 0 "$code"
}

test_explain_fail_exit_code_flag() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE --exit-code explain \"SELECT FROM no_such_table_xyz\"")
  assert_exit_code 2 "$code"
}

test_explain_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json explain \"SELECT count(*) FROM sys_class\"")
  assert_json_valid "$out"
  assert_contains "$out" '"command":"explain"'
}

test_explain_json_no_arg_exits_nonzero() {
  # warn() redirects to stderr in JSON mode (dropped by the test harness),
  # so only the exit code is observable here
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE --format json explain")
  assert_exit_code 1 "${code:-0}"
}
