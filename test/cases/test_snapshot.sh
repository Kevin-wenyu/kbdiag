# test_snapshot.sh — tests for "kbdiag snapshot" command
# Snapshot takes ~12s, so tests share one generated archive.

_SNAP_PATH="/tmp/kbdiag_test_snapshot.tar.gz"
_SNAP_EXIT=""

_snap_ensure() {
  [[ -n "$_SNAP_EXIT" ]] && return 0
  _SNAP_EXIT=$(ssh_node1_exit "$KBDIAG_REMOTE snapshot $_SNAP_PATH")
}

test_snapshot_exits_zero() {
  _snap_ensure
  assert_exit_code 0 "${_SNAP_EXIT:-1}"
}

test_snapshot_tar_lists() {
  _snap_ensure
  local out; out=$(ssh_node1 "tar -tzf $_SNAP_PATH")
  assert_contains "$out" "metadata.txt"
}

test_snapshot_has_expected_files() {
  _snap_ensure
  local out; out=$(ssh_node1 "tar -tzf $_SNAP_PATH")
  local missing="" f
  for f in metadata.txt status.txt check.txt sessions.txt locks.txt wait.txt \
           perf.txt progress.txt temp.txt stat.txt replication.txt cluster.txt \
           params.txt log_tail.txt; do
    echo "$out" | grep -q "/$f" || missing="$missing $f"
  done
  if [[ -z "$missing" ]]; then
    _pass
  else
    _fail "files missing:$missing"
  fi
}

test_snapshot_metadata_content() {
  _snap_ensure
  local out; out=$(ssh_node1 "tar -xzOf $_SNAP_PATH --wildcards '*/metadata.txt'")
  if echo "$out" | grep -q "db_version: KingbaseES" && echo "$out" | grep -qE "role: (primary|standby)"; then
    _pass
  else
    _fail "metadata incomplete: $(echo "$out" | head -6)"
  fi
}

test_snapshot_log_tail_redacted() {
  _snap_ensure
  # any surviving quoted literal longer than a few chars means redaction failed
  local cnt
  cnt=$(ssh_node1 "tar -xzOf $_SNAP_PATH --wildcards '*/log_tail.txt' | grep -cE \"'[^*'][^']{3,}'\"" | tr -d '[:space:]')
  assert_exit_code 0 "${cnt:-1}"
}

test_snapshot_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE snapshot /tmp/kbdiag_test_snapshot_json.tar.gz --format json")
  assert_json_valid "$out"
}

test_snapshot_works_on_standby() {
  local code; code=$(ssh_node2_exit "$KBDIAG_REMOTE snapshot /tmp/kbdiag_test_snapshot_sb.tar.gz")
  assert_exit_code 0 "${code:-1}"
}
