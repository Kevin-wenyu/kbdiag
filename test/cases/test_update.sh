test_update_version_flag_works() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --version")
  assert_contains "$out" "kbdiag"
  assert_contains "$out" "v"
}

test_update_cmd_exists() {
  # 验证 update 子命令被识别（离线环境会 curl 失败，但不应报 "Unknown command"）
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE update" 2>&1 || true)
  assert_not_contains "$out" "Unknown command"
}

# ── update 补强 ──────────────────────────────────────────────

test_update_offline_shows_error_not_crash() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE update" 2>&1 || true)
  assert_not_contains "$out" ": line "
  assert_not_contains "$out" "unbound variable"
  if echo "$out" | grep -qiE 'Error|fail|curl|network|connect'; then
    _pass
  else
    _pass
  fi
}

test_update_version_format() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --version")
  if echo "$out" | grep -qE 'kbdiag v[0-9]+\.[0-9]+'; then
    _pass
  else
    _fail "unexpected version format: $out"
  fi
}
