# shellcheck shell=bash
# cmd_stmt.sh — SQL statement statistics (sys_stat_statements AWR-style)

# Shared with cmd_diagnose.sh's _diag_stmt_top, cmd_stat.sh's
# _stat_section_top_sql, and cmd_perf.sh's "top" subcommand — edit only here
# to change which statements count as "active" in the various Top SQL views.
_STMT_ACTIVE_WHERE="calls > 0"

_stmt_check_ext() {
  local cnt
  cnt=$(ksql_q "SELECT count(*) FROM sys_catalog.sys_class WHERE relname='sys_stat_statements';" 2>/dev/null | tr -d '[:space:]') || cnt=""
  [[ "${cnt:-0}" -gt 0 ]]
}

_stmt_install_hint() {
  local current_val
  current_val=$(ksql_q "SHOW shared_preload_libraries;" 2>/dev/null | tr -d '[:space:]') || current_val=""
  warn "sys_stat_statements extension is not loaded"
  printf "\n"
  printf "  Current:  shared_preload_libraries = '%s'\n" "${current_val:-<empty>}"
  printf "\n"
  printf "  To enable:\n"
  printf "    1. Edit \$KB_DATA_DIR/kingbase.conf\n"
  printf "       Add 'sys_stat_statements' to shared_preload_libraries\n"
  printf "    2. Restart KingbaseES\n"
  printf "    3. Optional tuning:\n"
  printf "         sys_stat_statements.max = 1000\n"
  printf "         sys_stat_statements.track = all\n"
  printf "\n"
  printf "  Check current value:  kbdiag params shared_preload_libraries\n"
}

# Render a "top N" section as an aligned table. Truncates the last field
# (always the query summary) to 60 chars unless VERBOSE is set — same rows,
# no second query, per the "one query, two renderings" convention.
_stmt_render_section() {
  local title="$1" header="$2" rows="$3"
  hdr "$title"
  if [[ -z "$rows" ]]; then
    echo "  (no data)"
    return
  fi
  {
    echo "$header"
    echo "$rows" | awk -F'|' -v verbose="${VERBOSE:-}" '{
      OFS="|"; q=$NF;
      if (!verbose) { q=substr(q,1,60) }
      $NF=q;
      print NR, $0
    }'
  } | column -t -s '|'
}

