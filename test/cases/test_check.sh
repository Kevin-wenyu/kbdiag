test_check_exit_ok_or_warn() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE check")
  # 正常环境下 exit 0 或 1（archiver 失败是已知 WARN）
  [[ "$code" -le 1 ]] && _pass || _fail "exit code $code > 1 on healthy node"
}

test_check_has_ok_or_warn() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE check")
  assert_contains "$out" "[OK]"
}

test_check_quiet_hides_ok() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -q check")
  assert_not_contains "$out" "[OK]"
  assert_not_contains "$out" "[INFO]"
}

test_check_verbose_shows_detail() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -v check")
  # verbose 应显示连接用户明细
  assert_contains "$out" "esrep"
}

test_check_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json check")
  assert_json_valid "$out"
  assert_contains "$out" '"command":"check"'
  assert_contains "$out" '"items":'
}

test_check_xid_item_present() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE check")
  assert_contains "$out" "XID"
}

test_check_buffer_hit_present() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE check")
  assert_contains "$out" "Buffer hit"
}

test_check_standby_has_replication_lag() {
  local out; out=$(ssh_node2 "$KBDIAG_REMOTE check")
  assert_contains "$out" "slot lag"
}

test_check_oldest_txn_present() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE check")
  assert_contains "$out" "oldest"
}

test_check_wal_archiving_item_present() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE check")
  assert_contains "$out" "WAL archiving"
}

# ── check --os ───────────────────────────────────────────────

test_check_os_exits_0_1_2() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE check --os")
  case "${code:-x}" in
    0|1|2) _pass ;;
    *) _fail "expected exit 0/1/2, got $code" ;;
  esac
}

test_check_os_has_os_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE check --os")
  local missing="" kw
  for kw in "OS check" "THP" "Swappiness" "Overcommit" "Open files" \
            "Clock sync" "filesystem"; do
    echo "$out" | grep -q "$kw" || missing="$missing '$kw'"
  done
  if [[ -z "$missing" ]]; then
    _pass
  else
    _fail "OS items missing:$missing"
  fi
}

test_check_default_has_no_os_section() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE check")
  assert_not_contains "$out" "OS check"
}

test_check_os_json_has_os_items() {
  local out cnt
  out=$(ssh_node1 "$KBDIAG_REMOTE check --os --format json")
  cnt=$(echo "$out" | python3 -c "
import json,sys
print(len([i for i in json.load(sys.stdin)['items'] if i['name'].startswith('os_')]))" 2>/dev/null)
  if [[ "${cnt:-0}" -ge 9 ]]; then
    _pass
  else
    _fail "expected >=9 os_ json items, got ${cnt:-parse-error}"
  fi
}
