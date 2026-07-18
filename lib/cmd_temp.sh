# shellcheck shell=bash
# Presentation-type command: KingbaseES has no per-session live temp columns in
# sys_stat_activity (verified), so only cumulative db-level and per-statement
# figures exist — no live threshold to judge, hence INFO verdicts only.
cmd_temp() {
  hdr "Temp file usage"

  local dbrows db_cnt tot_files tot_mb
  dbrows=$(ksql_q "
    SELECT datname, temp_files, pg_size_pretty(temp_bytes), temp_bytes
    FROM sys_stat_database WHERE temp_bytes > 0 ORDER BY temp_bytes DESC;")
  read -r db_cnt tot_files tot_mb < <(awk -F'|' '
    NF { n++; f += $2; b += $4 }
    END { printf "%d %d %.0f\n", n, f, b / 1024 / 1024 }' <<< "$dbrows")

  # per-statement temp usage needs sys_stat_statements; degrade gracefully
  local stmtrows
  stmtrows=$(ksql_q "
    SELECT queryid, calls, pg_size_pretty(temp_blks_written::bigint * 8192),
           translate(left(query, 100), E'\n\r|', '   ')
    FROM sys_stat_statements WHERE temp_blks_written > 0
    ORDER BY temp_blks_written DESC LIMIT $TOP_N;" 2>/dev/null) || stmtrows=""

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_begin "temp"
    json_item "temp_cumulative" "ok" "${tot_mb}MB" "$tot_files files across $db_cnt database(s) since stats reset"
    while IFS='|' read -r db files size bytes; do
      [[ -z "${db// /}" ]] && continue
      json_row "scope=database" "datname=${db// /}" "temp_files=${files// /}" \
        "temp_size=${size# }" "temp_bytes=${bytes// /}"
    done <<< "$dbrows"
    while IFS='|' read -r qid calls size query; do
      [[ -z "${qid// /}" ]] && continue
      json_row "scope=statement" "queryid=${qid// /}" "calls=${calls// /}" \
        "temp_written=${size# }" "query=$query"
    done <<< "$stmtrows"
    json_end
    return 0
  fi

  if [[ "$db_cnt" -eq 0 ]]; then
    ok "Temp usage: none recorded since stats reset"
  else
    info "Temp usage (cumulative): $tot_files files, ${tot_mb}MB across $db_cnt database(s)"
  fi

  if [[ -z "$QUIET" ]]; then
    if [[ "$db_cnt" -gt 0 ]]; then
      echo
      { echo "DATABASE|TEMP_FILES|TEMP_SIZE"
        awk -F'|' 'NF { print $1 "|" $2 "|" $3 }' <<< "$dbrows"
      } | column -t -s '|' || true
    fi
    if [[ -n "${stmtrows// /}" ]]; then
      echo
      info "Top statements by temp written (cumulative):"
      { echo "QUERYID|CALLS|TEMP_WRITTEN|QUERY"
        awk -F'|' 'NF { print $1 "|" $2 "|" $3 "|" substr($4, 1, 60) }' <<< "$stmtrows"
      } | column -t -s '|' || true
    fi
    if [[ -n "$VERBOSE" ]]; then
      echo
      info "Related settings:"
      ksql_qh "
        SELECT name, setting, unit FROM sys_settings
        WHERE name IN ('work_mem', 'temp_file_limit', 'maintenance_work_mem')
        ORDER BY name;" | column -t -s '|' || true
    fi
  fi

  return 0
}
