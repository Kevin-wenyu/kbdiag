# test_report.sh — tests for "kbdiag report" command
# The report takes ~16s to generate, so tests share one generated file.

_REPORT_PATH="/tmp/kbdiag_test_report.md"
_REPORT_EXIT=""

_report_ensure() {
  [[ -n "$_REPORT_EXIT" ]] && return 0
  _REPORT_EXIT=$(ssh_node1_exit "$KBDIAG_REMOTE report $_REPORT_PATH")
}

test_report_generates_and_exits_0_1_2() {
  _report_ensure
  case "${_REPORT_EXIT:-x}" in
    0|1|2) _pass ;;
    *) _fail "expected exit 0/1/2, got $_REPORT_EXIT" ;;
  esac
}

test_report_file_nonempty() {
  _report_ensure
  local sz; sz=$(ssh_node1 "wc -c < $_REPORT_PATH" | tr -d '[:space:]')
  if [[ "${sz:-0}" -gt 1000 ]]; then
    _pass
  else
    _fail "report file too small: ${sz:-missing} bytes"
  fi
}

test_report_has_executive_summary() {
  local out; out=$(ssh_node1 "cat $_REPORT_PATH")
  if echo "$out" | grep -q "Executive summary" && echo "$out" | grep -q "Overall:"; then
    _pass
  else
    _fail "missing executive summary"
  fi
}

test_report_has_all_sections() {
  local out; out=$(ssh_node1 "grep '^## ' $_REPORT_PATH")
  local missing="" s
  for s in "Instance status" "License" "Health check" "Failover readiness" \
           "Replication" "Capacity" "Backup & archiving" "Throughput" \
           "Index health" "Security audit" "Recommendations"; do
    echo "$out" | grep -q "$s" || missing="$missing '$s'"
  done
  if [[ -z "$missing" ]]; then
    _pass
  else
    _fail "sections missing:$missing"
  fi
}

test_report_no_ansi_codes() {
  local cnt; cnt=$(ssh_node1 "grep -cP '\x1b' $_REPORT_PATH" | tr -d '[:space:]')
  assert_exit_code 0 "${cnt:-1}"
}

test_report_summary_counts_match_table() {
  # "N WARN / M FAIL" in the Overall line must equal summary table rows
  local out warn_n fail_n warn_rows fail_rows
  out=$(ssh_node1 "cat $_REPORT_PATH")
  warn_n=$(echo "$out" | sed -nE 's/.*Overall.* ([0-9]+) WARN.*/\1/p')
  fail_n=$(echo "$out" | sed -nE 's/.*Overall.* ([0-9]+) FAIL.*/\1/p')
  warn_rows=$(echo "$out" | grep -c '| WARN |')
  fail_rows=$(echo "$out" | grep -c '| FAIL |')
  if [[ "$warn_n" == "$warn_rows" && "$fail_n" == "$fail_rows" ]]; then
    _pass
  else
    _fail "overall says ${warn_n}W/${fail_n}F but table has ${warn_rows}W/${fail_rows}F rows"
  fi
}

test_report_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE report /tmp/kbdiag_test_report_json.md --format json")
  assert_json_valid "$out"
}

test_report_works_on_standby() {
  local code; code=$(ssh_node2_exit "$KBDIAG_REMOTE report /tmp/kbdiag_test_report_sb.md")
  case "${code:-x}" in
    0|1|2) _pass ;;
    *) _fail "standby report failed: exit $code" ;;
  esac
}