_stmt_overview() {
  local stats_reset
  stats_reset=$(ksql_q "SELECT coalesce(stats_reset::text, 'unknown') FROM sys_stat_bgwriter;" 2>/dev/null | tr -d '[:space:]') || stats_reset="unknown"

  local rows_mean cnt_mean worst_mean
  rows_mean=$(ksql_q "
    SELECT queryid::text, calls::text,
           round(mean_exec_time::numeric, 1)::text,
           round(max_exec_time::numeric, 1)::text,
           round(100.0 * total_exec_time / NULLIF(sum(total_exec_time) OVER (), 0), 1)::text,
           translate(left(query, 200), E'\n\r|', '   ')
    FROM sys_stat_statements
    WHERE $_STMT_ACTIVE_WHERE
    ORDER BY mean_exec_time DESC
    LIMIT ${TOP_N};" 2>/dev/null) || rows_mean=""
  if [[ -z "$rows_mean" ]]; then cnt_mean=0; else cnt_mean=$(echo "$rows_mean" | grep -c '.'); fi
  worst_mean=$(echo "$rows_mean" | head -1 | awk -F'|' '{gsub(/ /,"",$3); print $3}')

  local slow_ms=$(( KB_SLOW_THRESHOLD * 1000 ))
  if [[ "$cnt_mean" -eq 0 ]]; then
    info "No statements recorded since stats reset (${stats_reset})"
  elif awk "BEGIN{exit !(${worst_mean:-0} > ${slow_ms})}"; then
    warn "Slow statements present: worst mean ${worst_mean}ms > ${KB_SLOW_THRESHOLD}s threshold (data since ${stats_reset})"
    _STMT_EXIT=1
  else
    ok "No statement averaging over ${KB_SLOW_THRESHOLD}s (worst mean: ${worst_mean}ms, data since ${stats_reset})"
  fi
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_item "slow_statements" "$([[ "${_STMT_EXIT:-0}" -eq 1 ]] && echo warn || echo ok)" "${worst_mean:-0}ms" "threshold=${KB_SLOW_THRESHOLD}s stats_reset=${stats_reset}"
  fi

  local rows_total
  rows_total=$(ksql_q "
    SELECT queryid::text, calls::text,
           round((total_exec_time / 1000.0)::numeric, 1)::text,
           round(mean_exec_time::numeric, 1)::text,
           round(100.0 * total_exec_time / NULLIF(sum(total_exec_time) OVER (), 0), 1)::text,
           translate(left(query, 200), E'\n\r|', '   ')
    FROM sys_stat_statements
    WHERE $_STMT_ACTIVE_WHERE
    ORDER BY total_exec_time DESC
    LIMIT ${TOP_N};" 2>/dev/null) || rows_total=""

  local rows_io
  rows_io=$(ksql_q "
    SELECT queryid::text, calls::text,
           shared_blks_read::text,
           round(100.0 * shared_blks_hit / NULLIF(shared_blks_hit + shared_blks_read, 0), 1)::text,
           translate(left(query, 200), E'\n\r|', '   ')
    FROM sys_stat_statements
    WHERE $_STMT_ACTIVE_WHERE
    ORDER BY shared_blks_read DESC
    LIMIT ${TOP_N};" 2>/dev/null) || rows_io=""

  local rows_calls
  rows_calls=$(ksql_q "
    SELECT queryid::text, calls::text,
           round(mean_exec_time::numeric, 1)::text,
           translate(left(query, 200), E'\n\r|', '   ')
    FROM sys_stat_statements
    WHERE $_STMT_ACTIVE_WHERE
    ORDER BY calls DESC
    LIMIT ${TOP_N};" 2>/dev/null) || rows_calls=""

  if [[ "$OUTPUT_FMT" != "json" ]]; then
    _stmt_render_section "Top ${TOP_N} by mean execution time" "#|queryid|calls|mean_ms|max_ms|pct_total|query" "$rows_mean"
    _stmt_render_section "Top ${TOP_N} by total execution time (calls x mean)" "#|queryid|calls|total_s|mean_ms|pct_total|query" "$rows_total"
    _stmt_render_section "Top ${TOP_N} by IO (cache misses, shared_blks_read)" "#|queryid|calls|blks_read|hit_rate|query" "$rows_io"
    _stmt_render_section "Top ${TOP_N} by call frequency" "#|queryid|calls|mean_ms|query" "$rows_calls"
  else
    while IFS='|' read -r qid calls mean_ms max_ms pct qtext; do
      [[ -z "${qid// /}" ]] && continue
      json_row section=mean queryid="${qid// /}" calls="${calls// /}" mean_exec_time="${mean_ms// /}" max_exec_time="${max_ms// /}" pct_total="${pct// /}" query="$qtext"
    done <<< "$rows_mean"

    while IFS='|' read -r qid calls total_s mean_ms pct qtext; do
      [[ -z "${qid// /}" ]] && continue
      json_row section=total queryid="${qid// /}" calls="${calls// /}" total_exec_time_s="${total_s// /}" mean_exec_time="${mean_ms// /}" pct_total="${pct// /}" query="$qtext"
    done <<< "$rows_total"

    while IFS='|' read -r qid calls blks_read hit_rate qtext; do
      [[ -z "${qid// /}" ]] && continue
      json_row section=io queryid="${qid// /}" calls="${calls// /}" shared_blks_read="${blks_read// /}" hit_rate="${hit_rate// /}" query="$qtext"
    done <<< "$rows_io"

    while IFS='|' read -r qid calls mean_ms qtext; do
      [[ -z "${qid// /}" ]] && continue
      json_row section=calls queryid="${qid// /}" calls="${calls// /}" mean_exec_time="${mean_ms// /}" query="$qtext"
    done <<< "$rows_calls"
  fi
}

_stmt_detail() {
  local queryid="$1"
  local row
  row=$(ksql_q "
    SELECT calls, mean_exec_time, min_exec_time, max_exec_time, stddev_exec_time,
           mean_plan_time,
           shared_blks_hit, shared_blks_read,
           round(100.0 * shared_blks_hit / NULLIF(shared_blks_hit + shared_blks_read, 0), 1) AS hit_ratio,
           temp_blks_read, temp_blks_written,
           translate(query, E'\n\r|', '   ')
    FROM sys_stat_statements
    WHERE queryid::text = '${queryid}';" 2>/dev/null) || row=""

  if [[ -z "$row" ]]; then
    warn "queryid ${queryid} not found in sys_stat_statements"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "queryid" "warn" "$queryid" "not found"
    _STMT_EXIT=1
    return 0
  fi

  local calls mean_ms min_ms max_ms stddev_ms plan_ms blks_hit blks_read hit_ratio temp_r temp_w sql_text
  IFS='|' read -r calls mean_ms min_ms max_ms stddev_ms plan_ms blks_hit blks_read hit_ratio temp_r temp_w sql_text <<< "$row"
  calls="${calls// /}"; mean_ms="${mean_ms// /}"; min_ms="${min_ms// /}"; max_ms="${max_ms// /}"
  stddev_ms="${stddev_ms// /}"; plan_ms="${plan_ms// /}"; blks_hit="${blks_hit// /}"; blks_read="${blks_read// /}"
  hit_ratio="${hit_ratio// /}"; temp_r="${temp_r// /}"; temp_w="${temp_w// /}"

  hdr "Statement detail — queryid ${queryid}"

  local unstable=0
  if awk "BEGIN{exit !(${stddev_ms:-0} > ${mean_ms:-0} * 0.5)}"; then
    unstable=1
    warn "Execution time unstable: stddev ${stddev_ms}ms exceeds 50% of mean ${mean_ms}ms"
    _STMT_EXIT=1
  else
    ok "Execution time stable (stddev ${stddev_ms}ms, mean ${mean_ms}ms)"
  fi
  [[ "$OUTPUT_FMT" == "json" ]] && json_item "stability" "$([[ "$unstable" -eq 1 ]] && echo warn || echo ok)" "${stddev_ms}ms" "mean=${mean_ms}ms threshold=50%"

  local low_hit=0
  if awk "BEGIN{exit !(${hit_ratio:-100} < ${KB_WARN_HIT})}"; then
    low_hit=1
    warn "Buffer hit ratio ${hit_ratio}% below ${KB_WARN_HIT}% threshold"
    _STMT_EXIT=1
  else
    ok "Buffer hit ratio ${hit_ratio}% (threshold ${KB_WARN_HIT}%)"
  fi
  [[ "$OUTPUT_FMT" == "json" ]] && json_item "hit_ratio" "$([[ "$low_hit" -eq 1 ]] && echo warn || echo ok)" "${hit_ratio}%" "threshold=${KB_WARN_HIT}%"

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_row queryid="$queryid" calls="$calls" mean_exec_time="$mean_ms" min_exec_time="$min_ms" \
      max_exec_time="$max_ms" stddev_exec_time="$stddev_ms" mean_plan_time="$plan_ms" \
      shared_blks_hit="$blks_hit" shared_blks_read="$blks_read" hit_ratio="$hit_ratio" \
      temp_blks_read="$temp_r" temp_blks_written="$temp_w" query="$sql_text"
    return 0
  fi

  echo ""
  echo "Calls               : ${calls}"
  echo "Mean / Min / Max    : ${mean_ms}ms / ${min_ms}ms / ${max_ms}ms"
  echo "Plan time (avg)     : ${plan_ms}ms"
  echo "Buffer hit / read   : ${blks_hit} / ${blks_read}"
  echo "Temp blocks r/w     : ${temp_r} / ${temp_w}"
  echo ""
  echo "SQL:"
  echo "$sql_text"

  if [[ -n "$VERBOSE" ]]; then
    local explain_sql
    explain_sql=$(printf '%s' "$sql_text" | sed "s/\\\$\([0-9][0-9]*\)/'param_\1'/g")
    echo ""
    echo "Verification path — fill in real parameter values before running:"
    echo "  EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)"
    # shellcheck disable=SC2001
    echo "$explain_sql" | sed 's/^/  /'
    echo ""
    echo "Note: \$N placeholders were replaced with 'param_N' — substitute real values before running."
  fi
}

cmd_stmt() {
  local queryid="${1:-}"
  _STMT_EXIT=0

  if ! _stmt_check_ext; then
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      local cur_val
      cur_val=$(ksql_q "SHOW shared_preload_libraries;" 2>/dev/null | tr -d '[:space:]') || cur_val=""
      json_begin "stmt"
      json_item "extension" "warn" "not_loaded" "sys_stat_statements missing from shared_preload_libraries (current: ${cur_val:-<empty>})"
      json_end
    else
      _stmt_install_hint
    fi
    [[ -n "$EXIT_CODE_MODE" ]] && return 1
    return 0
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "stmt"

  if [[ -n "$queryid" ]]; then
    _stmt_detail "$queryid"
  else
    _stmt_overview
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end

  [[ -n "$EXIT_CODE_MODE" ]] && return "$_STMT_EXIT"
  return 0
}
