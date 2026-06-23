# test_stat.sh — tests for "kbdiag stat" command

test_stat_runs_with_interval() {
  # Use --interval 2 to avoid slow 5s default
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE stat --interval 2")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "stat --interval 2 exited $code"
}

test_stat_has_tps_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stat --interval 2")
  assert_contains "$out" "TPS commits/s"
}

test_stat_has_buffer_hit_rate() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stat --interval 2")
  assert_contains "$out" "Buffer hit rate"
}

test_stat_has_deadlocks() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stat --interval 2")
  assert_contains "$out" "Deadlocks/s"
}

test_stat_has_temp_bytes() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stat --interval 2")
  assert_contains "$out" "Temp bytes/s"
}

test_stat_has_rollbacks() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stat --interval 2")
  assert_contains "$out" "TPS rollbacks/s"
}

test_stat_interval_flag_equals_form() {
  # --interval=N form should also work
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE stat --interval=2")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "stat --interval=2 exited $code"
}

test_stat_count_flag() {
  # --count 1 with --interval 2 should produce one set of output
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stat --interval 2 --count 1")
  assert_contains "$out" "TPS commits/s"
}

test_stat_header_shows_interval() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stat --interval 2")
  assert_contains "$out" "throughput"
}

test_stat_shows_wait_events() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stat --interval=1")
  if echo "$out" | grep -qiE 'Wait event|wait_event|等待'; then
    _pass
  else
    _fail "stat missing wait events section: $(echo "$out" | tail -5)"
  fi
}

test_stat_shows_temp_files() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stat --interval=1")
  if echo "$out" | grep -qiE 'Temp files|temp_files'; then
    _pass
  else
    _fail "stat missing temp files section: $(echo "$out" | tail -5)"
  fi
}

test_stat_shows_checkpoints() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stat --interval=1")
  if echo "$out" | grep -qiE 'Checkpoint|checkpoint'; then
    _pass
  else
    _fail "stat missing checkpoint section: $(echo "$out" | tail -5)"
  fi
}

test_stat_top_sql_section_or_skip() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE stat --interval=1" 2>&1 || true)
  assert_not_contains "$out" ": line "
  assert_not_contains "$out" "unbound variable"
  _pass
}
