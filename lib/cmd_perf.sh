# shellcheck shell=bash
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
  ksql_qh "
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
  # Threshold shared with advisor/diagnose's dead-tuple checks (live_pct = 100 - dead_pct)
  local live_floor=$(( 100 - KB_WARN_DEAD_PCT ))
  hdr "Table bloat (live ratio < ${live_floor}%)"
  ksql_qh "
    SELECT schemaname, relname,
           pg_size_pretty(pg_relation_size(schemaname||'.'||relname)) AS table_size,
           n_live_tup, n_dead_tup,
           CASE WHEN n_live_tup+n_dead_tup > 0
                THEN round(100.0*n_live_tup/(n_live_tup+n_dead_tup),1)
                ELSE 100 END AS live_pct
    FROM sys_stat_user_tables
    WHERE n_live_tup+n_dead_tup > 1000
      AND (n_live_tup::float/(n_live_tup+n_dead_tup+1)) < ${live_floor}/100.0
    ORDER BY n_dead_tup DESC
    LIMIT 20;" \
    | column -t -s '|' || true

  local cnt
  cnt=$(ksql_q "
    SELECT count(*) FROM sys_stat_user_tables
    WHERE n_live_tup+n_dead_tup > 1000
      AND (n_live_tup::float/(n_live_tup+n_dead_tup+1)) < ${live_floor}/100.0;" | tr -d '[:space:]')
  if [[ ${cnt:-0} -eq 0 ]]; then
    ok "No bloated tables"
  else
    warn "${cnt} bloated table(s) — consider VACUUM"
  fi
}

_perf_vacuum() {
  hdr "Vacuum status (stale or high dead tuples, with autovacuum threshold)"
  ksql_qh "
    SELECT schemaname, relname,
           n_dead_tup,
           last_autovacuum::date AS last_autovacuum,
           last_autoanalyze::date AS last_autoanalyze,
           round(
             (SELECT setting::int FROM sys_settings WHERE name='autovacuum_vacuum_threshold') +
             (SELECT setting::float FROM sys_settings WHERE name='autovacuum_vacuum_scale_factor') * reltuples
           ) AS threshold,
           round(100.0 * n_dead_tup /
             NULLIF(
               (SELECT setting::int FROM sys_settings WHERE name='autovacuum_vacuum_threshold') +
               (SELECT setting::float FROM sys_settings WHERE name='autovacuum_vacuum_scale_factor') * reltuples,
             0), 1) AS pct
    FROM sys_stat_user_tables
    WHERE n_dead_tup > 1000
       OR last_autovacuum < now() - interval '7 days'
       OR last_autovacuum IS NULL
    ORDER BY n_dead_tup DESC
    LIMIT ${TOP_N};" \
    | column -t -s '|' || true
}

_perf_wait() {
  hdr "Wait event distribution (active sessions)"
  ksql_qh "
    SELECT wait_event_type, wait_event, count(*) AS cnt,
           round(avg(EXTRACT(EPOCH FROM (now()-query_start))),1) AS avg_sec
    FROM sys_stat_activity
    WHERE $_WAIT_EVENT_WHERE
    GROUP BY wait_event_type, wait_event
    ORDER BY cnt DESC LIMIT ${TOP_N};" \
    | column -t -s '|' || true
}

_perf_io() {
  hdr "Table I/O hotspots (by physical reads)"
  ksql_qh "
    SELECT schemaname, relname,
           heap_blks_read, heap_blks_hit,
           CASE WHEN (heap_blks_read+heap_blks_hit)=0 THEN 100
                ELSE round(100.0*heap_blks_hit/(heap_blks_read+heap_blks_hit),1) END AS hit_pct
    FROM sys_statio_user_tables
    ORDER BY heap_blks_read DESC LIMIT ${TOP_N};" \
    | column -t -s '|' || true
}

_perf_wal() {
  hdr "WAL and checkpoint statistics"
  ksql_qh "
    SELECT checkpoints_timed, checkpoints_req,
           pg_size_pretty(buffers_checkpoint*8192::bigint) AS ckpt_bytes,
           pg_size_pretty(buffers_clean*8192::bigint) AS bgwriter_bytes,
           pg_size_pretty(buffers_backend*8192::bigint) AS backend_bytes
    FROM sys_stat_bgwriter;" \
    | column -t -s '|' || true
}

_perf_top() {
  local mode="${1:-buffers}"
  hdr "Top sessions by ${mode}"
  case "$mode" in
    buffers)
      ksql_qh "
        SELECT pid, usename, left(query,60) AS query,
               shared_blks_hit + shared_blks_read AS total_buffers
        FROM sys_stat_activity
        WHERE state='active' AND pid <> sys_backend_pid()
        ORDER BY total_buffers DESC NULLS LAST LIMIT ${TOP_N};" \
        | column -t -s '|' || true
      ;;
    reads)
      ksql_qh "
        SELECT pid, usename, left(query,60) AS query,
               shared_blks_read AS blks_read
        FROM sys_stat_activity
        WHERE state='active' AND pid <> sys_backend_pid()
        ORDER BY blks_read DESC NULLS LAST LIMIT ${TOP_N};" \
        | column -t -s '|' || true
      ;;
    writes)
      ksql_qh "
        SELECT pid, usename, left(query,60) AS query,
               shared_blks_written AS blks_written
        FROM sys_stat_activity
        WHERE state='active' AND pid <> sys_backend_pid()
        ORDER BY blks_written DESC NULLS LAST LIMIT ${TOP_N};" \
        | column -t -s '|' || true
      ;;
    cpu)
      ksql_qh "
        SELECT pid, usename, left(query,60) AS query,
               round(EXTRACT(EPOCH FROM (now()-query_start))::numeric,1) AS cpu_sec
        FROM sys_stat_activity
        WHERE state='active' AND pid <> sys_backend_pid()
        ORDER BY cpu_sec DESC NULLS LAST LIMIT ${TOP_N};" \
        | column -t -s '|' || true
      ;;
    temp)
      ksql_qh "
        SELECT pid, usename, left(query,60) AS query,
               temp_blks_read + temp_blks_written AS temp_blks
        FROM sys_stat_activity
        WHERE state='active' AND pid <> sys_backend_pid()
        ORDER BY temp_blks DESC NULLS LAST LIMIT ${TOP_N};" \
        | column -t -s '|' || true
      ;;
    *)
      echo "Unknown top mode: $mode (buffers|reads|writes|cpu|temp)" >&2
      return 1
      ;;
  esac
}

_perf_index() {
  hdr "Unused indexes (0 scans in last stats reset)"
  ksql_qh "
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
  shift 2>/dev/null || true
  case "$subcmd" in
    slow)   _perf_slow   ;;
    bloat)  _perf_bloat  ;;
    vacuum) _perf_vacuum ;;
    index)  _perf_index  ;;
    wait)   _perf_wait   ;;
    io)     _perf_io     ;;
    wal)    _perf_wal    ;;
    top)    _perf_top "${1:-buffers}" ;;
    "")
      _perf_slow
      _perf_bloat
      _perf_vacuum
      _perf_index
      _perf_wait
      _perf_io
      _perf_wal
      ;;
    *)
      echo "Unknown perf subcommand: $subcmd (slow|bloat|vacuum|index|wait|io|wal|top)" >&2
      return 1
      ;;
  esac
}
