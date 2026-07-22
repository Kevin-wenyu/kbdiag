# test_kill.sh — tests for "kbdiag kill"

# Helper: run SQL via temp file on node1 to avoid single-quote quoting issues
_kill_ksql_node1() {
  local sql="$1"
  local tmpfile="/tmp/_kbdiag_kill_test_$$.sql"
  $NODE1_SSH "echo $(printf '%q' "$sql") > $tmpfile" 2>/dev/null
  ssh_node1 "/home/kingbase/cluster/install/kingbase/bin/ksql -p 54321 -U system test -AXt -f $tmpfile"
  $NODE1_SSH "rm -f $tmpfile" 2>/dev/null
}

test_kill_no_args_shows_usage() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE kill 2>&1" 2>/dev/null || true)
  assert_contains "$out" "Usage"
}

test_kill_dry_run_long_shows_header() {
  # Start a slow query, then verify dry-run lists it
  start_slow_query_bg
  sleep 2
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE kill --long 1 --dry-run")
  stop_slow_query_bg
  assert_contains "$out" "Sessions matching"
}

test_kill_dry_run_does_not_terminate() {
  start_slow_query_bg
  sleep 2
  ssh_node1 "$KBDIAG_REMOTE kill --long 1 --dry-run" > /dev/null 2>&1 || true
  # Query should still be running
  local cnt; cnt=$(_kill_ksql_node1 "SELECT count(*) FROM sys_stat_activity WHERE query LIKE '%pg_sleep%' AND state='active';" | tr -d '[:space:]')
  stop_slow_query_bg
  [[ "${cnt:-0}" -ge 1 ]] && _pass || _fail "dry-run should not have killed query (cnt=$cnt)"
}

test_kill_force_cancels_long_query() {
  start_slow_query_bg
  sleep 2
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE kill --long 1 --force")
  assert_contains "$out" "[OK]"
}

test_kill_pid_cancel_success() {
  start_slow_query_bg
  sleep 2
  local pid; pid=$(_kill_ksql_node1 "SELECT pid FROM sys_stat_activity WHERE query LIKE '%pg_sleep%' AND state='active' LIMIT 1;" | tr -d '[:space:]')
  if [[ -z "$pid" ]]; then
    stop_slow_query_bg
    _fail "could not get slow query pid"
    return
  fi
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE kill $pid")
  stop_slow_query_bg
  assert_contains "$out" "[OK]"
}

test_kill_nonexistent_pid_warns() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE kill 999999999" 2>&1 || true)
  assert_contains "$out" "[WARN]"
}

test_kill_dry_run_no_match_ok() {
  # No queries running > 9999 seconds — should say 0 matches
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE kill --long 9999 --dry-run")
  assert_contains "$out" "0 session"
}

test_kill_json_valid() {
  start_slow_query_bg
  sleep 2
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json kill --long 1 --force")
  stop_slow_query_bg
  assert_json_valid "$out"
}

test_kill_idle_txn_dry_run() {
  # No idle-in-transaction sessions expected on clean test env
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE kill --idle-txn 1 --dry-run")
  assert_contains "$out" "Sessions matching"
}

test_kill_query_text_not_mangled() {
  # _kill_show_sessions used to blanket-strip all spaces from the query
  # column, turning "SELECT pg_sleep(30)" into "SELECTpg_sleep(30)".
  start_slow_query_bg
  sleep 2
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE kill --long 1 --dry-run")
  stop_slow_query_bg
  assert_contains "$out" "SELECT pg_sleep"
}

test_kill_force_acts_on_all_matches_beyond_display_cap() {
  # Batch queries used to bake LIMIT $TOP_N into the SQL, so --force only
  # cancelled the displayed top N sessions. With --top 1 and 2 matching
  # slow queries, both must be cancelled even though only 1 is displayed.
  start_slow_query_bg
  start_slow_query_bg
  sleep 4
  ssh_node1 "$KBDIAG_REMOTE --top 1 kill --long 1 --force" > /dev/null
  sleep 1
  # Exclude our own backend: this counting query's text contains the literal
  # substring "pg_sleep" (in the LIKE pattern itself), so without excluding
  # sys_backend_pid() it always matches itself and cnt could never reach 0.
  local cnt; cnt=$(_kill_ksql_node1 "SELECT count(*) FROM sys_stat_activity WHERE query LIKE '%pg_sleep%' AND state='active' AND pid <> sys_backend_pid();" | tr -d '[:space:]')
  stop_slow_query_bg
  [[ "${cnt:-0}" -eq 0 ]] && _pass || _fail "expected all matching sessions cancelled despite --top 1 cap, cnt=$cnt"
}

test_kill_json_has_verdict_item() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json kill --idle-txn 9999 --dry-run")
  assert_json_valid "$out"
  assert_contains "$out" '"name":"idle_txn"'
}
