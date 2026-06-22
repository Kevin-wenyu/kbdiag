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
