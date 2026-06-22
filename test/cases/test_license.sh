test_license_exit_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE license")
  assert_exit_code 0 "$code"
}

test_license_status_ok() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE license")
  assert_contains "$out" "[OK]"
  assert_contains "$out" "授权状态"
}

test_license_contains_serial() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE license")
  assert_contains "$out" "序列号"
}

test_license_contains_product() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE license")
  assert_contains "$out" "产品名称"
}

test_license_contains_edition() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE license")
  assert_contains "$out" "版本模板"
}

test_license_quiet_hides_info() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -q license")
  assert_not_contains "$out" "[INFO]"
}

test_license_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json license")
  assert_json_valid "$out"
}

test_license_json_contains_status() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json license")
  assert_contains "$out" "license_status"
}
