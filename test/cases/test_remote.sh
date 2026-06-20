# test_remote.sh — tests for kbdiag remote command

test_remote_explicit_node() {
  # Run status on node2 (standby) via explicit IP
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE remote 192.168.105.11 status")
  assert_contains "$out" "STANDBY"
}

test_remote_connection_failure() {
  # Unreachable IP should warn gracefully, not crash
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE remote 192.168.105.99 status")
  assert_contains "$out" "connection failed or kbdiag not deployed"
}

test_remote_no_subcommand() {
  # Missing command argument — should exit non-zero
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE remote 192.168.105.10")
  assert_exit_code 1 "$code"
}

test_remote_all_from_nodes_conf() {
  # Create ~/.kbdiag/nodes.conf on node1 with self (loopback) and run --all status
  ssh_node1 "mkdir -p ~/.kbdiag && printf '192.168.105.10\n' > ~/.kbdiag/nodes.conf"
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE remote --all status")
  assert_contains "$out" "=== [192.168.105.10] ==="
  # Clean up
  ssh_node1 "rm -f ~/.kbdiag/nodes.conf" || true
}
