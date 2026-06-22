DEPLOY_SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/scripts/deploy.sh"

test_deploy_syntax_ok() {
  bash -n "$DEPLOY_SCRIPT"
  _pass
}

test_deploy_usage_no_args() {
  local out; out=$(bash "$DEPLOY_SCRIPT" 2>&1 || true)
  assert_contains "$out" "Usage: bash scripts/deploy.sh <hosts-file>"
}

test_deploy_usage_mentions_ssh_port() {
  local out; out=$(bash "$DEPLOY_SCRIPT" 2>&1 || true)
  assert_contains "$out" "SSH_PORT"
}

test_deploy_missing_hosts_file() {
  local out; out=$(bash "$DEPLOY_SCRIPT" /tmp/nonexistent_hosts_file_kbdiag 2>&1 || true)
  assert_contains "$out" "Error: hosts file not found"
}
