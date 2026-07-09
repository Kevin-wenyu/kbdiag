# test_partition.sh — tests for "kbdiag partition" command

test_partition_exits_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE partition")
  assert_exit_code 0 "${code:-0}"
}

test_partition_shows_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE partition")
  assert_contains "$out" "Partition Health"
}

test_partition_handles_empty_or_lists() {
  # Either no partitioned tables (test env default) or a real inventory line
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE partition")
  if echo "$out" | grep -qE 'No partitioned tables found|\(list,|\(range,|\(hash,|0 partitions'; then
    _pass
  else
    _fail "unexpected partition output: $(echo "$out" | head -3)"
  fi
}

test_partition_works_on_standby() {
  local code; code=$(ssh_node2_exit "$KBDIAG_REMOTE partition")
  assert_exit_code 0 "${code:-0}"
}
