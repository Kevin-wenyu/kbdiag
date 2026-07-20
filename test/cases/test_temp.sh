# test_temp.sh — tests for "kbdiag temp" command

test_temp_exits_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE temp")
  assert_exit_code 0 "${code:-0}"
}

test_temp_shows_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE temp")
  assert_contains "$out" "Temp file usage"
}

test_temp_verdict_present() {
  # output contract: verdict line always present, with values or explicit "none"
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE temp")
  echo "$out" | grep -qE 'Temp usage.*(none recorded|[0-9]+ files)' && _pass \
    || _fail "temp verdict line missing"
}

test_temp_exit_code_always_zero() {
  # presentation-type command: no verdicts to escalate, exit 0 even with --exit-code
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE --exit-code temp")
  assert_exit_code 0 "${code:-0}"
}

test_temp_verbose_shows_settings_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -v temp")
  assert_contains "$out" "Related settings"
}

test_temp_verbose_shows_work_mem_setting() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -v temp")
  assert_contains "$out" "work_mem"
}

test_temp_quiet_flag() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE -q temp")
  assert_exit_code 0 "${code:-0}"
}

test_temp_verbose_has_settings_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -v temp")
  # The "Related settings" table renders under -v (3 rows), so header must appear
  assert_contains "$out" "name"
}
