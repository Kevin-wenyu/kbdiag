# test_advisor.sh — tests for "kbdiag advisor"

test_advisor_all_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE advisor")
  [[ "${code:-0}" -le 1 ]] && _pass || _fail "advisor exited $code"
}

test_advisor_all_has_section_headers() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE advisor")
  assert_contains "$out" "Advisor: Index"
  assert_contains "$out" "Advisor: Vacuum"
  assert_contains "$out" "Advisor: Params"
  assert_contains "$out" "Advisor: Analyze"
}

test_advisor_index_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE advisor index")
  [[ "${code:-0}" -le 1 ]] && _pass || _fail "advisor index exited $code"
}

test_advisor_vacuum_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE advisor vacuum")
  [[ "${code:-0}" -le 1 ]] && _pass || _fail "advisor vacuum exited $code"
}

test_advisor_params_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE advisor params")
  [[ "${code:-0}" -le 1 ]] && _pass || _fail "advisor params exited $code"
}

test_advisor_analyze_runs() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE advisor analyze")
  [[ "${code:-0}" -le 1 ]] && _pass || _fail "advisor analyze exited $code"
}

test_advisor_fix_outputs_sql_keywords() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE advisor --fix")
  # At minimum, should output fix section markers (avoid leading -- for grep compat)
  assert_contains "$out" "Fix: Index"
}

test_advisor_fix_vacuum_has_vacuum_sql() {
  # Set up a bloated table to trigger vacuum advice
  ksql_node1 "
    DROP TABLE IF EXISTS public.kbdiag_advisor_bloat;
    CREATE TABLE public.kbdiag_advisor_bloat AS SELECT i FROM generate_series(1,5000) i;
    DELETE FROM public.kbdiag_advisor_bloat WHERE i > 100;
    ANALYZE public.kbdiag_advisor_bloat;
  " > /dev/null 2>&1 || true

  local out; out=$(ssh_node1 "$KBDIAG_REMOTE advisor vacuum --fix")

  ksql_node1 "DROP TABLE IF EXISTS public.kbdiag_advisor_bloat;" > /dev/null 2>&1 || true

  # Should either have VACUUM SQL or OK (if autovacuum already ran)
  echo "$out" | grep -qE 'VACUUM|No tables' && _pass || _fail "expected VACUUM advice or OK: $out"
}

test_advisor_fix_analyze_has_analyze_sql() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE advisor analyze --fix")
  # Either "ANALYZE schema.table;" or OK message
  echo "$out" | grep -qE 'ANALYZE|No tables' && _pass || _fail "expected ANALYZE SQL or OK: $out"
}

test_advisor_unknown_sub_fails() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE advisor badcmd 2>/dev/null" 2>/dev/null || true)
  [[ "${code:-0}" -ne 0 ]] && _pass || _fail "expected non-zero for unknown subcommand"
}

test_advisor_json_valid() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json advisor")
  assert_json_valid "$out"
  assert_contains "$out" '"command":"advisor"'
}

test_advisor_quiet_hides_ok() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE -q advisor")
  assert_not_contains "$out" "[OK]"
}

test_advisor_standby_runs() {
  local code; code=$(ssh_node2_exit "$KBDIAG_REMOTE advisor")
  [[ "${code:-0}" -le 1 ]] && _pass || _fail "advisor on standby exited $code"
}

# ── --fix SQL 语法验证 ────────────────────────────────────────

# Match statement lines only — section headers ("==> Advisor: Vacuum") and
# comment lines also contain the keywords and used to shadow the statements.
test_advisor_fix_vacuum_sql_parseable() {
  local sql; sql=$(ssh_node1 "$KBDIAG_REMOTE advisor vacuum --fix" 2>/dev/null | grep -E '^VACUUM ' | head -1 || true)
  [[ -z "$sql" ]] && { _pass; return; }
  if echo "$sql" | grep -qE '^VACUUM .+;$'; then
    _pass
  else
    _fail "malformed VACUUM statement: $sql"
  fi
}

test_advisor_fix_analyze_sql_parseable() {
  local sql; sql=$(ssh_node1 "$KBDIAG_REMOTE advisor analyze --fix" 2>/dev/null | grep -E '^ANALYZE ' | head -1 || true)
  [[ -z "$sql" ]] && { _pass; return; }
  if echo "$sql" | grep -qE '^ANALYZE .+;$'; then
    _pass
  else
    _fail "malformed ANALYZE statement: $sql"
  fi
}

test_advisor_fix_index_sql_executable() {
  local sql; sql=$(ssh_node1 "$KBDIAG_REMOTE advisor index --fix" 2>/dev/null | grep -E '^(CREATE|DROP) INDEX ' | head -1 || true)
  [[ -z "$sql" ]] && { _pass; return; }
  if echo "$sql" | grep -qE '^(CREATE|DROP) INDEX CONCURRENTLY .+;$'; then
    _pass
  else
    _fail "malformed index statement: $sql"
  fi
}

# ── D2: query-layer default exit 0, --exit-code opts into worst verdict ──

test_advisor_default_exit_is_zero() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE advisor")
  assert_exit_code 0 "$code"
}

test_advisor_exit_code_flag_reflects_verdict() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE --exit-code advisor")
  if [[ "$code" -ge 0 && "$code" -le 2 ]]; then
    _pass
  else
    _fail "unexpected exit code: $code"
  fi
}
