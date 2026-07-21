# shellcheck shell=bash
# cmd_progress.sh — snapshot of in-flight VACUUM / CREATE INDEX / long-running
# queries. VACUUM and CREATE INDEX activity is informational (running is
# normal); long-running queries is the one actionable signal, mirrors
# locks.sh's wait-time treatment.

cmd_progress() {
  hdr "Operation progress"
  local _exit=0
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "progress"

  # ── VACUUM progress ──────────────────────────────────────────────────────────
  local vac_rows vac_cnt
  vac_rows=$(ksql_q "
    SELECT p.pid, p.phase, p.heap_blks_scanned, p.heap_blks_total,
           CASE WHEN p.heap_blks_total > 0
                THEN round(100.0*p.heap_blks_scanned/p.heap_blks_total,1) ELSE 0 END,
           a.query
    FROM sys_stat_progress_vacuum p JOIN sys_stat_activity a ON a.pid=p.pid;" 2>/dev/null) || vac_rows=""
  if [[ -z "$vac_rows" ]]; then vac_cnt=0; else vac_cnt=$(echo "$vac_rows" | grep -c '.'); fi

  if [[ "$vac_cnt" -eq 0 ]]; then
    ok "No vacuum operations in progress"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "vacuum_progress" "ok" "0" ""
  else
    info "${vac_cnt} vacuum operation(s) in progress"
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_item "vacuum_progress" "ok" "$vac_cnt" "${vac_cnt} vacuum(s) running"
      while IFS='|' read -r pid phase scanned total pct query; do
        [[ -z "${pid// /}" ]] && continue
        json_row check=vacuum_progress pid="${pid// /}" phase="${phase// /}" heap_blks_scanned="${scanned// /}" heap_blks_total="${total// /}" pct="${pct// /}" query="$(printf '%s' "$query" | tr '\n' ' ')"
      done <<< "$vac_rows"
    else
      { echo "pid|phase|blks_scanned|blks_total|pct|query"; echo "$vac_rows"; } | column -t -s '|'
    fi
  fi
  [[ "$OUTPUT_FMT" != "json" ]] && echo ""

  # ── CREATE INDEX progress ────────────────────────────────────────────────────
  local idx_rows idx_cnt
  idx_rows=$(ksql_q "
    SELECT p.pid, p.phase, p.blocks_done, p.blocks_total,
           CASE WHEN p.blocks_total > 0
                THEN round(100.0*p.blocks_done/p.blocks_total,1) ELSE 0 END,
           a.query
    FROM sys_stat_progress_create_index p JOIN sys_stat_activity a ON a.pid=p.pid;" 2>/dev/null) || idx_rows=""
  if [[ -z "$idx_rows" ]]; then idx_cnt=0; else idx_cnt=$(echo "$idx_rows" | grep -c '.'); fi

  if [[ "$idx_cnt" -eq 0 ]]; then
    ok "No index creation in progress"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "create_index_progress" "ok" "0" ""
  else
    info "${idx_cnt} CREATE INDEX operation(s) in progress"
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_item "create_index_progress" "ok" "$idx_cnt" "${idx_cnt} create index op(s) running"
      while IFS='|' read -r pid phase blk_done blk_total pct query; do
        [[ -z "${pid// /}" ]] && continue
        json_row check=create_index_progress pid="${pid// /}" phase="${phase// /}" blocks_done="${blk_done// /}" blocks_total="${blk_total// /}" pct="${pct// /}" query="$(printf '%s' "$query" | tr '\n' ' ')"
      done <<< "$idx_rows"
    else
      { echo "pid|phase|blocks_done|blocks_total|pct|query"; echo "$idx_rows"; } | column -t -s '|'
    fi
  fi
  [[ "$OUTPUT_FMT" != "json" ]] && echo ""

  # ── Long-running queries ─────────────────────────────────────────────────────
  local warn_dur="${KB_WARN_LONG_QUERY:-60}"
  local slow_rows slow_cnt
  slow_rows=$(ksql_q "
    SELECT pid, usename, extract(epoch FROM now()-query_start)::bigint,
           date_trunc('second', now()-query_start)::text,
           left(query, 80)
    FROM sys_stat_activity
    WHERE state='active' AND now()-query_start > interval '${warn_dur} seconds'
      AND pid <> sys_backend_pid()
    ORDER BY query_start ASC;" 2>/dev/null) || slow_rows=""
  if [[ -z "$slow_rows" ]]; then slow_cnt=0; else slow_cnt=$(echo "$slow_rows" | grep -c '.'); fi

  if [[ "$slow_cnt" -eq 0 ]]; then
    ok "No long-running queries (> ${warn_dur}s)"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "long_running_queries" "ok" "0" "threshold=${warn_dur}s"
  else
    local plural="ies"; [[ "$slow_cnt" -eq 1 ]] && plural="y"
    warn "${slow_cnt} long-running quer${plural} (> ${warn_dur}s) — verify with: kbdiag sql <pid>"
    _exit=1
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_item "long_running_queries" "warn" "$slow_cnt" "threshold=${warn_dur}s"
      while IFS='|' read -r pid usename secs duration query; do
        [[ -z "${pid// /}" ]] && continue
        json_row check=long_running_queries pid="${pid// /}" usename="${usename// /}" duration_seconds="${secs// /}" duration="${duration// /}" query="$(printf '%s' "$query" | tr '\n' ' ')"
      done <<< "$slow_rows"
    else
      local show_rows
      if [[ -z "$VERBOSE" && "$slow_cnt" -gt "$TOP_N" ]]; then
        show_rows=$(echo "$slow_rows" | head -n "$TOP_N" | awk -F'|' '{print $1"|"$2"|"$4"|"$5}')
      else
        show_rows=$(echo "$slow_rows" | awk -F'|' '{print $1"|"$2"|"$4"|"$5}')
      fi
      { echo "pid|usename|duration|query"; echo "$show_rows"; } | column -t -s '|'
      [[ -z "$VERBOSE" && "$slow_cnt" -gt "$TOP_N" ]] && echo "  (showing top ${TOP_N} of ${slow_cnt} — use -v for full list)"
    fi
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}
