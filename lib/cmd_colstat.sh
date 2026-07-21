# shellcheck shell=bash
# cmd_colstat.sh — single-table column statistics deep dive: n_distinct,
# null%, correlation, MCV, and ANALYZE drift. DBA tier: verifies advisor's
# whole-DB stale-stats findings on one specific table.

_colstat_parse_mcv() {
  # Parse "{val1,val2,val3}" and "{0.42,0.31,0.18}" into "val1(42%) val2(31%) val3(18%)"
  local vals_raw="$1" freqs_raw="$2"
  # Strip braces
  vals_raw="${vals_raw#\{}"; vals_raw="${vals_raw%\}}"
  freqs_raw="${freqs_raw#\{}"; freqs_raw="${freqs_raw%\}}"
  [[ -z "$vals_raw" || "$vals_raw" == "N/A" ]] && echo "—" && return

  local IFS=','
  # shellcheck disable=SC2206
  local vals=($vals_raw)
  # shellcheck disable=SC2206
  local freqs=($freqs_raw)
  local result=""
  local i=0
  while [[ $i -lt ${#vals[@]} && $i -lt 3 ]]; do
    local v="${vals[$i]}" f="${freqs[$i]:-0}"
    # Convert float freq to percent integer
    local pct; pct=$(awk "BEGIN{printf \"%d\", ${f}*100}" 2>/dev/null || echo "?")
    result="${result}${v}(${pct}%) "
    i=$(( i + 1 ))
  done
  echo "${result% }"
}

cmd_colstat() {
  local table_arg="${1:-}"
  shift 2>/dev/null || true
  local filter_col=""

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --col) filter_col="${2:-}"; shift ;;
      *)     ;;
    esac
    shift
  done

  if [[ -z "$table_arg" ]]; then
    warn "Usage: kbdiag colstat <schema.table> [--col <column>]"
    return 1
  fi

  local schema table
  if echo "$table_arg" | grep -q '\.'; then
    schema="${table_arg%%.*}"
    table="${table_arg#*.}"
  else
    schema="public"
    table="$table_arg"
  fi

  if [[ ! "$schema" =~ ^[A-Za-z_][A-Za-z0-9_]*$ || ! "$table" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
    warn "Invalid schema/table name: $table_arg"
    return 1
  fi
  if [[ -n "$filter_col" && ! "$filter_col" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
    warn "Invalid column name: $filter_col"
    return 1
  fi

  local _exit=0
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "colstat"

  # Table name is a stable identifier, unlike a transient pid/queryid — a
  # nonexistent table is a usage error (unconditional non-zero), same
  # reasoning as obj.sh.
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

  hdr "Column statistics: $schema.$table"

  # 1. ANALYZE drift — shared math with advisor's whole-DB sweep (KB_DRIFT_EXPR
  # in core.sh, "别重新发明"). KB_WARN_DRIFT_PCT is colstat-local; advisor
  # keeps its own hardcoded 10% (out of scope for this pass).
  local meta last_analyze n_live_tup reltuples drift_pct
  meta=$(ksql_q "
    SELECT coalesce(last_analyze::text, last_autoanalyze::text, 'never'),
           n_live_tup::text,
           reltuples::bigint::text,
           coalesce(round((${KB_DRIFT_EXPR})::numeric, 1)::text, '0')
    FROM sys_stat_user_tables t
    JOIN sys_class sc ON sc.relname = t.relname
    JOIN sys_namespace ns ON ns.oid = sc.relnamespace AND ns.nspname = t.schemaname
    WHERE t.schemaname='$schema' AND t.relname='$table';" 2>/dev/null) || meta=""
  IFS='|' read -r last_analyze n_live_tup reltuples drift_pct <<< "$meta"
  # last_analyze is a timestamp ("2026-07-21 22:48:23") — trim leading/
  # trailing whitespace only, never blanket-strip all spaces (that's the
  # bug class caught earlier in obj.sh: it would collapse the date/time gap).
  last_analyze="${last_analyze:-never}"
  last_analyze="$(echo "$last_analyze" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
  n_live_tup="${n_live_tup// /}"; reltuples="${reltuples// /}"; drift_pct="${drift_pct// /}"

  local drift_warn="${KB_WARN_DRIFT_PCT:-10}"
  local abs_drift="${drift_pct#-}"
  local drift_status="ok"
  if awk "BEGIN{exit !(${abs_drift:-0} > ${drift_warn})}" 2>/dev/null; then
    warn "ANALYZE drift ${drift_pct}% (> ${drift_warn}%, rows at last ANALYZE=${reltuples:-0} current=${n_live_tup:-0}) — consider: ANALYZE ${schema}.${table};"
    _exit=1
    drift_status="warn"
  else
    ok "ANALYZE drift ${drift_pct:-0}% (rows at last ANALYZE=${reltuples:-0} current=${n_live_tup:-0})"
  fi
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_item "drift" "$drift_status" "${drift_pct:-0}" "last_analyze=${last_analyze}"
  else
    echo "  Last ANALYZE: ${last_analyze}"
    echo ""
  fi

  # 2. Per-column stats — capture once, render table + flag low correlation
  # from the same rows (D6: one query, two renderings).
  local col_filter_sql=""
  [[ -n "$filter_col" ]] && col_filter_sql="AND attname = '$filter_col'"
  local corr_warn="${KB_WARN_CORR:-0.3}"

  local col_rows
  col_rows=$(ksql_q "
    SELECT attname, n_distinct::text, round(null_frac*100,1)::text,
           coalesce(round(correlation::numeric,2)::text,'N/A'),
           coalesce(most_common_vals::text,'N/A'),
           coalesce(most_common_freqs::text,'N/A')
    FROM sys_stats
    WHERE schemaname='$schema' AND tablename='$table' ${col_filter_sql}
    ORDER BY attname;" 2>/dev/null) || col_rows=""

  local col_total=0 low_corr_cnt=0 col_display=""
  while IFS='|' read -r col ndist null_pct corr mcv_raw mcf_raw; do
    [[ -z "${col// /}" ]] && continue
    col_total=$((col_total + 1))
    col="${col// /}"; ndist="${ndist// /}"; null_pct="${null_pct// /}"; corr="${corr// /}"
    local mcv; mcv=$(_colstat_parse_mcv "$mcv_raw" "$mcf_raw")
    local flag=""
    if [[ "$corr" != "N/A" ]]; then
      local abs_corr="${corr#-}"
      if awk "BEGIN{exit !(${abs_corr} < ${corr_warn})}" 2>/dev/null; then
        low_corr_cnt=$((low_corr_cnt + 1))
        flag="low-corr"
      fi
    fi
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_row check=column name="$col" n_distinct="$ndist" null_pct="$null_pct" correlation="$corr" mcv_top3="$mcv" flag="$flag"
    else
      col_display="${col_display}${col}|${ndist}|${null_pct}%|${corr}|${mcv}|${flag}"$'\n'
    fi
  done <<< "$col_rows"

  if [[ "$col_total" -eq 0 ]]; then
    warn "No column statistics found for $schema.$table (has ANALYZE ever run?)"
    [[ "$_exit" -lt 1 ]] && _exit=1
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "columns" "warn" "0" "no stats"
  elif [[ "$low_corr_cnt" -gt 0 ]]; then
    warn "${low_corr_cnt} of ${col_total} column(s) with low correlation (< ${corr_warn}) — range scans may be suboptimal"
    [[ "$_exit" -lt 1 ]] && _exit=1
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "columns" "warn" "$low_corr_cnt" "of ${col_total} low correlation"
  else
    ok "${col_total} column(s), correlation healthy"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "columns" "ok" "$col_total" ""
  fi

  if [[ "$OUTPUT_FMT" != "json" && "$col_total" -gt 0 ]]; then
    { echo "column|n_distinct|null_pct|correlation|mcv_top3|flag"; printf '%s' "$col_display"; } | column -t -s '|'
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}
