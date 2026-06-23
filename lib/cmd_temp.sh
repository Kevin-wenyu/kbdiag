# shellcheck shell=bash
cmd_temp() {
  hdr "Temp file usage"

  # ── Cumulative temp file stats per database ──────────────────────────────────
  info "Cumulative temp file stats (all time):"
  local db_cnt
  db_cnt=$(ksql_q "SELECT count(*) FROM sys_stat_database WHERE temp_bytes > 0;" 2>/dev/null | tr -d '[:space:]' || echo 0)
  if [[ "${db_cnt:-0}" -eq 0 ]]; then
    info "No temp file usage recorded"
  else
    ksql_qh "
      SELECT datname, temp_files, pg_size_pretty(temp_bytes) AS temp_size
      FROM sys_stat_database WHERE temp_bytes > 0 ORDER BY temp_bytes DESC;
    " 2>/dev/null | column -t -s '|' || true
  fi

  echo ""

  # ── Current sessions using temp files ───────────────────────────────────────
  info "Current sessions using temp files:"
  local sess_cnt
  sess_cnt=$(ksql_q "SELECT count(*) FROM sys_stat_activity WHERE temp_blks_written > 0 AND state <> 'idle';" 2>/dev/null | tr -d '[:space:]' || echo 0)
  if [[ "${sess_cnt:-0}" -eq 0 ]]; then
    info "No sessions currently using temp files"
  else
    ksql_qh "
      SELECT pid, usename, temp_blks_read, temp_blks_written,
             left(query, 60) AS query
      FROM sys_stat_activity
      WHERE temp_blks_written > 0 AND state <> 'idle';
    " 2>/dev/null | column -t -s '|' || true
  fi

  echo ""

  # ── Memory settings related to temp file usage ───────────────────────────────
  info "Related settings:"
  ksql_qh "
    SELECT name, setting, unit FROM sys_settings
    WHERE name IN ('work_mem','temp_file_limit','maintenance_work_mem')
    ORDER BY name;" | column -t -s '|' || true
}
