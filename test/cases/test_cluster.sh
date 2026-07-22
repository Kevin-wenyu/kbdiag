# test_cluster.sh — tests for "kbdiag cluster" command

test_cluster_show_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE cluster")
  assert_contains "$out" "Cluster status"
}

test_cluster_show_lists_nodes() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE cluster")
  if echo "$out" | grep -q "node1" && echo "$out" | grep -q "node2"; then
    _pass
  else
    _fail "cluster show missing nodes; got: $(echo "$out" | head -5)"
  fi
}

test_cluster_show_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json cluster" 2>/dev/null)
  assert_json_valid "$out"
}

test_cluster_show_json_has_node_rows() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json cluster" 2>/dev/null)
  assert_contains "$out" '"name":"node1"'
  assert_contains "$out" '"name":"node2"'
}

test_cluster_show_exit_zero_by_default() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE cluster")
  assert_exit_code 0 "$code"
}

# ── cluster ready ────────────────────────────────────────────

test_cluster_ready_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE cluster ready")
  assert_contains "$out" "Failover readiness"
}

test_cluster_ready_runs_all_checks() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE cluster ready")
  local missing="" kw
  for kw in Topology Promotion Arbitration repmgrd "Failover mode" \
            "Standby attach" Slots "Archive backlog" "Standby node2" VIP; do
    echo "$out" | grep -q "$kw" || missing="$missing $kw"
  done
  if [[ -z "$missing" ]]; then
    _pass
  else
    _fail "checks missing:$missing"
  fi
}

test_cluster_ready_exit_ok_or_warn() {
  # healthy test cluster: only environmental WARNs (e.g. archive backlog) allowed
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE cluster ready")
  if [[ "${code:-0}" == "0" || "${code:-0}" == "1" ]]; then
    _pass
  else
    _fail "expected exit 0/1, got $code"
  fi
}

test_cluster_ready_on_standby() {
  local out; out=$(ssh_node2 "$KBDIAG_REMOTE cluster ready")
  # standby path: remote primary query + VIP-unbound judgement must both work
  if echo "$out" | grep -q "Standby attach" && echo "$out" | grep -q "VIP"; then
    _pass
  else
    _fail "standby-side run incomplete; got: $(echo "$out" | head -8)"
  fi
}

test_cluster_ready_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE cluster ready --format json")
  assert_json_valid "$out"
}

test_cluster_ready_json_has_items() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE cluster ready --format json")
  local cnt
  cnt=$(echo "$out" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['items']))" 2>/dev/null)
  if [[ "${cnt:-0}" -ge 10 ]]; then
    _pass
  else
    _fail "expected >=10 json items, got ${cnt:-parse-error}"
  fi
}

test_cluster_ready_json_text_verdicts_agree() {
  # text exit code and worst json status must tell the same story
  local code json worst
  code=$(ssh_node1_exit "$KBDIAG_REMOTE cluster ready")
  json=$(ssh_node1 "$KBDIAG_REMOTE cluster ready --format json")
  worst=$(echo "$json" | python3 -c "
import json,sys
sts=[i['status'] for i in json.load(sys.stdin)['items']]
print(2 if 'fail' in sts else 1 if 'warn' in sts else 0)" 2>/dev/null)
  assert_exit_code "${worst:-x}" "${code:-y}"
}
