# shellcheck shell=bash
cmd_stat() {
  local interval=5 count=1
  local i=0

  # Parse arguments (skip empty strings from dispatch)
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --interval=*) interval="${1#--interval=}" ;;
      --interval)   shift; interval="${1:-5}" ;;
      --count=*)    count="${1#--count=}" ;;
      --count)      shift; count="${1:-1}" ;;
      "")           ;; # skip empty arg from dispatch
    esac
    shift
  done

  hdr "Instance throughput (${interval}s sample)"
  local warn_hit="${KB_WARN_HITRATE:-90}" warn_rb="${KB_WARN_ROLLBACK_PCT:-10}" _exit=0

  # One snapshot pair feeds both the throughput and temp-files deltas —
  # a separate sleep per section would double stat's runtime.
  local snap1 snap2 tps rps hit_pct rb_pct dl_d tf_s tb_s
  while [[ $i -lt $count ]]; do
    snap1=$(ksql_q "SELECT xact_commit, xact_rollback, blks_read, blks_hit, deadlocks, temp_files, temp_bytes FROM sys_stat_database WHERE datname=current_database();")
    sleep "$interval"
    snap2=$(ksql_q "SELECT xact_commit, xact_rollback, blks_read, blks_hit, deadlocks, temp_files, temp_bytes FROM sys_stat_database WHERE datname=current_database();")

    read -r tps rps hit_pct rb_pct dl_d tf_s tb_s < <(printf "%s\n%s\n" "$snap1" "$snap2" | awk -F'|' -v iv="$interval" '
      NR==1 { c1=$1; r1=$2; br1=$3; bh1=$4; d1=$5; f1=$6; b1=$7 }
      NR==2 {
        dc = $1-c1; dr = $2-r1; dbr = $3-br1; dbh = $4-bh1
        tot = dbh + dbr
        hit = (tot > 0) ? 100.0*dbh/tot : 100
        xact = dc + dr
        rbp = (xact > 0) ? 100.0*dr/xact : 0
        printf "%.1f %.1f %.1f %.1f %d %.2f %.0f\n", dc/iv, dr/iv, hit, rbp, $5-d1, ($6-f1)/iv, ($7-b1)/iv
      }')

    [[ "$OUTPUT_FMT" != "json" ]] && _stat_verdicts
    i=$(( i + 1 ))
  done
  # verdicts from the last sample drive the exit code
  awk -v h="$hit_pct" -v wh="$warn_hit" -v r="$rb_pct" -v wr="$warn_rb" -v d="$dl_d" \
    'BEGIN { exit !(h < wh || r > wr || d > 0) }' && _exit=1

  # capture remaining sections once; text and json render from the same data
  local wait_rows ckpt_row sql_rows=""
  wait_rows=$(ksql_q "SELECT wait_event_type, wait_event, count(*)
    FROM sys_stat_activity WHERE $_WAIT_EVENT_WHERE
    GROUP BY wait_event_type, wait_event ORDER BY 3 DESC LIMIT 5;" 2>/dev/null) || wait_rows=""
  ckpt_row=$(bgwriter_stats) || ckpt_row=""
  if _stmt_check_ext >/dev/null 2>&1; then
    sql_rows=$(ksql_q "SELECT calls::text,
      round(total_exec_time::numeric, 1)::text,
      round(mean_exec_time::numeric, 1)::text,
      translate(left(query, 100), E'\n\r|', '   ')
      FROM sys_stat_statements
      WHERE $_STMT_ACTIVE_WHERE
      ORDER BY total_exec_time DESC
      LIMIT 5;" 2>/dev/null) || sql_rows=""
  fi

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_begin "stat"
    json_item "tps_commit" "ok" "$tps" "per second over ${interval}s sample"
    json_item "tps_rollback" "ok" "$rps" "per second"
    if awk -v h="$hit_pct" -v t="$warn_hit" 'BEGIN { exit !(h < t) }'; then
      json_item "buffer_hit_pct" "warn" "$hit_pct" "< threshold ${warn_hit}%"
    else
      json_item "buffer_hit_pct" "ok" "$hit_pct" "threshold ${warn_hit}%"
    fi
    if awk -v r="$rb_pct" -v t="$warn_rb" 'BEGIN { exit !(r > t) }'; then
      json_item "rollback_pct" "warn" "$rb_pct" "> threshold ${warn_rb}%"
    else
      json_item "rollback_pct" "ok" "$rb_pct" "threshold ${warn_rb}%"
    fi
    if [[ "${dl_d:-0}" -gt 0 ]]; then
      json_item "deadlocks_sample" "warn" "$dl_d" "during ${interval}s sample"
    else
      json_item "deadlocks_sample" "ok" "0" "during ${interval}s sample"
    fi
    json_item "temp_files_per_s" "ok" "$tf_s" ""
    json_item "temp_bytes_per_s" "ok" "$tb_s" ""
    while IFS='|' read -r wtype wevent cnt; do
      [[ -z "${wtype// /}" ]] && continue
      json_row "scope=wait_event" "type=${wtype// /}" "event=${wevent// /}" "sessions=${cnt// /}"
    done <<< "$wait_rows"
    while IFS='|' read -r calls total_ms mean_ms query; do
      [[ -z "${calls// /}" ]] && continue
      json_row "scope=top_sql" "calls=${calls// /}" "total_ms=${total_ms// /}" \
        "mean_ms=${mean_ms// /}" "query=$query"
    done <<< "$sql_rows"
    json_end
    [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
    return 0
  fi

  if [[ -z "$QUIET" ]]; then
    if [[ -n "${wait_rows// /}" ]]; then
      echo
      info "Wait events during sample (top 5):"
      { echo "TYPE|EVENT|SESSIONS"
        awk -F'|' 'NF' <<< "$wait_rows"
      } | column -t -s '|' || true
    fi
    if [[ -n "${ckpt_row// /}" ]]; then
      echo
      info "Checkpoint activity (cumulative):"
      echo "$ckpt_row" | while IFS='|' read -r timed req write_s sync_s bufs _; do
        printf "  timed/requested: %s / %s   write %ss   sync %ss   buffers %s\n" \
          "${timed// /}" "${req// /}" "${write_s// /}" "${sync_s// /}" "${bufs// /}"
      done || true
    fi
    if [[ -n "${sql_rows// /}" ]]; then
      echo
      info "Top SQL by total execution time:"
      { echo "CALLS|TOTAL_MS|MEAN_MS|QUERY"
        if [[ -n "$VERBOSE" ]]; then
          awk -F'|' 'NF' <<< "$sql_rows"
        else
          awk -F'|' 'NF { print $1 "|" $2 "|" $3 "|" substr($4, 1, 60) }' <<< "$sql_rows"
        fi
      } | column -t -s '|' || true
    fi
  fi

  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}

# verdict block for one sample; reads the computed metrics from caller scope
_stat_verdicts() {
  info "Throughput: $tps commits/s, $rps rollbacks/s (${interval}s sample)"
  info "Temp files: ${tf_s}/s, ${tb_s} bytes/s during sample"
  if awk -v h="$hit_pct" -v t="$warn_hit" 'BEGIN { exit !(h < t) }'; then
    warn "Buffer hit rate: ${hit_pct}% < threshold ${warn_hit}% (during sample)"
  else
    ok "Buffer hit rate: ${hit_pct}% (threshold ${warn_hit}%)"
  fi
  if awk -v r="$rb_pct" -v t="$warn_rb" 'BEGIN { exit !(r > t) }'; then
    warn "Rollback ratio: ${rb_pct}% > threshold ${warn_rb}%"
  else
    ok "Rollback ratio: ${rb_pct}% (threshold ${warn_rb}%)"
  fi
  if [[ "${dl_d:-0}" -gt 0 ]]; then
    warn "Deadlocks: $dl_d during ${interval}s sample"
  else
    ok "Deadlocks: 0 during sample"
  fi
}
