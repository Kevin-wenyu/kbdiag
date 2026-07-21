# test_obj.sh — tests for "kbdiag obj" command

test_obj_setup_table() {
  # Create the test table on node1
  setup_test_table
  _pass
}

test_obj_runs_with_table() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE obj kbdiag_test")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "obj kbdiag_test exited $code"
}

test_obj_has_table_name_in_output() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE obj kbdiag_test")
  assert_contains "$out" "kbdiag_test"
}

test_obj_has_size_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE obj kbdiag_test")
  assert_contains "$out" "Dead tuples"
}

test_obj_has_vacuum_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE obj kbdiag_test")
  assert_contains "$out" "Vacuum"
}

test_obj_has_indexes_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE obj kbdiag_test")
  assert_contains "$out" "index(es)"
}

test_obj_has_constraints_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE obj kbdiag_test")
  assert_contains "$out" "constraint(s)"
}

test_obj_schema_dot_table_form() {
  # "public.kbdiag_test" form should work the same
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE obj public.kbdiag_test")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "obj public.kbdiag_test exited $code"
}

test_obj_schema_dot_table_contains_table_name() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE obj public.kbdiag_test")
  assert_contains "$out" "kbdiag_test"
}

test_obj_no_argument_exits_nonzero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE obj")
  [[ "${code:-0}" -ne 0 ]] && _pass || _fail "obj with no argument should exit non-zero, got $code"
}

test_obj_verbose_shows_column_stats() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -v obj kbdiag_test")
  assert_contains "$out" "Column statistics"
}

test_obj_nonexistent_table_returns_error() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE obj public.kbdiag_nonexistent_xyz")
  assert_exit_code 1 "$code"
}

test_obj_has_column_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE obj kbdiag_test")
  assert_contains "$out" "total_size"
}

test_obj_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json obj kbdiag_test")
  assert_json_valid "$out"
  assert_contains "$out" '"command":"obj"'
}

test_obj_pretty_size_has_space() {
  # pg_size_pretty text must not be corrupted by blanket space-stripping
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE obj kbdiag_test")
  echo "$out" | grep -qE '[0-9] (MB|KB|GB|bytes)' && _pass || _fail "pretty size missing space: $(echo "$out" | grep -E 'MB|KB|GB' | head -1)"
}

test_obj_invalid_identifier_rejected() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE obj 'bad;name'")
  assert_exit_code 1 "${code:-0}"
}

test_obj_pk_present_shows_ok() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE obj kbdiag_test")
  assert_contains "$out" "PRIMARY KEY present"
}
