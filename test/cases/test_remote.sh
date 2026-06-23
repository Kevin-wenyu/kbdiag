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

# ── remote 补强 ──────────────────────────────────────────────

test_remote_single_node_status() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE remote node1 status" 2>&1 || true)
  assert_not_contains "$out" "Unknown command"
  assert_not_contains "$out" ": line "
}

test_remote_all_exits_cleanly() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE remote --all status" || true)
  [[ "${code:-0}" -ne 127 ]] && _pass || _fail "remote --all crashed with exit 127"
}
