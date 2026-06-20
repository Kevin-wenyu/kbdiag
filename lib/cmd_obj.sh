cmd_obj() {
  local target="${1:-}"
  if [[ -z "$target" ]]; then
    echo "Usage: kbdiag obj <schema.table>" >&2; return 1
  fi

  local schema table
  if echo "$target" | grep -q '\.'; then
    schema="${target%%.*}"; table="${target#*.}"
  else
    schema="public"; table="$target"
  fi

  hdr "Object analysis: $schema.$table"

  # 1. Size & tuples
  info "Size & tuples:"
  ksql_q "
    SELECT pg_size_pretty(pg_total_relation_size('\"$schema\".\"$table\"')) AS total_size,
           pg_size_pretty(pg_relation_size('\"$schema\".\"$table\"')) AS table_size,
           pg_size_pretty(pg_indexes_size('\"$schema\".\"$table\"')) AS index_size,
           n_live_tup, n_dead_tup,
           round(100.0*n_dead_tup/NULLIF(n_live_tup+n_dead_tup,0),1) AS dead_pct
    FROM sys_stat_user_tables WHERE schemaname='$schema' AND relname='$table';" | column -t -s '|' || true

  # 2. Vacuum/Analyze status
  info "Vacuum/Analyze status:"
  ksql_q "
    SELECT last_vacuum::date, last_autovacuum::date, last_analyze::date, last_autoanalyze::date,
           n_mod_since_analyze AS mods_since_analyze
    FROM sys_stat_user_tables WHERE schemaname='$schema' AND relname='$table';" | column -t -s '|' || true

  # 3. Indexes
  info "Indexes (usage):"
  ksql_q "
    SELECT i.indexrelname, pg_size_pretty(pg_relation_size(i.indexrelid)) AS size,
           i.idx_scan, i.idx_tup_read, i.idx_tup_fetch,
           CASE WHEN si.indisunique THEN 'UNIQUE' ELSE '' END AS flags
    FROM sys_stat_user_indexes i
    JOIN sys_index si ON si.indexrelid = i.indexrelid
    WHERE i.schemaname='$schema' AND i.relname='$table'
    ORDER BY i.idx_scan DESC;" | column -t -s '|' || true

  # 4. Constraints
  info "Constraints:"
  ksql_q "
    SELECT conname, contype,
           CASE contype WHEN 'p' THEN 'PRIMARY KEY' WHEN 'f' THEN 'FOREIGN KEY'
                        WHEN 'u' THEN 'UNIQUE' WHEN 'c' THEN 'CHECK' END AS type
    FROM sys_constraint WHERE conrelid = '\"$schema\".\"$table\"'::regclass;" | column -t -s '|' || true

  # 5. verbose: Column statistics
  if [[ -n "$VERBOSE" ]]; then
    info "Column statistics:"
    ksql_q "
      SELECT attname, null_frac, n_distinct, most_common_vals
      FROM sys_stats WHERE schemaname='$schema' AND tablename='$table'
      ORDER BY attname LIMIT $TOP_N;" | column -t -s '|' || true
  fi
}
