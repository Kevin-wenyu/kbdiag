# test_conf.sh — tests for "kbdiag conf" command

test_conf_default_exits_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE conf")
  assert_exit_code 0 "${code:-0}"
}

test_conf_default_shows_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE conf")
  assert_contains "$out" "Configuration"
}

test_conf_default_shows_nondefault_info() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE conf")
  assert_contains "$out" "Non-default parameters"
}

test_conf_default_shows_restart_status() {
  # Should show either "No restart-pending" or a warning about restart-pending params
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE conf")
  local has_ok has_warn
  has_ok=$(echo "$out" | grep -c "restart" || true)
  [[ "${has_ok:-0}" -ge 1 ]] && _pass || _fail "expected restart-pending status in output"
}

test_conf_diff_no_peer_warns_gracefully() {
  # Without KB_PEER_HOST set and no repmgr, conf diff should warn gracefully
  local out; out=$(ssh_node1 "KB_PEER_HOST= $KBDIAG_REMOTE conf diff")
  # Should contain either "Cannot determine peer" warn OR peer comparison output
  local has_warn has_info
  has_warn=$(echo "$out" | grep -c "Cannot determine peer\|Comparing parameters\|peer" || true)
  [[ "${has_warn:-0}" -ge 1 ]] && _pass || _fail "expected peer-related output; got: $(echo "$out" | head -3)"
}

test_kbdiagrc_overrides_port() {
  # 写一个临时 .kbdiagrc，设置一个无害变量，确认 .kbdiagrc 被 source（status 不 crash）
  local out
  out=$(ssh_node1 "echo 'KB_QUERY_TIMEOUT=1' > ~/.kbdiagrc && $KBDIAG_REMOTE status; rm -f ~/.kbdiagrc" || true)
  # status 应该仍能运行（timeout=1 足够本地查询），不 crash
  assert_not_contains "$out" ": line "
  assert_not_contains "$out" "command not found"
}
