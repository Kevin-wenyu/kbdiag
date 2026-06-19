cmd_space() {
  hdr "Storage & space"

  # 1. 数据目录磁盘使用率
  local disk_pct disk_info
  disk_pct=$(df "$KB_DATA_DIR" 2>/dev/null | awk 'NR==2{gsub(/%/,"",$5); print $5}')
  disk_info=$(df -h "$KB_DATA_DIR" 2>/dev/null | awk 'NR==2{print $3"/"$2, "used ("$5")"}')
  if [[ -z "$disk_pct" ]]; then
    warn "Cannot read disk usage for $KB_DATA_DIR"
  elif [[ $disk_pct -ge 90 ]]; then
    fail "Disk usage: $disk_info — FAIL threshold 90%"
  elif [[ $disk_pct -ge 80 ]]; then
    warn "Disk usage: $disk_info — WARN threshold 80%"
  else
    ok "Disk usage: $disk_info"
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
  fi

  # 3. 各数据库大小
  info "Database sizes:"
  ksql_q "
    SELECT datname,
           pg_size_pretty(pg_database_size(datname)) AS size
    FROM sys_database
    WHERE datname NOT IN ('template0','template1')
    ORDER BY pg_database_size(datname) DESC;" \
    | column -t -s '|' || true

  # 4. Top 10 最大表
  info "Top 10 largest tables (total incl. indexes):"
  ksql_q "
    SELECT schemaname, relname,
           pg_size_pretty(pg_total_relation_size(schemaname||'.'||relname)) AS total_size,
           pg_size_pretty(pg_relation_size(schemaname||'.'||relname))       AS table_size,
           pg_size_pretty(pg_indexes_size(schemaname||'.'||relname))        AS index_size
    FROM sys_stat_user_tables
    ORDER BY pg_total_relation_size(schemaname||'.'||relname) DESC
    LIMIT 10;" \
    | column -t -s '|' || true

  # 5. 归档目录（仅配置了 archive_command 时）
  if [[ -n "$VERBOSE" ]]; then
    local arch_cmd
    arch_cmd=$(ksql_q "SELECT setting FROM sys_settings WHERE name='archive_command';" | tr -d '[:space:]')
    if [[ -n "$arch_cmd" && "$arch_cmd" != "(disabled)" ]]; then
      info "Archive command: $arch_cmd"
      local arch_stat
      arch_stat=$(ksql_q "SELECT archived_count, failed_count, last_archived_wal, last_archived_time::text FROM sys_stat_archiver;")
      info "Archiver: $arch_stat"
    fi
  fi
}
