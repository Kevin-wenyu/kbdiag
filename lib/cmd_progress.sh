# shellcheck shell=bash
cmd_progress() {
  hdr "Operation progress"

  # ── VACUUM progress ──────────────────────────────────────────────────────────
  local vac_cnt
  vac_cnt=$(ksql_q "SELECT count(*) FROM sys_stat_progress_vacuum;" 2>/dev/null | tr -d '[:space:]' || echo 0)
  if [[ "${vac_cnt:-0}" -eq 0 ]]; then
    info "No vacuum operations in progress"
  else
    info "VACUUM operations in progress:"
    ksql_qh "
      SELECT p.pid, p.phase, p.heap_blks_scanned, p.heap_blks_total,
             CASE WHEN p.heap_blks_total > 0
                  THEN round(100.0*p.heap_blks_scanned/p.heap_blks_total,1) ELSE 0 END AS pct,
             a.query
      FROM sys_stat_progress_vacuum p JOIN sys_stat_activity a ON a.pid=p.pid;
    " 2>/dev/null | column -t -s '|' || true
  fi

  echo ""

  # ── CREATE INDEX progress ────────────────────────────────────────────────────
  local idx_cnt
  idx_cnt=$(ksql_q "SELECT count(*) FROM sys_stat_progress_create_index;" 2>/dev/null | tr -d '[:space:]' || echo 0)
  if [[ "${idx_cnt:-0}" -eq 0 ]]; then
    info "No index creation in progress"
  else
    info "CREATE INDEX operations in progress:"
    ksql_qh "
      SELECT p.pid, p.phase, p.blocks_done, p.blocks_total,
             CASE WHEN p.blocks_total > 0
                  THEN round(100.0*p.blocks_done/p.blocks_total,1) ELSE 0 END AS pct,
             a.query
      FROM sys_stat_progress_create_index p JOIN sys_stat_activity a ON a.pid=p.pid;
    " 2>/dev/null | column -t -s '|' || true
  fi

  echo ""

  # ── Long-running queries (> 60s) ─────────────────────────────────────────────
  local slow_cnt
  slow_cnt=$(ksql_q "
    SELECT count(*) FROM sys_stat_activity
    WHERE state='active' AND now()-query_start > interval '60 seconds'
      AND pid <> sys_backend_pid();" 2>/dev/null | tr -d '[:space:]' || echo 0)
  if [[ "${slow_cnt:-0}" -eq 0 ]]; then
    info "No long-running queries (> 60s)"
  else
    info "Long-running queries (> 60s):"
    ksql_qh "
      SELECT pid, usename, date_trunc('second', now()-query_start)::text AS duration,
             left(query, 80) AS query
      FROM sys_stat_activity
      WHERE state='active' AND now()-query_start > interval '60 seconds'
        AND pid <> sys_backend_pid() LIMIT $TOP_N;
    " 2>/dev/null | column -t -s '|' || true
  fi
}
