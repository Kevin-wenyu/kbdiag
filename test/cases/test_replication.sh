# test_replication.sh — tests for "kbdiag replication" command

test_replication_default_exits_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE replication")
  assert_exit_code 0 "${code:-0}"
}

test_replication_shows_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE replication")
  assert_contains "$out" "Replication"
}

test_replication_verbose_has_header_or_ok() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -v replication")
  # On primary with standby: column header "client_addr" appears
  # On primary without standby: "No standbys connected"
  # On standby: "WAL receiver" or "Replay" lines appear
  if echo "$out" | grep -qE 'client_addr|No standbys|WAL receiver|Replay'; then
    _pass
  else
    _fail "no expected replication output; got: $(echo "$out" | head -5)"
  fi
}

test_replication_standby_exits_zero() {
  local code; code=$(ssh_node2_exit "$KBDIAG_REMOTE replication")
  assert_exit_code 0 "${code:-0}"
}

# ── replication 补强 ─────────────────────────────────────────

test_replication_primary_shows_standby_count() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE replication")
  if echo "$out" | grep -qE '[0-9]+ standby|No standbys|client_addr'; then
    _pass
  else
    _fail "primary replication output missing standby info: $(echo "$out" | head -5)"
  fi
}

test_replication_standby_shows_lag() {
  local out; out=$(ssh_node2 "$KBDIAG_REMOTE replication")
  if echo "$out" | grep -qiE 'lag|delay|replay|behind'; then
    _pass
  else
    _fail "standby replication output missing lag info: $(echo "$out" | head -5)"
  fi
}

test_replication_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json replication")
  assert_json_valid "$out"
}

test_replication_exit_code_flag_healthy_is_zero() {
  # Test env replication is healthy — --exit-code must still be 0.
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE --exit-code replication")
  assert_exit_code 0 "$code"
}

test_replication_verbose_column_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -v replication")
  if echo "$out" | grep -qiE 'write_lag|flush_lag|replay_lag|client_addr|sent_lsn|No standbys'; then
    _pass
  else
    _fail "verbose replication missing column headers: $(echo "$out" | head -8)"
  fi
}
