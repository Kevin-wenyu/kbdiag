# shellcheck shell=bash
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
    echo "Usage: kbdiag colstat <schema.table> [--col <column>]" >&2
    return 1
  fi

  # Split schema.table
  local schema table
  if echo "$table_arg" | grep -q '\.'; then
    schema="${table_arg%%.*}"
    table="${table_arg#*.}"
  else
    schema="public"
    table="$table_arg"
  fi
  # Sanitize
  schema="${schema//\'/}"; table="${table//\'/}"

  # Existence check
  local exists
  exists=$(ksql_q "SELECT count(*) FROM sys_stat_user_tables
                   WHERE schemaname='${schema}' AND relname='${table}';" | tr -d '[:space:]')
  if [[ "${exists:-0}" -eq 0 ]]; then
    echo "Error: table '${schema}.${table}' not found in sys_stat_user_tables" >&2
    return 1
  fi

  # Table-level metadata; drift comes from the shared KB_DRIFT_EXPR so this
  # stays on the same math as advisor's whole-DB sweep.
  local last_analyze n_live_tup reltuples drift_pct
  local meta
  meta=$(ksql_q "
    SELECT coalesce(last_analyze::text, last_autoanalyze::text, 'never') AS last_analyze,
           n_live_tup::text,
           reltuples::bigint::text,
           coalesce(round((${KB_DRIFT_EXPR})::numeric, 1)::text, '0') AS drift_pct
    FROM sys_stat_user_tables t
    JOIN sys_class sc ON sc.relname = t.relname
    JOIN sys_namespace ns ON ns.oid = sc.relnamespace
      AND ns.nspname = t.schemaname
    WHERE t.schemaname='${schema}' AND t.relname='${table}';")

  IFS='|' read -r last_analyze n_live_tup reltuples drift_pct <<< "$meta"
  last_analyze="${last_analyze:-never}"
  last_analyze="$(echo "$last_analyze" | sed 's/^[[:space:]]*//' | sed 's/[[:space:]]*$//')"
  n_live_tup="$(echo "${n_live_tup:-0}" | tr -d '[:space:]')"
  reltuples="$(echo "${reltuples:-0}" | tr -d '[:space:]')"
  drift_pct="$(echo "${drift_pct:-0}" | tr -d '[:space:]')"

  local _exit=0
  local warnings=()

  local col_filter_sql=""
  [[ -n "$filter_col" ]] && col_filter_sql="AND attname = '${filter_col//\'/}'"

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    # JSON output — build manually
    local cols_json=""

    local col_rows
    col_rows=$(ksql_q "
      SELECT attname,
             n_distinct::text,
             round(null_frac * 100, 1)::text AS null_pct,
             coalesce(correlation::text, 'N/A'),
             coalesce(most_common_vals::text, 'N/A'),
             coalesce(most_common_freqs::text, 'N/A')
      FROM sys_stats
      WHERE schemaname='${schema}' AND tablename='${table}'
      ${col_filter_sql}
      ORDER BY attname;" 2>/dev/null || true)

    local first=1
    local json_warnings=()
    while IFS='|' read -r col ndist null_pct corr mcv_raw mcf_raw; do
      [[ -z "$(echo "$col" | tr -d '[:space:]')" ]] && continue
      col="$(echo "$col" | tr -d '[:space:]')"
      ndist="$(echo "$ndist" | tr -d '[:space:]')"
      null_pct="$(echo "$null_pct" | tr -d '[:space:]')"
      corr="$(echo "$corr" | tr -d '[:space:]')"
      local mcv; mcv=$(_colstat_parse_mcv "$mcv_raw" "$mcf_raw")
      # Escape double quotes in mcv for JSON safety
      local mcv_escaped; mcv_escaped="${mcv//\"/\\\"}"
      if [[ $first -eq 1 ]]; then
        cols_json="{\"name\":\"${col}\",\"n_distinct\":\"${ndist}\",\"null_pct\":\"${null_pct}\",\"correlation\":\"${corr}\",\"mcv_top3\":\"${mcv_escaped}\"}"
        first=0
      else
        cols_json="${cols_json},{\"name\":\"${col}\",\"n_distinct\":\"${ndist}\",\"null_pct\":\"${null_pct}\",\"correlation\":\"${corr}\",\"mcv_top3\":\"${mcv_escaped}\"}"
      fi
      # Check correlation warning
      if [[ "$corr" != "N/A" ]]; then
        local abs_corr_json="${corr#-}"
        if awk "BEGIN{exit !(${abs_corr_json} < 0.3)}" 2>/dev/null; then
          json_warnings+=("\"${col}: correlation=${corr}, range scans may be slow\"")
        fi
      fi
    done <<< "$col_rows"

    # Check drift warning
    local abs_drift="${drift_pct#-}"
    if awk "BEGIN{exit !(${abs_drift} > 10.0)}" 2>/dev/null; then
      json_warnings+=("\"drift ${drift_pct}% since last ANALYZE\"")
    fi

    # Build warnings JSON array
    local warnings_json=""
    local wi=0
    for w in "${json_warnings[@]}"; do
      if [[ $wi -eq 0 ]]; then
        warnings_json="${w}"
      else
        warnings_json="${warnings_json},${w}"
      fi
      wi=$(( wi + 1 ))
    done

    echo "{\"table\":\"${schema}.${table}\",\"last_analyze\":\"${last_analyze}\",\"row_drift_pct\":${drift_pct},\"columns\":[${cols_json}],\"warnings\":[${warnings_json}]}"

    # Return 1 if any warnings exist
    if [[ ${#json_warnings[@]} -gt 0 ]]; then
      return 1
    fi
    return 0
  fi

  # Text output
  hdr "Column statistics: ${schema}.${table}"
  echo "  Last ANALYZE: ${last_analyze}"
  printf "  Rows at last ANALYZE: %s  |  Current estimate: %s  |  Drift: %s%%\n" \
    "$reltuples" "$n_live_tup" "$drift_pct"
  echo ""

  printf "  %-20s %-12s %-10s %-6s %s\n" \
    "COLUMN" "N_DISTINCT" "NULL%" "CORR" "MCV_TOP3"
  printf "  %-20s %-12s %-10s %-6s %s\n" \
    "------" "----------" "-----" "----" "--------"

  local col_rows
  col_rows=$(ksql_q "
    SELECT attname,
           n_distinct::text,
           round(null_frac * 100, 1)::text AS null_pct,
           coalesce(round(correlation::numeric, 2)::text, 'N/A'),
           coalesce(most_common_vals::text, 'N/A'),
           coalesce(most_common_freqs::text, 'N/A')
    FROM sys_stats
    WHERE schemaname='${schema}' AND tablename='${table}'
    ${col_filter_sql}
    ORDER BY attname;" 2>/dev/null || true)

  while IFS='|' read -r col ndist null_pct corr mcv_raw mcf_raw; do
    local col_trim; col_trim="$(echo "$col" | tr -d '[:space:]')"
    [[ -z "$col_trim" ]] && continue
    local ndist_trim; ndist_trim="$(echo "$ndist" | tr -d '[:space:]')"
    local null_pct_trim; null_pct_trim="$(echo "$null_pct" | tr -d '[:space:]')"
    local corr_trim; corr_trim="$(echo "$corr" | tr -d '[:space:]')"
    local mcv; mcv=$(_colstat_parse_mcv "$mcv_raw" "$mcf_raw")
    printf "  %-20s %-12s %-10s %-6s %s\n" \
      "$col_trim" "$ndist_trim" "${null_pct_trim}%" "$corr_trim" "$mcv"

    # Warn on poor correlation (between -0.3 and 0.3, excluding N/A)
    if [[ "$corr_trim" != "N/A" ]]; then
      local abs_corr
      abs_corr="${corr_trim#-}"
      if awk "BEGIN{exit !(${abs_corr} < 0.3)}" 2>/dev/null; then
        warnings+=("${col_trim}: correlation=${corr_trim} — low correlation, range scans may be suboptimal")
      fi
    fi
  done <<< "$col_rows"

  echo ""

  # Drift warning
  local abs_drift="${drift_pct#-}"
  if awk "BEGIN{exit !(${abs_drift} > 10.0)}" 2>/dev/null; then
    warn "Drift ${drift_pct}% since last ANALYZE — consider: ANALYZE ${schema}.${table};"
    warnings+=("drift")
    _exit=1
  fi

  for w in "${warnings[@]}"; do
    [[ "$w" == "drift" ]] && continue
    warn "$w"
    _exit=1
  done

  [[ $_exit -eq 0 ]] && ok "Statistics look healthy"
  return $_exit
}
