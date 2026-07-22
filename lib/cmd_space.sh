# shellcheck shell=bash
_space_frag() {
  # Intentionally a lower/earlier bar than KB_WARN_DEAD_PCT (used by
  # advisor/diagnose's actionable vacuum checks) — this is a soft heads-up,
  # not "go vacuum now", so it keeps its own KB_WARN_FRAG_PCT threshold.
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "space_frag"
  hdr "Table fragmentation (dead tuple ratio > ${KB_WARN_FRAG_PCT}%)"

  local cnt
  cnt=$(ksql_q "
    SELECT count(*) FROM sys_stat_user_tables
    WHERE n_live_tup+n_dead_tup > 1000
      AND n_dead_tup::float/(n_live_tup+n_dead_tup+1) > ${KB_WARN_FRAG_PCT}/100.0;" | tr -d '[:space:]')

  local _exit=0
  if [[ "${cnt:-0}" -eq 0 ]]; then
    ok "No heavily fragmented tables"
    json_item "fragmentation" "ok" "0" ""
  else
    warn "$cnt fragmented table(s) — consider VACUUM"
    json_item "fragmentation" "warn" "$cnt" "dead tuple ratio > ${KB_WARN_FRAG_PCT}%"
    _exit=1

    local rows
    rows=$(ksql_q "
      SELECT schemaname, relname,
             pg_size_pretty(pg_relation_size(schemaname||'.'||relname)) AS size,
             n_live_tup, n_dead_tup,
             round(100.0*n_dead_tup/NULLIF(n_live_tup+n_dead_tup,0),1) AS dead_pct
      FROM sys_stat_user_tables
      WHERE n_live_tup+n_dead_tup > 1000
        AND n_dead_tup::float/(n_live_tup+n_dead_tup+1) > ${KB_WARN_FRAG_PCT}/100.0
      ORDER BY dead_pct DESC
      LIMIT ${TOP_N};")

    if [[ "$OUTPUT_FMT" == "json" ]]; then
      while IFS='|' read -r schemaname relname size live dead dead_pct; do
        [[ -z "${schemaname// /}" ]] && continue
        json_row schemaname="${schemaname// /}" relname="${relname// /}" size="${size// /}" \
          n_live_tup="${live// /}" n_dead_tup="${dead// /}" dead_pct="${dead_pct// /}"
      done <<< "$rows"
    else
      { echo "schemaname|relname|size|n_live_tup|n_dead_tup|dead_pct"; echo "$rows"; } \
        | column -t -s '|'
    fi
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}

_space_all() {
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "space"
  hdr "Storage & space"
  local _exit=0

  # 1. 数据目录磁盘使用率
  local disk_pct disk_info
  disk_pct=$(df "$KB_DATA_DIR" 2>/dev/null | awk 'NR==2{gsub(/%/,"",$5); print $5}')
  disk_info=$(df -h "$KB_DATA_DIR" 2>/dev/null | awk 'NR==2{print $3"/"$2, "used ("$5")"}')
  if [[ -z "$disk_pct" ]]; then
    warn "Cannot read disk usage for $KB_DATA_DIR"
    json_item "disk_usage" "warn" "" "cannot read disk usage for $KB_DATA_DIR"
    _exit=$(_check_level "$_exit" 1)
  elif [[ $disk_pct -ge 90 ]]; then
    fail "Disk usage: $disk_info — FAIL threshold 90%"
    json_item "disk_usage" "fail" "${disk_pct}%" "$disk_info — FAIL threshold 90%"
    _exit=$(_check_level "$_exit" 2)
  elif [[ $disk_pct -ge 80 ]]; then
    warn "Disk usage: $disk_info — WARN threshold 80%"
    json_item "disk_usage" "warn" "${disk_pct}%" "$disk_info — WARN threshold 80%"
    _exit=$(_check_level "$_exit" 1)
  else
    ok "Disk usage: $disk_info"
    json_item "disk_usage" "ok" "${disk_pct}%" "$disk_info"
  fi

  # 2. WAL 目录大小
  local wal_dir
  if [[ -d "$KB_DATA_DIR/sys_wal" ]]; then
    wal_dir="$KB_DATA_DIR/sys_wal"
  else
    wal_dir="$KB_DATA_DIR/pg_wal"
  fi
  if [[ -d "$wal_dir" ]]; then
    local wal_size
    wal_size=$(du -sh "$wal_dir" 2>/dev/null | awk '{print $1}')
    info "WAL directory: $wal_size  ($wal_dir)"
    json_item "wal_dir" "ok" "$wal_size" "$wal_dir"
  fi

  # 3. 各数据库大小
  info "Database sizes:"
  local db_rows
  db_rows=$(ksql_q "
    SELECT datname,
           pg_size_pretty(pg_database_size(datname)) AS size
    FROM sys_database
    WHERE datname NOT IN ('template0','template1')
    ORDER BY pg_database_size(datname) DESC;")
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    while IFS='|' read -r datname size; do
      [[ -z "${datname// /}" ]] && continue
      json_row datname="${datname// /}" size="${size// /}"
    done <<< "$db_rows"
  else
    { echo "datname|size"; echo "$db_rows"; } | column -t -s '|'
  fi

  # 4. Top 10 最大表
  info "Top 10 largest tables (total incl. indexes):"
  local tbl_rows
  tbl_rows=$(ksql_q "
    SELECT schemaname, relname,
           pg_size_pretty(pg_total_relation_size(schemaname||'.'||relname)) AS total_size,
           pg_size_pretty(pg_relation_size(schemaname||'.'||relname))       AS table_size,
           pg_size_pretty(pg_indexes_size(schemaname||'.'||relname))        AS index_size
    FROM sys_stat_user_tables
    ORDER BY pg_total_relation_size(schemaname||'.'||relname) DESC
    LIMIT 10;")
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    while IFS='|' read -r schemaname relname total_size table_size index_size; do
      [[ -z "${schemaname// /}" ]] && continue
      json_row schemaname="${schemaname// /}" relname="${relname// /}" \
        total_size="${total_size// /}" table_size="${table_size// /}" index_size="${index_size// /}"
    done <<< "$tbl_rows"
  else
    { echo "schemaname|relname|total_size|table_size|index_size"; echo "$tbl_rows"; } | column -t -s '|'
  fi

  # 5. 归档目录（仅配置了 archive_command 时）
  if [[ -n "$VERBOSE" ]]; then
    local arch_cmd
    arch_cmd=$(ksql_q "SELECT setting FROM sys_settings WHERE name='archive_command';" | tr -d '[:space:]')
    if [[ -n "$arch_cmd" && "$arch_cmd" != "(disabled)" ]]; then
      info "Archive command: $arch_cmd"
      local arch_stat
      arch_stat=$(ksql_q "SELECT archived_count, failed_count, last_archived_wal, last_archived_time::text FROM sys_stat_archiver;")
      info "Archiver: $arch_stat"
      json_item "archiver" "ok" "$arch_cmd" "$arch_stat"
    fi
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}

cmd_space() {
  local subcmd="${1:-}"
  case "$subcmd" in
    frag) _space_frag; return $? ;;
    "")   _space_all;  return $? ;;
    *)
      echo "Unknown space subcommand: $subcmd (frag)" >&2
      return 1
      ;;
  esac
}
