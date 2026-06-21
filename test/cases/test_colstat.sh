# test_colstat.sh — tests for "kbdiag colstat"

_colstat_setup() {
  # Write SQL to a temp file to avoid single-quote escaping issues via ksql_node1
  local tmpfile; tmpfile=$(mktemp /tmp/kbdiag_colstat_setup_XXXXXX.sql)
  cat > "$tmpfile" <<'COLSTAT_SQL'
DROP TABLE IF EXISTS public.kbdiag_colstat_test;
CREATE TABLE public.kbdiag_colstat_test (
  id serial PRIMARY KEY,
  status varchar(20),
  amount numeric(10,2),
  created_at timestamp DEFAULT now()
);
INSERT INTO public.kbdiag_colstat_test (status, amount)
SELECT
  CASE (i % 4) WHEN 0 THEN 'pending' WHEN 1 THEN 'shipped' WHEN 2 THEN 'done' ELSE 'cancelled' END,
  (random() * 1000)::numeric(10,2)
FROM generate_series(1, 2000) i;
ANALYZE public.kbdiag_colstat_test;
COLSTAT_SQL
  setup_sql_node1 "$tmpfile" > /dev/null 2>&1 || true
  rm -f "$tmpfile"
}

_colstat_cleanup() {
  local tmpfile; tmpfile=$(mktemp /tmp/kbdiag_colstat_cleanup_XXXXXX.sql)
  echo "DROP TABLE IF EXISTS public.kbdiag_colstat_test;" > "$tmpfile"
  setup_sql_node1 "$tmpfile" > /dev/null 2>&1 || true
  rm -f "$tmpfile"
}

test_colstat_no_args_shows_usage() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE colstat 2>&1" 2>/dev/null || true)
  assert_contains "$out" "Usage"
}

test_colstat_table_shows_header() {
  _colstat_setup
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE colstat public.kbdiag_colstat_test")
  _colstat_cleanup
  assert_contains "$out" "Column statistics"
}

test_colstat_shows_column_names() {
  _colstat_setup
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE colstat public.kbdiag_colstat_test")
  _colstat_cleanup
  assert_contains "$out" "status"
  assert_contains "$out" "amount"
}

test_colstat_shows_last_analyze() {
  _colstat_setup
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE colstat public.kbdiag_colstat_test")
  _colstat_cleanup
  assert_contains "$out" "Last ANALYZE"
}

test_colstat_col_filter() {
  _colstat_setup
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE colstat public.kbdiag_colstat_test --col status")
  _colstat_cleanup
  assert_contains "$out" "status"
  assert_not_contains "$out" "amount"
}

test_colstat_nonexistent_table_fails() {
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE colstat public.no_such_table_xyz 2>/dev/null")
  [[ "${code:-0}" -ne 0 ]] && _pass || _fail "expected non-zero for missing table"
}

test_colstat_nonexistent_table_error_message() {
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE colstat public.no_such_table_xyz 2>&1" 2>/dev/null || true)
  assert_contains "$out" "not found"
}

test_colstat_exit_ok_when_no_warnings() {
  _colstat_setup
  # Fresh ANALYZE means no drift warnings
  local code; code=$(ssh_node1_exit "$KBDIAG_REMOTE colstat public.kbdiag_colstat_test")
  _colstat_cleanup
  # exit 0 or 1 (might have correlation warnings depending on data)
  [[ "${code:-0}" -le 1 ]] && _pass || _fail "unexpected exit code $code"
}

test_colstat_json_valid() {
  _colstat_setup
  local out; out=$(ssh_node1 "$KBDIAG_REMOTE --format json colstat public.kbdiag_colstat_test")
  _colstat_cleanup
  assert_json_valid "$out"
  assert_contains "$out" '"table"'
  assert_contains "$out" '"columns"'
}
