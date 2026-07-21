# shellcheck shell=bash

# Shared filter fragments — cmd_advisor.sh's _advisor_index() reuses these
# instead of duplicating the SQL; edit only here to change what counts as
# unused/duplicate/missing-FK.
_IDX_UNUSED_FROM="FROM sys_stat_user_indexes sui
    JOIN sys_index si ON si.indexrelid = sui.indexrelid
    JOIN sys_stat_user_tables sut
      ON sut.schemaname = sui.schemaname AND sut.relname = sui.relname"
_IDX_UNUSED_WHERE="sui.idx_scan = 0
      AND sut.n_live_tup > 1000
      AND si.indisunique = false
      AND si.indisprimary = false"

_IDX_DUP_FROM="FROM sys_stat_user_indexes a
    JOIN sys_stat_user_indexes b
      ON a.schemaname = b.schemaname
     AND a.relname = b.relname
     AND a.indexrelid < b.indexrelid
    JOIN sys_index ia ON ia.indexrelid = a.indexrelid
    JOIN sys_index ib ON ib.indexrelid = b.indexrelid"
_IDX_DUP_WHERE="ia.indkey = ib.indkey
      AND ia.indpred IS NULL AND ib.indpred IS NULL"

_IDX_MISSING_FROM="FROM sys_constraint con
    JOIN sys_class c ON c.oid = con.conrelid
    JOIN sys_namespace n ON n.oid = c.relnamespace
    JOIN sys_attribute a
      ON a.attrelid = c.oid AND a.attnum = con.conkey[1]
    JOIN sys_stat_user_tables sut
      ON sut.schemaname = n.nspname AND sut.relname = c.relname"
_IDX_MISSING_WHERE="con.contype = 'f'
      AND sut.n_live_tup > 10000
      AND n.nspname NOT IN ('sys_catalog', 'information_schema', 'sys_toast')
      AND NOT EXISTS (
        SELECT 1 FROM sys_index i
        WHERE i.indrelid = c.oid
          AND i.indkey[0] = con.conkey[1]
      )"

# Render a captured row set as an aligned table. Shows only the first TOP_N
# rows with a caveat unless VERBOSE is set — same captured rows, no second
# query, per the "one query, two renderings" convention.
_idx_render_rows() {
  local header="$1" rows="$2" cnt="$3"
  if [[ -z "$VERBOSE" && "$cnt" -gt "$TOP_N" ]]; then
    { echo "$header"; echo "$rows" | head -n "$TOP_N"; } | column -t -s '|'
    echo "  (showing top ${TOP_N} of ${cnt} — use -v for full list)"
  else
    { echo "$header"; echo "$rows"; } | column -t -s '|'
  fi
}

