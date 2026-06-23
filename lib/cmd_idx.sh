# shellcheck shell=bash
_idx_unused() {
  hdr "Unused indexes (idx_scan=0, table rows > 1000)"
  local cnt
  cnt=$(ksql_q "
    SELECT count(*)
    FROM sys_stat_user_indexes sui
    JOIN sys_index si ON si.indexrelid = sui.indexrelid
    JOIN sys_stat_user_tables sut
      ON sut.schemaname = sui.schemaname AND sut.relname = sui.relname
    WHERE sui.idx_scan = 0
      AND sut.n_live_tup > 1000
      AND si.indisunique = false
      AND si.indisprimary = false;" 2>/dev/null | tr -d '[:space:]')

  if [[ "${cnt:-0}" -eq 0 ]]; then
    ok "No unused non-unique indexes found"
    json_item "idx_unused" "ok" "0" ""
    return 0
  fi

  warn "${cnt} unused index(es) — consider DROP INDEX CONCURRENTLY"
  json_item "idx_unused" "warn" "${cnt}" "run 'idx unused' for detail"
  ksql_qh "
    SELECT sui.schemaname, sui.relname AS tablename, sui.indexrelname,
           pg_size_pretty(pg_relation_size(sui.indexrelid)) AS index_size,
           sui.idx_scan
    FROM sys_stat_user_indexes sui
    JOIN sys_index si ON si.indexrelid = sui.indexrelid
    JOIN sys_stat_user_tables sut
      ON sut.schemaname = sui.schemaname AND sut.relname = sui.relname
    WHERE sui.idx_scan = 0
      AND sut.n_live_tup > 1000
      AND si.indisunique = false
      AND si.indisprimary = false
    ORDER BY pg_relation_size(sui.indexrelid) DESC
    LIMIT ${TOP_N};" \
    | column -t -s '|' || true
  return 1
}

_idx_dup() {
  hdr "Duplicate indexes (same leading column set)"
  local cnt
  cnt=$(ksql_q "
    SELECT count(*)
    FROM sys_stat_user_indexes a
    JOIN sys_stat_user_indexes b
      ON a.schemaname = b.schemaname
     AND a.relname = b.relname
     AND a.indexrelid < b.indexrelid
    JOIN sys_index ia ON ia.indexrelid = a.indexrelid
    JOIN sys_index ib ON ib.indexrelid = b.indexrelid
    WHERE ia.indkey = ib.indkey
      AND ia.indpred IS NULL AND ib.indpred IS NULL;" 2>/dev/null | tr -d '[:space:]')

  if [[ "${cnt:-0}" -eq 0 ]]; then
    ok "No duplicate indexes found"
    json_item "idx_dup" "ok" "0" ""
    return 0
  fi

  warn "${cnt} duplicate index pair(s)"
  json_item "idx_dup" "warn" "${cnt}" ""
  ksql_qh "
    SELECT a.schemaname, a.relname AS tablename,
           a.indexrelname AS index1, b.indexrelname AS index2,
           pg_size_pretty(pg_relation_size(a.indexrelid)) AS size1
    FROM sys_stat_user_indexes a
    JOIN sys_stat_user_indexes b
      ON a.schemaname = b.schemaname
     AND a.relname = b.relname
     AND a.indexrelid < b.indexrelid
    JOIN sys_index ia ON ia.indexrelid = a.indexrelid
    JOIN sys_index ib ON ib.indexrelid = b.indexrelid
    WHERE ia.indkey = ib.indkey
      AND ia.indpred IS NULL AND ib.indpred IS NULL
    ORDER BY a.schemaname, a.relname
    LIMIT ${TOP_N};" \
    | column -t -s '|' || true
  return 1
}

_idx_bloat() {
  hdr "Bloated indexes (estimated bloat > 30%)"
  local cnt
  cnt=$(ksql_q "
    SELECT count(*)
    FROM sys_stat_user_indexes sui
    JOIN sys_class ic ON ic.oid = sui.indexrelid
    WHERE ic.relpages > 10
      AND CASE WHEN ic.relpages > 0
            THEN (1.0 - (ic.reltuples * 8.0 / (ic.relpages * 8192.0)))
            ELSE 0
          END > 0.3;" 2>/dev/null | tr -d '[:space:]')

  if [[ "${cnt:-0}" -eq 0 ]]; then
    ok "No highly bloated indexes found (threshold: 30%)"
    json_item "idx_bloat" "ok" "0" ""
    return 0
  fi

  warn "${cnt} bloated index(es) — consider REINDEX CONCURRENTLY"
  json_item "idx_bloat" "warn" "${cnt}" ""
  ksql_qh "
    SELECT sui.schemaname, sui.relname AS tablename, sui.indexrelname,
           pg_size_pretty(pg_relation_size(sui.indexrelid)) AS index_size,
           ic.relpages,
           round(ic.reltuples) AS reltuples,
           CASE WHEN ic.relpages > 0
             THEN round(100.0 * (1.0 - (ic.reltuples * 8.0 / (ic.relpages * 8192.0))), 1)
             ELSE 0
           END AS est_bloat_pct
    FROM sys_stat_user_indexes sui
    JOIN sys_class ic ON ic.oid = sui.indexrelid
    WHERE ic.relpages > 10
      AND CASE WHEN ic.relpages > 0
            THEN (1.0 - (ic.reltuples * 8.0 / (ic.relpages * 8192.0)))
            ELSE 0
          END > 0.3
    ORDER BY pg_relation_size(sui.indexrelid) DESC
    LIMIT ${TOP_N};" \
    | column -t -s '|' || true
  return 1
}

_idx_missing() {
  hdr "Missing FK indexes (large tables with unindexed FK leading column)"
  local cnt
  cnt=$(ksql_q "
    SELECT count(DISTINCT (n.nspname, c.relname, con.conname))
    FROM sys_constraint con
    JOIN sys_class c ON c.oid = con.conrelid
    JOIN sys_namespace n ON n.oid = c.relnamespace
    JOIN sys_attribute a
      ON a.attrelid = c.oid AND a.attnum = con.conkey[1]
    JOIN sys_stat_user_tables sut
      ON sut.schemaname = n.nspname AND sut.relname = c.relname
    WHERE con.contype = 'f'
      AND sut.n_live_tup > 10000
      AND n.nspname NOT IN ('sys_catalog', 'information_schema', 'sys_toast')
      AND NOT EXISTS (
        SELECT 1 FROM sys_index i
        WHERE i.indrelid = c.oid
          AND i.indkey[0] = con.conkey[1]
      );" 2>/dev/null | tr -d '[:space:]')

  if [[ "${cnt:-0}" -eq 0 ]]; then
    ok "No missing FK indexes found"
    json_item "idx_missing" "ok" "0" ""
    return 0
  fi

  warn "${cnt} FK column(s) without index on large table(s)"
  json_item "idx_missing" "warn" "${cnt}" ""
  ksql_qh "
    SELECT DISTINCT n.nspname AS schemaname, c.relname AS tablename,
           a.attname AS fk_col, con.conname AS constraint_name,
           sut.n_live_tup AS row_count
    FROM sys_constraint con
    JOIN sys_class c ON c.oid = con.conrelid
    JOIN sys_namespace n ON n.oid = c.relnamespace
    JOIN sys_attribute a
      ON a.attrelid = c.oid AND a.attnum = con.conkey[1]
    JOIN sys_stat_user_tables sut
      ON sut.schemaname = n.nspname AND sut.relname = c.relname
    WHERE con.contype = 'f'
      AND sut.n_live_tup > 10000
      AND n.nspname NOT IN ('sys_catalog', 'information_schema', 'sys_toast')
      AND NOT EXISTS (
        SELECT 1 FROM sys_index i
        WHERE i.indrelid = c.oid
          AND i.indkey[0] = con.conkey[1]
      )
    ORDER BY sut.n_live_tup DESC
    LIMIT ${TOP_N};" \
    | column -t -s '|' || true
  return 1
}

cmd_idx() {
  local sub="${1:-}"
  local _exit=0

  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "idx"

  case "$sub" in
    unused)  _idx_unused  || _exit=1 ;;
    dup)     _idx_dup     || _exit=1 ;;
    bloat)   _idx_bloat   || _exit=1 ;;
    missing) _idx_missing || _exit=1 ;;
    "")
      _idx_unused  || _exit=1
      _idx_dup     || _exit=1
      _idx_bloat   || _exit=1
      _idx_missing || _exit=1
      ;;
    *)
      echo "Unknown idx subcommand: $sub. Use: unused|dup|bloat|missing" >&2
      [[ "$OUTPUT_FMT" == "json" ]] && json_end
      return 1
      ;;
  esac

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  return $_exit
}
