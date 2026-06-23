# test_sql.sh — tests for "kbdiag sql" subcommand

# Helper: write SQL to a temp file on node1 and run it, avoiding single-quote nesting issues
_ksql_node1_file() {
  local sql="$1"
  local tmpfile="/tmp/_kbdiag_test_sql_$$.sql"
  # Write SQL via echo through SSH (no shell quoting of SQL content needed)
  $NODE1_SSH "echo $(printf '%q' "$sql") > $tmpfile" 2>/dev/null
  ssh_node1 "/home/kingbase/cluster/install/kingbase/bin/ksql -p 54321 -U system test -AXt -f $tmpfile"
  $NODE1_SSH "rm -f $tmpfile" 2>/dev/null
}

# Helper: get PID of background slow query on node1
_get_slow_query_pid() {
  _ksql_node1_file "SELECT pid FROM sys_stat_activity WHERE query LIKE '%pg_sleep%' AND pid <> sys_backend_pid() LIMIT 1;" | tr -d '[:space:]'
}

# ── sql all ───────────────────────────────────────────────────────────────────

test_sql_all_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE sql all")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "sql all exited $code"
}

test_sql_default_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE sql")
  [[ "${code:-0}" -eq 0 ]] && _pass || _fail "sql (default) exited $code"
}

test_sql_all_shows_header() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE sql all")
  assert_contains "$out" "SQL analysis"
}

test_sql_all_shows_slow_query() {
  start_slow_query_bg
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE sql all")
  stop_slow_query_bg
  assert_contains "$out" "pg_sleep"
}

# ── sql <pid> ─────────────────────────────────────────────────────────────────

test_sql_pid_deep_dive() {
  start_slow_query_bg
  local pid; pid=$(_get_slow_query_pid)
  local out code
  if [[ -n "$pid" && "$pid" =~ ^[0-9]+$ ]]; then
    out=$(ssh_node1 "$KBDIAG_REMOTE sql $pid")
    code=$?
    stop_slow_query_bg
    [[ $code -eq 0 ]] && _pass || _fail "sql $pid exited $code"
  else
    stop_slow_query_bg
    _fail "could not get slow query pid (got: '$pid')"
  fi
}

test_sql_pid_shows_query_text() {
  start_slow_query_bg
  local pid; pid=$(_get_slow_query_pid)
  local out
  if [[ -n "$pid" && "$pid" =~ ^[0-9]+$ ]]; then
    out=$(ssh_node1 "$KBDIAG_REMOTE sql $pid")
    stop_slow_query_bg
    assert_contains "$out" "pg_sleep"
  else
    stop_slow_query_bg
    _fail "could not get slow query pid (got: '$pid')"
  fi
}

test_sql_pid_shows_explain() {
  start_slow_query_bg
  local pid; pid=$(_get_slow_query_pid)
  local out
  if [[ -n "$pid" && "$pid" =~ ^[0-9]+$ ]]; then
    out=$(ssh_node1 "$KBDIAG_REMOTE sql $pid")
    stop_slow_query_bg
    # EXPLAIN header or "EXPLAIN failed" warning should appear
    { echo "$out" | grep -qF "EXPLAIN" && _pass; } || _fail "no EXPLAIN in output"
  else
    stop_slow_query_bg
    _fail "could not get slow query pid (got: '$pid')"
  fi
}

test_sql_pid_verbose_uses_verbose_explain() {
  start_slow_query_bg
  local pid; pid=$(_get_slow_query_pid)
  local out
  if [[ -n "$pid" && "$pid" =~ ^[0-9]+$ ]]; then
    out=$(ssh_node1 "$KBDIAG_REMOTE -v sql $pid")
    stop_slow_query_bg
    assert_contains "$out" "VERBOSE"
  else
    stop_slow_query_bg
    _fail "could not get slow query pid (got: '$pid')"
  fi
}

test_sql_pid_invalid_shows_warn() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE sql 999999")
  assert_contains "$out" "No active session"
}

# ── header verification ──────────────────────────────────────────────────────

test_sql_all_has_column_header() {
  # Force at least one non-idle session so the result set isn't empty.
  start_slow_query_bg
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE sql all" 2>&1)
  stop_slow_query_bg
  echo "$out" | grep -qi "usename" && _pass || _fail "sql all missing 'usename' column header"
}
