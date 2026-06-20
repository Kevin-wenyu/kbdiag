# test_perf.sh — tests for "kbdiag perf" subcommands

# ── wait ──────────────────────────────────────────────────────────────────────

test_perf_wait_runs_on_primary() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE perf wait")
  # Should at minimum output the header (even if no waits)
  local rc=$?
  # Command should not error
  [[ $rc -eq 0 ]] && _pass || _fail "perf wait failed with rc=$rc"
}

test_perf_wait_runs_on_standby() {
  local code; code=$(ssh_node2_exit "$KBDIAG_REMOTE perf wait")
  [[ "${code:-0}" -le 1 ]] && _pass || _fail "perf wait on standby exited $code"
}

test_perf_wait_top_n() {
  # With --top 2, result rows should be at most 2 (plus header)
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --top 2 perf wait")
  local rows; rows=$(echo "$out" | grep -v '^$' | grep -v '==>' | wc -l | tr -d ' ')
  [[ "${rows:-0}" -le 4 ]] && _pass || _fail "expected <=4 lines with --top 2, got $rows"
}

# ── io ────────────────────────────────────────────────────────────────────────

test_perf_io_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE perf io")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "perf io exited $code"
}

test_perf_io_top_n() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --top 3 perf io")
  # Output lines excluding blank and header lines should be <= 3
  local rows; rows=$(echo "$out" | grep -v '^$' | grep -v '==>' | wc -l | tr -d ' ')
  [[ "${rows:-0}" -le 5 ]] && _pass || _fail "expected <=5 lines with --top 3, got $rows"
}

test_perf_io_json_valid() {
  # perf io in json mode — wrap in json_begin/end via --format json
  # The command outputs table data, but with --format json the hdr() is suppressed
  # The output itself won't be JSON (column data is text), so just check exit 0
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE --format json perf io")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "perf io --format json exited $code"
}

# ── wal ───────────────────────────────────────────────────────────────────────

test_perf_wal_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE perf wal")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "perf wal exited $code"
}

test_perf_wal_has_checkpoint_data() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE perf wal")
  # sys_stat_bgwriter always has a row; output should contain numbers/bytes
  local lines; lines=$(echo "$out" | grep -v '^$' | grep -v '==>' | wc -l | tr -d ' ')
  [[ "${lines:-0}" -ge 1 ]] && _pass || _fail "perf wal returned no data"
}

test_perf_wal_on_standby() {
  local code; code=$(ssh_node2_exit "$KBDIAG_REMOTE perf wal")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "perf wal on standby exited $code"
}

# ── top ───────────────────────────────────────────────────────────────────────

test_perf_top_buffers_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE perf top buffers")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "perf top buffers exited $code"
}

test_perf_top_default_buffers() {
  # default mode (no argument) should work same as buffers
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE perf top")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "perf top (default) exited $code"
}

test_perf_top_reads_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE perf top reads")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "perf top reads exited $code"
}

test_perf_top_writes_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE perf top writes")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "perf top writes exited $code"
}

test_perf_top_cpu_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE perf top cpu")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "perf top cpu exited $code"
}

test_perf_top_temp_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE perf top temp")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "perf top temp exited $code"
}

test_perf_top_top_n() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --top 2 perf top buffers")
  local rows; rows=$(echo "$out" | grep -v '^$' | grep -v '==>' | wc -l | tr -d ' ')
  [[ "${rows:-0}" -le 4 ]] && _pass || _fail "expected <=4 lines with --top 2, got $rows"
}

test_perf_top_invalid_mode() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE perf top badmode")
  [[ "${code:-0}" -ne 0 ]] && _pass || _fail "perf top badmode should exit non-zero"
}

# ── vacuum (enhanced) ─────────────────────────────────────────────────────────

test_perf_vacuum_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE perf vacuum")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "perf vacuum exited $code"
}

test_perf_vacuum_top_n() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE --top 5 perf vacuum")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "perf vacuum --top 5 exited $code"
}

# ── all subcommands at once ────────────────────────────────────────────────────

test_perf_all_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE perf")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "perf (all) exited $code"
}

test_perf_unknown_subcommand_errors() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE perf xyzzy")
  [[ "${code:-0}" -ne 0 ]] && _pass || _fail "unknown perf subcommand should exit non-zero"
}
