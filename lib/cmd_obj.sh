# shellcheck shell=bash
# cmd_obj.sh — single-table deep dive: size/tuples, vacuum/analyze status,
# index usage, constraints. DBA tier: verifies idx/space's table-level
# findings on one specific object.

cmd_obj() {
  local target="${1:-}"
  if [[ -z "$target" ]]; then
    warn "Usage: kbdiag obj <schema.table>"
    return 1
  fi

  local schema table
  if echo "$target" | grep -q '\.'; then
    schema="${target%%.*}"; table="${target#*.}"
  else
    schema="public"; table="$target"
  fi

  if [[ ! "$schema" =~ ^[A-Za-z_][A-Za-z0-9_]*$ || ! "$table" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
    warn "Invalid schema/table name: $target"
    return 1
  fi

  local _exit=0
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "obj"

  # Table name is a stable identifier, unlike a transient pid/queryid — a
  # nonexistent table is treated as a usage error (unconditional non-zero),
  # not a gated data-availability WARN.
  local exists
  exists=$(ksql_q "SELECT count(*) FROM sys_stat_user_tables WHERE schemaname='$schema' AND relname='$table';" 2>/dev/null | tr -d '[:space:]') || exists=0
  if [[ "${exists:-0}" -eq 0 ]]; then
    warn "Table '$schema.$table' not found in sys_stat_user_tables"
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_item "table" "fail" "$schema.$table" "not found"
      json_end
    fi
    return 1
  fi

  hdr "Object analysis: $schema.$table"

  # 1. Size & tuples — dead_pct verdict reuses the same thresholds as
  # space/bloat (KB_WARN_DEAD_PCT / KB_FAIL_DEAD_PCT).
  local row total_bytes total_size table_bytes table_size index_bytes index_size live dead dead_pct
  row=$(ksql_q "
    SELECT pg_total_relation_size('\"$schema\".\"$table\"'),
           pg_size_pretty(pg_total_relation_size('\"$schema\".\"$table\"')),
           pg_relation_size('\"$schema\".\"$table\"'),
           pg_size_pretty(pg_relation_size('\"$schema\".\"$table\"')),
           pg_indexes_size('\"$schema\".\"$table\"'),
           pg_size_pretty(pg_indexes_size('\"$schema\".\"$table\"')),
           n_live_tup, n_dead_tup,
           round(100.0*n_dead_tup/NULLIF(n_live_tup+n_dead_tup,0),1)
    FROM sys_stat_user_tables WHERE schemaname='$schema' AND relname='$table';" 2>/dev/null) || row=""
  IFS='|' read -r total_bytes total_size table_bytes table_size index_bytes index_size live dead dead_pct <<< "$row"
  # total_size/table_size/index_size are pg_size_pretty() text ("38 MB") —
  # never blanket-strip spaces out of those, only the numeric fields.
  total_bytes="${total_bytes// /}"; table_bytes="${table_bytes// /}"; index_bytes="${index_bytes// /}"
  live="${live// /}"; dead="${dead// /}"; dead_pct="${dead_pct// /}"

  local dead_pct_int="${dead_pct%%.*}"
  if [[ -n "$dead_pct_int" && "$dead_pct_int" -ge "$KB_FAIL_DEAD_PCT" ]]; then
    fail "Dead tuples ${dead_pct}% (>= ${KB_FAIL_DEAD_PCT}%, live=${live:-0} dead=${dead:-0}) — run: kbdiag perf vacuum"
    _exit=2
  elif [[ -n "$dead_pct_int" && "$dead_pct_int" -ge "$KB_WARN_DEAD_PCT" ]]; then
    warn "Dead tuples ${dead_pct}% (>= ${KB_WARN_DEAD_PCT}%, live=${live:-0} dead=${dead:-0}) — run: kbdiag perf vacuum"
    [[ "$_exit" -lt 1 ]] && _exit=1
  else
    ok "Dead tuples ${dead_pct:-0}% (live=${live:-0} dead=${dead:-0})"
  fi
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    local dead_pct_status="ok"
    [[ "$_exit" -eq 1 ]] && dead_pct_status="warn"
    [[ "$_exit" -eq 2 ]] && dead_pct_status="fail"
    json_item "dead_pct" "$dead_pct_status" "${dead_pct:-0}" "live=${live:-0} dead=${dead:-0}"
    json_row check=size total_size_bytes="${total_bytes:-0}" table_size_bytes="${table_bytes:-0}" index_size_bytes="${index_bytes:-0}" n_live_tup="${live:-0}" n_dead_tup="${dead:-0}" dead_pct="${dead_pct:-0}"
  else
    { echo "total_size|table_size|index_size|n_live_tup|n_dead_tup|dead_pct"; echo "$total_size|$table_size|$index_size|$live|$dead|$dead_pct"; } | column -t -s '|'
    echo ""
  fi

  # 2. Vacuum/Analyze status — informational; staleness/drift is colstat's job.
  local va_row last_vac last_autovac last_an last_autoan mods
  va_row=$(ksql_q "
    SELECT last_vacuum::date, last_autovacuum::date, last_analyze::date, last_autoanalyze::date,
           n_mod_since_analyze
    FROM sys_stat_user_tables WHERE schemaname='$schema' AND relname='$table';" 2>/dev/null) || va_row=""
  IFS='|' read -r last_vac last_autovac last_an last_autoan mods <<< "$va_row"
  last_vac="${last_vac// /}"; last_autovac="${last_autovac// /}"; last_an="${last_an// /}"
  last_autoan="${last_autoan// /}"; mods="${mods// /}"
  if [[ "$OUTPUT_FMT" != "json" ]]; then
    info "Vacuum/Analyze status:"
    { echo "last_vacuum|last_autovacuum|last_analyze|last_autoanalyze|mods_since_analyze"
      echo "${last_vac:--}|${last_autovac:--}|${last_an:--}|${last_autoan:--}|${mods:-0}"; } | column -t -s '|'
    echo ""
  else
    json_row check=vacuum_analyze last_vacuum="${last_vac:-}" last_autovacuum="${last_autovac:-}" last_analyze="${last_an:-}" last_autoanalyze="${last_autoan:-}" mods_since_analyze="${mods:-0}"
  fi

  # 3. Indexes — flag any non-unique/non-PK index with zero scans.
  local idx_rows idx_total=0 idx_unused=0
  idx_rows=$(ksql_q "
    SELECT i.indexrelname, pg_relation_size(i.indexrelid), pg_size_pretty(pg_relation_size(i.indexrelid)),
           i.idx_scan, i.idx_tup_read, i.idx_tup_fetch, si.indisunique, si.indisprimary
    FROM sys_stat_user_indexes i
    JOIN sys_index si ON si.indexrelid = i.indexrelid
    WHERE i.schemaname='$schema' AND i.relname='$table'
    ORDER BY i.idx_scan DESC;" 2>/dev/null) || idx_rows=""

  local idx_display=""
  while IFS='|' read -r idxname idxbytes idxsize idxscan idxread idxfetch isuniq ispk; do
    [[ -z "${idxname// /}" ]] && continue
    idx_total=$((idx_total + 1))
    idxscan="${idxscan// /}"; isuniq="${isuniq// /}"; ispk="${ispk// /}"
    local flags=""
    if [[ "$ispk" == "true" ]]; then
      flags="PRIMARY KEY"
    elif [[ "$isuniq" == "true" ]]; then
      flags="UNIQUE"
    fi
    if [[ "${idxscan:-0}" -eq 0 && "$isuniq" != "true" && "$ispk" != "true" ]]; then
      idx_unused=$((idx_unused + 1))
    fi
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_row check=indexes indexrelname="${idxname// /}" index_size_bytes="${idxbytes// /}" idx_scan="$idxscan" idx_tup_read="${idxread// /}" idx_tup_fetch="${idxfetch// /}" flags="$flags"
    else
      idx_display="${idx_display}${idxname}|${idxsize}|${idxscan}|${idxread}|${idxfetch}|${flags}"$'\n'
    fi
  done <<< "$idx_rows"

  if [[ "$idx_total" -eq 0 ]]; then
    ok "No indexes on this table"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "indexes" "ok" "0" ""
  elif [[ "$idx_unused" -gt 0 ]]; then
    warn "${idx_unused} of ${idx_total} index(es) unused (idx_scan=0) — verify with: kbdiag idx unused"
    [[ "$_exit" -lt 1 ]] && _exit=1
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "indexes" "warn" "$idx_unused" "of ${idx_total} unused"
    [[ "$OUTPUT_FMT" != "json" ]] && { echo "indexrelname|size|idx_scan|idx_tup_read|idx_tup_fetch|flags"; printf '%s' "$idx_display"; } | column -t -s '|'
  else
    ok "${idx_total} index(es), all in use"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "indexes" "ok" "$idx_total" "all in use"
    [[ "$OUTPUT_FMT" != "json" ]] && { echo "indexrelname|size|idx_scan|idx_tup_read|idx_tup_fetch|flags"; printf '%s' "$idx_display"; } | column -t -s '|'
  fi
  [[ "$OUTPUT_FMT" != "json" ]] && echo ""

  # 4. Constraints — flag missing PRIMARY KEY.
  local con_rows con_total=0 has_pk=0 con_display=""
  con_rows=$(ksql_q "
    SELECT conname, contype,
           CASE contype WHEN 'p' THEN 'PRIMARY KEY' WHEN 'f' THEN 'FOREIGN KEY'
                        WHEN 'u' THEN 'UNIQUE' WHEN 'c' THEN 'CHECK' END
    FROM sys_constraint WHERE conrelid = '\"$schema\".\"$table\"'::regclass;" 2>/dev/null) || con_rows=""
  while IFS='|' read -r conname contype contypedesc; do
    [[ -z "${conname// /}" ]] && continue
    con_total=$((con_total + 1))
    [[ "${contype// /}" == "p" ]] && has_pk=1
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_row check=constraints conname="${conname// /}" contype="${contype// /}" type="$contypedesc"
    else
      con_display="${con_display}${conname}|${contypedesc}"$'\n'
    fi
  done <<< "$con_rows"

  if [[ "$has_pk" -eq 0 ]]; then
    warn "No PRIMARY KEY defined on $schema.$table"
    [[ "$_exit" -lt 1 ]] && _exit=1
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "constraints" "warn" "$con_total" "no primary key"
  else
    ok "${con_total} constraint(s), PRIMARY KEY present"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "constraints" "ok" "$con_total" ""
  fi
  if [[ "$OUTPUT_FMT" != "json" && "$con_total" -gt 0 ]]; then
    { echo "conname|type"; printf '%s' "$con_display"; } | column -t -s '|'
  fi

  # 5. verbose: Column statistics
  if [[ -n "$VERBOSE" && "$OUTPUT_FMT" != "json" ]]; then
    echo ""
    info "Column statistics:"
    ksql_qh "
      SELECT attname, null_frac, n_distinct, most_common_vals
      FROM sys_stats WHERE schemaname='$schema' AND tablename='$table'
      ORDER BY attname LIMIT $TOP_N;" 2>/dev/null | column -t -s '|' || true
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}
