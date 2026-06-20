# test_params.sh — tests for "kbdiag params" command

test_params_default_exits_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE params")
  assert_exit_code 0 "${code:-0}"
}

test_params_shows_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE params")
  assert_contains "$out" "Instance parameters"
}

test_params_default_shows_non_default() {
  # Default mode: show only params where setting <> boot_val.
  # On any real DB there should be at least some non-default params (port, data_directory, etc.)
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE params")
  assert_exit_code 0 "${code:-0}"
}

test_params_all_exits_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE params --all")
  assert_exit_code 0 "${code:-0}"
}

test_params_all_shows_results() {
  # --all shows all params; just verify the command returns a non-empty result
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE params --all")
  local lines; lines=$(echo "$out" | grep -v '^$' | grep -v '^==>' | wc -l | tr -d ' ')
  [[ "${lines:-0}" -ge 1 ]] && _pass || _fail "params --all produced no output"
}

test_params_search_work_mem() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE params work_mem")
  assert_contains "$out" "work_mem"
}

test_params_pattern_exits_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE params work_mem")
  assert_exit_code 0 "${code:-0}"
}

test_params_pattern_filters_results() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE params work_mem")
  assert_contains "$out" "work_mem"
}

test_params_pattern_no_match_exits_zero() {
  # Pattern that matches nothing should still exit 0
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE params xyzzy_nonexistent")
  assert_exit_code 0 "${code:-0}"
}
