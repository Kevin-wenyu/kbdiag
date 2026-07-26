# test_workload.sh — tests for "kbdiag workload" command

# ── usage errors (no DB dependency) ─────────────────────────────────────────

test_workload_to_without_from_is_usage_error() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE workload --to 30m")
  assert_exit_code 2 "$code"
}

test_workload_to_without_from_message() {
  # assert_contains's grep -F has no "--" separator, so patterns starting
  # with "--" get parsed as grep flags — assert on the full phrase instead.
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE workload --to 30m")
  assert_contains "$out" "requires --from"
}

test_workload_reversed_range_is_usage_error() {
  # --from 30m --to 2h means "from 30 min ago to 2h ago" — nonsensical since
  # 2h ago is earlier than 30m ago.
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE workload --from 30m --to 2h")
  assert_exit_code 2 "$code"
}

test_workload_reversed_range_message() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE workload --from 30m --to 2h")
  assert_contains "$out" "must be earlier than"
}

# ── happy path on kes-node1 (sys_kwr installed, >=2 snapshots) ──────────────

test_workload_node1_happy_path_exits_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE workload")
  assert_exit_code 0 "$code"
}

test_workload_node1_happy_path_ok_verdict() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE workload")
  assert_contains "$out" "OK"
}

test_workload_node1_happy_path_has_report_content() {
  # perf.kwr_report()'s text output always includes this section header.
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE workload")
  assert_contains "$out" "DB Time"
}

# ── --from/--to wiring (regression check on the reused duration parser) ────

test_workload_from_flag_exits_zero() {
  # A wide window that comfortably covers the fixture's snapshots.
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE workload --from 720h")
  assert_exit_code 0 "$code"
}

test_workload_from_flag_has_report_content() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE workload --from 720h")
  assert_contains "$out" "DB Time"
}

test_workload_from_to_flags_exits_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE workload --from 720h --to 0m")
  assert_exit_code 0 "$code"
}

# ── standby (kes-node2): forced --no-snapshot on insufficient data (Q5) ─────

test_workload_node2_standby_narrow_window_warns() {
  # Fixture snapshots are ~24h old — a 1-minute window has zero coverage,
  # forcing the "insufficient data" branch. On a standby, auto-snapshot
  # is impossible (read-only), so this must degrade to a WARN, not attempt
  # perf.create_snapshot() (which would error against a read-only replica).
  local out; out=$(ssh_node2 "$KBDIAG_REMOTE workload --from 1m --to 0m")
  assert_contains "$out" "备库（只读）"
}

test_workload_node2_standby_narrow_window_exit_code() {
  local code; code=$(ssh_node2_exit "$KBDIAG_REMOTE workload --from 1m --to 0m --exit-code")
  assert_exit_code 1 "$code"
}

# ── --no-snapshot is a no-op when the default window already has enough data ─

test_workload_no_snapshot_flag_noop_on_sufficient_data() {
  # Default (no --from/--to) already resolves to the latest 2 snapshots on
  # node1 — --no-snapshot only matters on the "insufficient data" branch, so
  # here it must behave identically to a plain `workload` call: OK, exit 0.
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE workload --no-snapshot --exit-code")
  assert_exit_code 0 "$code"
}

test_workload_no_snapshot_flag_noop_has_report_content() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE workload --no-snapshot")
  assert_contains "$out" "DB Time"
}

# ── --format json ────────────────────────────────────────────────────────────

test_workload_json_format_is_valid() {
  # The 900+-line kwr_report() text (Chinese section headers, quotes, table
  # borders) is exactly the payload that would break naive JSON escaping.
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE workload --format json")
  assert_json_valid "$out"
}

test_workload_json_format_has_report_field() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE workload --format json")
  assert_contains "$out" "\"name\":\"report\""
}

# ── untestable WARN branches (no live fixture in this 2-node cluster) ──────
#
# sys_kwr, once installed on node1, replicates to node2 via physical
# streaming — there is no node in this cluster where it's absent, so the
# "extension not installed" and "neither data source available" degrade
# paths can't be exercised black-box. Decided 2026-07-26: no third instance
# just for this; these stay logic-only/code-review-verified. Stubs below
# graceful-skip if the precondition (extension actually missing) isn't met,
# matching test_logs.sh's _find_log_file skip convention — so they activate
# automatically if the fixture ever changes.

test_workload_extension_missing_warns() {
  local avail
  avail=$(ssh_node1 "/home/kingbase/cluster/install/kingbase/bin/ksql -p 54321 -U system test -AXtc \"SELECT count(*) FROM sys_extension WHERE extname='sys_kwr';\"" 2>/dev/null | tr -d '[:space:]')
  [[ "${avail:-0}" -gt 0 ]] && { _pass; return; }  # sys_kwr present on this fixture, skip
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE workload")
  assert_contains "$out" "sys_kwr 扩展未安装"
}

test_workload_no_data_source_warns() {
  local avail
  avail=$(ssh_node1 "/home/kingbase/cluster/install/kingbase/bin/ksql -p 54321 -U system test -AXtc \"SELECT count(*) FROM sys_extension WHERE extname='sys_kwr';\"" 2>/dev/null | tr -d '[:space:]')
  [[ "${avail:-0}" -gt 0 ]] && { _pass; return; }  # sys_kwr present on this fixture, skip
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE workload")
  assert_contains "$out" "两个数据源均不可用"
}
