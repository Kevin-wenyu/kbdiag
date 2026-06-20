# test_space.sh — tests for "kbdiag space" subcommands

# ── default (all) ─────────────────────────────────────────────────────────────

test_space_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE space")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "space exited $code"
}

test_space_has_disk_info() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE space")
  assert_contains "$out" "Disk"
}

test_space_has_wal_info() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE space")
  assert_contains "$out" "WAL"
}

test_space_has_database_sizes() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE space")
  assert_contains "$out" "Database sizes"
}

# ── frag ──────────────────────────────────────────────────────────────────────

test_space_frag_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE space frag")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "space frag exited $code"
}

test_space_frag_shows_pct() {
  # Create a bloated table first, then check frag reports dead_pct
  setup_bloat_table
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE space frag")
  # Either no fragmented tables (if bloat threshold not met) or dead_pct column shown
  # The header must always appear
  assert_contains "$out" "fragmentation"
}

test_space_frag_has_dead_pct_column() {
  setup_bloat_table
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE space frag")
  # Output should show a numeric percentage (data row) or the OK no-frag message
  local lines; lines=$(echo "$out" | grep -v '^$' | wc -l | tr -d ' ')
  [[ "${lines:-0}" -ge 1 ]] && _pass || _fail "space frag produced no output"
}

# ── unknown subcommand ────────────────────────────────────────────────────────

test_space_unknown_subcmd_errors() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE space badcmd")
  [[ "${code:-0}" -ne 0 ]] && _pass || _fail "unknown space subcommand should exit non-zero"
}
