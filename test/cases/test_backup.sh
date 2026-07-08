# test_backup.sh — tests for "kbdiag backup" command

test_backup_exits_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE backup")
  assert_exit_code 0 "${code:-0}"
}

test_backup_shows_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE backup")
  assert_contains "$out" "Backup"
}

test_backup_shows_archive_mode() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE backup")
  assert_contains "$out" "archive_mode"
}

test_backup_shows_wal_level() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE backup")
  assert_contains "$out" "wal_level"
}

test_backup_shows_slot_status() {
  # Either inactive slots are listed or the OK line appears
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE backup")
  if echo "$out" | grep -qE 'replication slot'; then _pass; else _fail "no replication slot status in output"; fi
}

test_backup_archiver_verdict_present() {
  # archive_mode=on on the test VM, so one of the archiver verdict lines must appear
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE backup")
  if echo "$out" | grep -qE 'Archiver (is FAILING|recovered|healthy|has not archived)'; then
    _pass
  else
    _fail "no archiver verdict in output: $(echo "$out" | head -5)"
  fi
}

test_backup_works_on_standby() {
  # Must degrade gracefully on the standby node too
  local code; code=$(ssh_node2_exit "$KBDIAG_REMOTE backup")
  assert_exit_code 0 "${code:-0}"
}
