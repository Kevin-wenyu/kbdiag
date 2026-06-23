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
