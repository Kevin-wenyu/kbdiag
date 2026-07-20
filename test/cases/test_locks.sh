# test_locks.sh — tests for "kbdiag locks" subcommands

# ── wait (default) ────────────────────────────────────────────────────────────

test_locks_wait_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE locks wait")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "locks wait exited $code"
}

test_locks_default_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE locks")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "locks (default) exited $code"
}

test_locks_wait_no_locks_shows_ok() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE locks wait")
  assert_contains "$out" "Waiting locks: none"
}

# ── hold ─────────────────────────────────────────────────────────────────────

test_locks_hold_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE locks hold")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "locks hold exited $code"
}

test_locks_hold_shows_locks() {
  # Setup a table so there are at least some lock-holding sessions possible.
  # Even with no active locks the command should run and produce header output.
  setup_test_table
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE locks hold")
  # Header should always appear
  assert_contains "$out" "Lock-holding"
}

# ── deadlock ──────────────────────────────────────────────────────────────────

test_locks_deadlock_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE locks deadlock")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "locks deadlock exited $code"
}

test_locks_deadlock_output() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE locks deadlock")
  # Should show either no-deadlocks OK or a warning — command must produce output
  local lines; lines=$(echo "$out" | grep -v '^$' | wc -l | tr -d ' ')
  [[ "${lines:-0}" -ge 1 ]] && _pass || _fail "locks deadlock produced no output"
}

# ── global flags ──────────────────────────────────────────────────────────────

test_locks_top_n() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --top 2 locks hold")
  # Should not error; we can't guarantee rows but command must succeed
  local code=$?
  [[ $code -eq 0 ]] && _pass || _fail "locks hold --top 2 exited $code"
}

test_locks_unknown_subcmd_errors() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE locks badcmd")
  [[ "${code:-0}" -ne 0 ]] && _pass || _fail "unknown locks subcommand should exit non-zero"
}

test_locks_hold_has_header_or_ok() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE locks hold" 2>&1)
  echo "$out" | grep -qE 'PID|Lock-holding sessions: none' && _pass || _fail "missing 'pid' header or 'No lock-holding' message"
}

test_locks_wait_has_header_or_ok() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE locks wait" 2>&1)
  echo "$out" | grep -qE 'WAIT_PID|Waiting locks: none' && _pass || _fail "missing 'wait_pid' header or 'No waiting' message"
}

test_locks_exit_code_flag_healthy() {
  # without contention, --exit-code must still exit 0
  local code
  code=$(ssh_node1_exit "$KBDIAG_REMOTE --exit-code locks")
  [[ "${code:-1}" -eq 0 ]] && _pass || _fail "locks --exit-code healthy should exit 0, got $code"
}

test_locks_deadlock_verdict_has_context() {
  # output contract: verdict carries measured value and scope
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE locks deadlock" 2>&1)
  echo "$out" | grep -q "since stats reset" && _pass || _fail "deadlock verdict missing 'since stats reset' context"
}