_idx_unused() {
  hdr "Unused indexes (idx_scan=0, table rows > 1000)"
  local rows cnt
  rows=$(ksql_q "
    SELECT sui.schemaname, sui.relname, sui.indexrelname,
           pg_relation_size(sui.indexrelid),
           pg_size_pretty(pg_relation_size(sui.indexrelid)),
           sui.idx_scan
    $_IDX_UNUSED_FROM
    WHERE $_IDX_UNUSED_WHERE
    ORDER BY pg_relation_size(sui.indexrelid) DESC;" 2>/dev/null) || rows=""
  if [[ -z "$rows" ]]; then cnt=0; else cnt=$(echo "$rows" | grep -c '.'); fi

  if [[ "$cnt" -eq 0 ]]; then
    ok "No unused non-unique indexes found"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "idx_unused" "ok" "0" ""
    return 0
  fi

  warn "${cnt} unused index(es) — consider DROP INDEX CONCURRENTLY"
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_item "idx_unused" "warn" "${cnt}" "run 'idx unused' for detail"
    while IFS='|' read -r schemaname tablename indexname bytes _ idx_scan; do
      [[ -z "${schemaname// /}" ]] && continue
      json_row check=idx_unused schemaname="${schemaname// /}" tablename="${tablename// /}" indexname="${indexname// /}" index_size_bytes="${bytes// /}" idx_scan="${idx_scan// /}"
    done <<< "$rows"
  else
    _idx_render_rows "schemaname|tablename|indexrelname|index_size|idx_scan" \
      "$(echo "$rows" | awk -F'|' '{print $1"|"$2"|"$3"|"$5"|"$6}')" "$cnt"
  fi
  return 1
}

_idx_dup() {
  hdr "Duplicate indexes (same leading column set)"
  local rows cnt
  rows=$(ksql_q "
    SELECT a.schemaname, a.relname,
           a.indexrelname, b.indexrelname,
           pg_relation_size(a.indexrelid),
           pg_size_pretty(pg_relation_size(a.indexrelid))
    $_IDX_DUP_FROM
    WHERE $_IDX_DUP_WHERE
    ORDER BY a.schemaname, a.relname;" 2>/dev/null) || rows=""
  if [[ -z "$rows" ]]; then cnt=0; else cnt=$(echo "$rows" | grep -c '.'); fi

  if [[ "$cnt" -eq 0 ]]; then
    ok "No duplicate indexes found"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "idx_dup" "ok" "0" ""
    return 0
  fi

  warn "${cnt} duplicate index pair(s)"
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_item "idx_dup" "warn" "${cnt}" "run 'idx dup' for detail"
    while IFS='|' read -r schemaname tablename index1 index2 bytes _; do
      [[ -z "${schemaname// /}" ]] && continue
      json_row check=idx_dup schemaname="${schemaname// /}" tablename="${tablename// /}" index1="${index1// /}" index2="${index2// /}" size1_bytes="${bytes// /}"
    done <<< "$rows"
  else
    _idx_render_rows "schemaname|tablename|index1|index2|size1" \
      "$(echo "$rows" | awk -F'|' '{print $1"|"$2"|"$3"|"$4"|"$6}')" "$cnt"
  fi
  return 1
}

_idx_bloat() {
  hdr "Bloated indexes (estimated bloat > 30%)"
  local rows cnt
  rows=$(ksql_q "
    SELECT sui.schemaname, sui.relname, sui.indexrelname,
           pg_relation_size(sui.indexrelid),
           pg_size_pretty(pg_relation_size(sui.indexrelid)),
           ic.relpages,
           round(ic.reltuples),
           CASE WHEN ic.relpages > 0
             THEN round(100.0 * (1.0 - (ic.reltuples * 8.0 / (ic.relpages * 8192.0))), 1)
             ELSE 0
           END
    FROM sys_stat_user_indexes sui
    JOIN sys_class ic ON ic.oid = sui.indexrelid
    WHERE ic.relpages > 10
      AND CASE WHEN ic.relpages > 0
            THEN (1.0 - (ic.reltuples * 8.0 / (ic.relpages * 8192.0)))
            ELSE 0
          END > 0.3
    ORDER BY pg_relation_size(sui.indexrelid) DESC;" 2>/dev/null) || rows=""
  if [[ -z "$rows" ]]; then cnt=0; else cnt=$(echo "$rows" | grep -c '.'); fi

  if [[ "$cnt" -eq 0 ]]; then
    ok "No highly bloated indexes found (threshold: 30%)"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "idx_bloat" "ok" "0" ""
    return 0
  fi

  warn "${cnt} bloated index(es) — consider REINDEX CONCURRENTLY"
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_item "idx_bloat" "warn" "${cnt}" "run 'idx bloat' for detail"
    while IFS='|' read -r schemaname tablename indexname bytes _ relpages reltuples pct; do
      [[ -z "${schemaname// /}" ]] && continue
      json_row check=idx_bloat schemaname="${schemaname// /}" tablename="${tablename// /}" indexname="${indexname// /}" index_size_bytes="${bytes// /}" relpages="${relpages// /}" reltuples="${reltuples// /}" est_bloat_pct="${pct// /}"
    done <<< "$rows"
  else
    _idx_render_rows "schemaname|tablename|indexrelname|index_size|relpages|reltuples|est_bloat_pct" \
      "$(echo "$rows" | awk -F'|' '{print $1"|"$2"|"$3"|"$5"|"$6"|"$7"|"$8}')" "$cnt"
  fi
  return 1
}

_idx_missing() {
  hdr "Missing FK indexes (large tables with unindexed FK leading column)"
  local rows cnt
  rows=$(ksql_q "
    SELECT DISTINCT n.nspname, c.relname,
           a.attname, con.conname,
           sut.n_live_tup
    $_IDX_MISSING_FROM
    WHERE $_IDX_MISSING_WHERE
    ORDER BY sut.n_live_tup DESC;" 2>/dev/null) || rows=""
  if [[ -z "$rows" ]]; then cnt=0; else cnt=$(echo "$rows" | grep -c '.'); fi

  if [[ "$cnt" -eq 0 ]]; then
    ok "No missing FK indexes found"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "idx_missing" "ok" "0" ""
    return 0
  fi

  warn "${cnt} FK column(s) without index on large table(s)"
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_item "idx_missing" "warn" "${cnt}" "run 'idx missing' for detail"
    while IFS='|' read -r schemaname tablename fk_col constraint_name row_count; do
      [[ -z "${schemaname// /}" ]] && continue
      json_row check=idx_missing schemaname="${schemaname// /}" tablename="${tablename// /}" fk_col="${fk_col// /}" constraint_name="${constraint_name// /}" row_count="${row_count// /}"
    done <<< "$rows"
  else
    _idx_render_rows "schemaname|tablename|fk_col|constraint_name|row_count" "$rows" "$cnt"
  fi
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
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}
