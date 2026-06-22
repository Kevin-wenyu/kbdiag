# cmd_stmt.sh — SQL 历史统计（sys_stat_statements AWR 风格）

_stmt_check_ext() {
  local cnt
  cnt=$(ksql_q "SELECT count(*) FROM sys_catalog.sys_class WHERE relname='sys_stat_statements';" 2>/dev/null | tr -d '[:space:]')
  [[ "${cnt:-0}" -gt 0 ]]
}

_stmt_overview() {
  local stats_reset
  stats_reset=$(ksql_q "SELECT coalesce(stats_reset::text, 'unknown') FROM sys_stat_bgwriter;" 2>/dev/null | tr -d '[:space:]')

  printf "\n${BOLD}==> SQL 历史统计  [数据覆盖: %s → 现在]${RESET}\n\n" "$stats_reset"

  printf "${BOLD}── Top %d by 平均耗时 ──────────────────────────────────────────────────────────────────────────────────────${RESET}\n" "$TOP_N"
  printf " # | queryid           | calls    | mean_ms   | max_ms    | %%total | SQL 摘要\n"
  ksql_q "
    SELECT row_number() OVER (ORDER BY mean_exec_time DESC)::text,
           queryid::text,
           calls::text,
           round(mean_exec_time::numeric, 1)::text,
           round(max_exec_time::numeric, 1)::text,
           round(100.0 * total_exec_time / NULLIF(sum(total_exec_time) OVER (), 0), 1)::text || '%',
           left(replace(query, E'\n', ' '), 60)
    FROM sys_stat_statements
    WHERE calls > 0
    ORDER BY mean_exec_time DESC
    LIMIT ${TOP_N};" 2>/dev/null | column -t -s '|' || true

  echo ""
  printf "${BOLD}── Top %d by 总耗时 (calls × mean) ──────────────────────────────────────────────────────────────────────${RESET}\n" "$TOP_N"
  printf " # | queryid           | calls     | total_s   | mean_ms | %%total | SQL 摘要\n"
  ksql_q "
    SELECT row_number() OVER (ORDER BY total_exec_time DESC)::text,
           queryid::text,
           calls::text,
           round((total_exec_time/1000.0)::numeric, 1)::text,
           round(mean_exec_time::numeric, 1)::text,
           round(100.0 * total_exec_time / NULLIF(sum(total_exec_time) OVER (), 0), 1)::text || '%',
           left(replace(query, E'\n', ' '), 60)
    FROM sys_stat_statements
    WHERE calls > 0
    ORDER BY total_exec_time DESC
    LIMIT ${TOP_N};" 2>/dev/null | column -t -s '|' || true

  echo ""
  printf "${BOLD}── Top %d by IO（cache miss，shared_blks_read）────────────────────────────────────────────────────────────${RESET}\n" "$TOP_N"
  printf " # | queryid           | calls  | blks_read  | hit_rate | SQL 摘要\n"
  ksql_q "
    SELECT row_number() OVER (ORDER BY shared_blks_read DESC)::text,
           queryid::text,
           calls::text,
           shared_blks_read::text,
           round(100.0 * shared_blks_hit / NULLIF(shared_blks_hit + shared_blks_read, 0), 1)::text || '%',
           left(replace(query, E'\n', ' '), 60)
    FROM sys_stat_statements
    WHERE calls > 0
    ORDER BY shared_blks_read DESC
    LIMIT ${TOP_N};" 2>/dev/null | column -t -s '|' || true

  echo ""
  printf "${BOLD}── Top %d by 调用频率 ──────────────────────────────────────────────────────────────────────────────────────${RESET}\n" "$TOP_N"
  printf " # | queryid           | calls     | mean_ms | SQL 摘要\n"
  ksql_q "
    SELECT row_number() OVER (ORDER BY calls DESC)::text,
           queryid::text,
           calls::text,
           round(mean_exec_time::numeric, 1)::text,
           left(replace(query, E'\n', ' '), 60)
    FROM sys_stat_statements
    WHERE calls > 0
    ORDER BY calls DESC
    LIMIT ${TOP_N};" 2>/dev/null | column -t -s '|' || true

  echo ""
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
           query
    FROM sys_stat_statements
    WHERE queryid::text = '${queryid}';" 2>/dev/null || true)

  if [[ -z "$row" ]]; then
    warn "未找到 queryid=${queryid}"
    return 0
  fi

  local calls mean_ms min_ms max_ms stddev_ms plan_ms blks_hit blks_read hit_ratio temp_r temp_w sql_text
  IFS='|' read -r calls mean_ms min_ms max_ms stddev_ms plan_ms blks_hit blks_read hit_ratio temp_r temp_w sql_text <<< "$row"

  printf "\n${BOLD}==> SQL 详情  queryid: %s${RESET}\n\n" "$queryid"

  printf "${BOLD}── 执行统计 ──────────────────────────────────────────────────────────────────────────────────────────────────${RESET}\n"
  printf "调用次数    : %s\n" "${calls// /}"
  printf "平均耗时    : %s ms\n" "${mean_ms// /}"
  printf "最小 / 最大 : %s ms / %s ms\n" "${min_ms// /}" "${max_ms// /}"

  local stddev_note=""
  if awk "BEGIN{exit !(${stddev_ms// /} + 0 > ${mean_ms// /} + 0 * 0.5 + 0)}"; then
    stddev_note="  ← 高标准差，执行时间不稳定"
  fi
  printf "标准差      : %s ms%s\n" "${stddev_ms// /}" "$stddev_note"
  printf "规划时间    : %s ms (avg)\n" "${plan_ms// /}"
  echo ""

  printf "${BOLD}── IO 分解 ───────────────────────────────────────────────────────────────────────────────────────────────────${RESET}\n"
  printf "shared_blks_hit   : %s\n" "${blks_hit// /}"
  printf "shared_blks_read  : %s\n" "${blks_read// /}"

  local hit_note=""
  if awk "BEGIN{exit !(${hit_ratio// /} + 0 < ${KB_WARN_HIT})}"; then
    hit_note="  ← 低于正常水平"
  fi
  printf "Buffer 命中率     : %s%%%s\n" "${hit_ratio// /}" "$hit_note"
  printf "temp_blks_read    : %s\n" "${temp_r// /}"
  printf "temp_blks_written : %s\n" "${temp_w// /}"
  echo ""

  printf "${BOLD}── SQL 全文 ──────────────────────────────────────────────────────────────────────────────────────────────────${RESET}\n"
  echo "$sql_text"
  echo ""

  local explain_sql
  explain_sql=$(printf '%s' "$sql_text" | sed "s/\\\$\([0-9][0-9]*\)/'param_\1'  -- 替换为实际值/g")
  printf "${BOLD}── 验证路径 ──────────────────────────────────────────────────────────────────────────────────────────────────${RESET}\n"
  printf "执行计划（填入实际参数后运行）:\n"
  printf "  EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)\n"
  echo "$explain_sql" | sed 's/^/  /'
  echo ""
  printf "注意: SQL 中 \$N 为占位符，已替换为 'param_N'，运行前请换成真实值\n"
}

cmd_stmt() {
  local queryid="${1:-}"

  if ! _stmt_check_ext; then
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      echo '{"command":"stmt","error":"sys_stat_statements not available","hint":"shared_preload_libraries = '"'"'sys_stat_statements'"'"' (需重启)"}'
    else
      warn "sys_stat_statements 扩展未启用"
      echo "  启用方式: shared_preload_libraries = 'sys_stat_statements'  (需重启)"
    fi
    return 0
  fi

  if [[ -n "$queryid" ]]; then
    _stmt_detail "$queryid"
  else
    _stmt_overview
  fi
}
