_perf_slow() {
  hdr "Slow queries (running > ${KB_SLOW_THRESHOLD}s)"
  local cnt
  cnt=$(ksql_q "
    SELECT count(*) FROM sys_stat_activity
    WHERE state = 'active'
      AND now() - query_start > interval '${KB_SLOW_THRESHOLD} seconds'
      AND pid <> sys_backend_pid();" | tr -d '[:space:]')

  if [[ ${cnt:-0} -eq 0 ]]; then
    ok "No slow queries"
    return 0
  fi

  warn "${cnt} slow query(ies)"
  ksql_q "
    SELECT pid, usename,
           date_trunc('second', now()-query_start)::text AS duration,
           client_addr,
           left(query, 80) AS query
    FROM sys_stat_activity
    WHERE state = 'active'
      AND now() - query_start > interval '${KB_SLOW_THRESHOLD} seconds'
      AND pid <> sys_backend_pid()
    ORDER BY query_start;" \
    | column -t -s '|' || true
}

_perf_bloat() {
  hdr "Table bloat (live ratio < 80%)"
  ksql_q "
    SELECT schemaname, relname,
           pg_size_pretty(pg_relation_size(schemaname||'.'||relname)) AS table_size,
           n_live_tup, n_dead_tup,
           CASE WHEN n_live_tup+n_dead_tup > 0
                THEN round(100.0*n_live_tup/(n_live_tup+n_dead_tup),1)
                ELSE 100 END AS live_pct
    FROM sys_stat_user_tables
    WHERE n_live_tup+n_dead_tup > 1000
      AND (n_live_tup::float/(n_live_tup+n_dead_tup+1)) < 0.8
    ORDER BY n_dead_tup DESC
    LIMIT 20;" \
    | column -t -s '|' || true

  local cnt
  cnt=$(ksql_q "
    SELECT count(*) FROM sys_stat_user_tables
    WHERE n_live_tup+n_dead_tup > 1000
      AND (n_live_tup::float/(n_live_tup+n_dead_tup+1)) < 0.8;" | tr -d '[:space:]')
  if [[ ${cnt:-0} -eq 0 ]]; then
    ok "No bloated tables"
  else
    warn "${cnt} bloated table(s) — consider VACUUM"
  fi
}

_perf_vacuum() {
  hdr "Vacuum status (stale or high dead tuples)"
  ksql_q "
    SELECT schemaname, relname,
           n_dead_tup,
           last_autovacuum::date AS last_autovacuum,
           last_autoanalyze::date AS last_autoanalyze
    FROM sys_stat_user_tables
    WHERE n_dead_tup > 1000
       OR last_autovacuum < now() - interval '7 days'
       OR last_autovacuum IS NULL
    ORDER BY n_dead_tup DESC
    LIMIT 20;" \
    | column -t -s '|' || true
}

_perf_index() {
  hdr "Unused indexes (0 scans in last stats reset)"
  ksql_q "
    SELECT schemaname, relname, indexrelname,
           pg_size_pretty(pg_relation_size(indexrelid)) AS index_size,
           idx_scan
    FROM sys_stat_user_indexes
    WHERE idx_scan = 0
      AND indexrelname NOT LIKE '%_pkey'
    ORDER BY pg_relation_size(indexrelid) DESC
    LIMIT 20;" \
    | column -t -s '|' || true

  local cnt
  cnt=$(ksql_q "
    SELECT count(*) FROM sys_stat_user_indexes
    WHERE idx_scan = 0 AND indexrelname NOT LIKE '%_pkey';" | tr -d '[:space:]')
  if [[ ${cnt:-0} -eq 0 ]]; then
    ok "No unused indexes found"
  else
    warn "${cnt} potentially unused index(es)"
  fi
}

cmd_perf() {
  local subcmd="${1:-}"
  case "$subcmd" in
    slow)   _perf_slow   ;;
    bloat)  _perf_bloat  ;;
    vacuum) _perf_vacuum ;;
    index)  _perf_index  ;;
    "")
      _perf_slow
      _perf_bloat
      _perf_vacuum
      _perf_index
      ;;
    *)
      echo "Unknown perf subcommand: $subcmd (slow|bloat|vacuum|index)" >&2
      return 1
      ;;
  esac
}
