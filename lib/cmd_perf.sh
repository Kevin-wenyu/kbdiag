# shellcheck shell=bash
# cmd_perf.sh — performance diagnostics: slow queries, bloat, vacuum lag,
# unused indexes, wait events, table I/O, WAL/checkpoint pressure, top
# resource-consuming statements.
#
# Every subcommand captures its rows once; verdict and -v detail render
# from the same captured data (no second DB round-trip for -v).

_perf_slow() {
  hdr "Slow queries (running > ${KB_SLOW_THRESHOLD}s)"
  local rows cnt
  rows=$(ksql_q "
    SELECT pid, usename, extract(epoch FROM now()-query_start)::int,
           coalesce(client_addr::text, '-'), translate(left(query, 200), E'\n\r|', '   ')
    FROM sys_stat_activity
    WHERE state = 'active'
      AND now() - query_start > interval '${KB_SLOW_THRESHOLD} seconds'
      AND pid <> sys_backend_pid()
    ORDER BY query_start;") || rows=""
  cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$rows")
  [[ "$cnt" -gt 0 ]] && { (( 1 > _PERF_EXIT )) && _PERF_EXIT=1; }

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    if [[ "$cnt" -gt 0 ]]; then
      json_item "slow_queries" "warn" "$cnt" "running > ${KB_SLOW_THRESHOLD}s"
    else
      json_item "slow_queries" "ok" "0" "threshold ${KB_SLOW_THRESHOLD}s"
    fi
    while IFS='|' read -r pid user dur addr query; do
      [[ -z "${pid// /}" ]] && continue
      json_row "scope=slow_query" "pid=${pid// /}" "usename=${user// /}" \
        "duration_s=${dur// /}" "client_addr=${addr// /}" "query=$query"
    done <<< "$rows"
    return
  fi

  if [[ "$cnt" -gt 0 ]]; then
    warn "Slow queries: $cnt running > ${KB_SLOW_THRESHOLD}s"
  else
    ok "Slow queries: none running > ${KB_SLOW_THRESHOLD}s"
  fi

  if [[ -z "$QUIET" && "$cnt" -gt 0 ]]; then
    if [[ -n "$VERBOSE" ]]; then
      { echo "PID|USER|DURATION|CLIENT_ADDR|QUERY"
        awk -F'|' "$AWK_FMT_DUR"'NF { print $1"|"$2"|"fmt($3)"|"$4"|"$5 }' <<< "$rows"
      } | column -t -s '|' || true
    else
      { echo "PID|USER|DURATION|QUERY"
        awk -F'|' "$AWK_FMT_DUR"'NF { print $1"|"$2"|"fmt($3)"|"substr($5,1,60) }' <<< "$rows"
      } | column -t -s '|' || true
    fi
  fi
}

_perf_bloat() {
  # Threshold shared with advisor/diagnose's dead-tuple checks (live_pct = 100 - dead_pct)
  local live_floor=$(( 100 - KB_WARN_DEAD_PCT ))
  hdr "Table bloat (live ratio < ${live_floor}%)"
  local rows cnt
  rows=$(ksql_q "
    SELECT schemaname, relname,
           pg_relation_size(schemaname||'.'||relname),
           pg_size_pretty(pg_relation_size(schemaname||'.'||relname)),
           n_live_tup, n_dead_tup,
           CASE WHEN n_live_tup+n_dead_tup > 0
                THEN round(100.0*n_live_tup/(n_live_tup+n_dead_tup),1)
                ELSE 100 END AS live_pct
    FROM sys_stat_user_tables
    WHERE n_live_tup+n_dead_tup > 1000
      AND (n_live_tup::float/(n_live_tup+n_dead_tup+1)) < ${live_floor}/100.0
    ORDER BY n_dead_tup DESC;") || rows=""
  cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$rows")
  [[ "$cnt" -gt 0 ]] && { (( 1 > _PERF_EXIT )) && _PERF_EXIT=1; }

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    if [[ "$cnt" -gt 0 ]]; then
      json_item "bloated_tables" "warn" "$cnt" "live ratio < ${live_floor}%"
    else
      json_item "bloated_tables" "ok" "0" "live ratio threshold ${live_floor}%"
    fi
    while IFS='|' read -r schema rel bytes size_pretty live dead pct; do
      [[ -z "${schema// /}" ]] && continue
      json_row "scope=bloat" "table=${schema// /}.${rel// /}" "bytes=${bytes// /}" \
        "n_live_tup=${live// /}" "n_dead_tup=${dead// /}" "live_pct=${pct// /}"
    done <<< "$rows"
    return
  fi

  if [[ "$cnt" -gt 0 ]]; then
    if [[ -n "$VERBOSE" || "$cnt" -le "$TOP_N" ]]; then
      warn "Bloated tables: $cnt with live ratio < ${live_floor}%"
    else
      warn "Bloated tables: $cnt with live ratio < ${live_floor}% (showing top ${TOP_N})"
    fi
  else
    ok "Bloated tables: none (live ratio threshold ${live_floor}%)"
  fi

  if [[ -z "$QUIET" && "$cnt" -gt 0 ]]; then
    if [[ -n "$VERBOSE" ]]; then
      { echo "TABLE|SIZE|N_LIVE_TUP|N_DEAD_TUP|LIVE_PCT"
        awk -F'|' 'NF { print $1"."$2"|"$4"|"$5"|"$6"|"$7"%" }' <<< "$rows"
      } | column -t -s '|' || true
    else
      { echo "TABLE|SIZE|LIVE_PCT"
        awk -F'|' 'NF { print $1"."$2"|"$4"|"$7"%" }' <<< "$rows" | head -n "$TOP_N"
      } | column -t -s '|' || true
    fi
  fi
}

_perf_vacuum() {
  hdr "Vacuum lag (dead tuples vs autovacuum threshold, or stale > 7d)"
  local rows cnt over_cnt
  rows=$(ksql_q "
    SELECT t.schemaname, t.relname,
           t.n_dead_tup,
           round(
             (SELECT setting::int FROM sys_settings WHERE name='autovacuum_vacuum_threshold') +
             (SELECT setting::float FROM sys_settings WHERE name='autovacuum_vacuum_scale_factor') * c.reltuples
           )::text AS threshold,
           round(100.0 * t.n_dead_tup /
             NULLIF(
               (SELECT setting::int FROM sys_settings WHERE name='autovacuum_vacuum_threshold') +
               (SELECT setting::float FROM sys_settings WHERE name='autovacuum_vacuum_scale_factor') * c.reltuples,
             0), 1) AS pct,
           coalesce(t.last_autovacuum::text, 'never'),
           coalesce(t.last_autoanalyze::text, 'never')
    FROM sys_stat_user_tables t
    JOIN sys_class c ON c.oid = t.relid
    WHERE t.n_dead_tup > 1000
       OR t.last_autovacuum < now() - interval '7 days'
       OR t.last_autovacuum IS NULL
    ORDER BY t.n_dead_tup DESC;") || rows=""
  cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$rows")
  over_cnt=$(awk -F'|' '$5+0 >= 100 { c++ } END { print c+0 }' <<< "$rows")
  [[ "$over_cnt" -gt 0 ]] && { (( 1 > _PERF_EXIT )) && _PERF_EXIT=1; }

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    if [[ "$over_cnt" -gt 0 ]]; then
      json_item "vacuum_lag" "warn" "$over_cnt" "table(s) over autovacuum threshold"
    else
      json_item "vacuum_lag" "ok" "0" "no table over autovacuum threshold"
    fi
    while IFS='|' read -r schema rel dead threshold pct lastvac lastan; do
      [[ -z "${schema// /}" ]] && continue
      json_row "scope=vacuum" "table=${schema// /}.${rel// /}" "n_dead_tup=${dead// /}" \
        "threshold=${threshold// /}" "pct_of_threshold=${pct// /}" \
        "last_autovacuum=${lastvac# }" "last_autoanalyze=${lastan# }"
    done <<< "$rows"
    return
  fi

  if [[ "$over_cnt" -gt 0 ]]; then
    warn "Vacuum lag: $over_cnt table(s) over autovacuum dead-tuple threshold"
  else
    ok "Vacuum lag: no table over autovacuum dead-tuple threshold"
  fi
  [[ "$cnt" -gt "$over_cnt" ]] && \
    info "Also stale (no autovacuum in 7d): $(( cnt - over_cnt ))"

  if [[ -z "$QUIET" && "$cnt" -gt 0 ]]; then
    if [[ -n "$VERBOSE" ]]; then
      { echo "TABLE|DEAD_TUP|THRESHOLD|PCT|LAST_AUTOVACUUM|LAST_AUTOANALYZE"
        awk -F'|' 'NF { print $1"."$2"|"$3"|"$4"|"$5"%|"$6"|"$7 }' <<< "$rows"
      } | column -t -s '|' || true
    else
      { echo "TABLE|DEAD_TUP|PCT_OF_THRESHOLD|LAST_AUTOVACUUM"
        awk -F'|' 'NF { print $1"."$2"|"$3"|"$5"%|"$6 }' <<< "$rows" | head -n "$TOP_N"
      } | column -t -s '|' || true
    fi
  fi
}

_perf_index() {
  hdr "Unused indexes (0 scans since last stats reset)"
  local rows cnt
  rows=$(ksql_q "
    SELECT schemaname, relname, indexrelname,
           pg_relation_size(indexrelid),
           pg_size_pretty(pg_relation_size(indexrelid))
    FROM sys_stat_user_indexes
    WHERE idx_scan = 0
      AND indexrelname NOT LIKE '%_pkey'
    ORDER BY pg_relation_size(indexrelid) DESC;") || rows=""
  cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$rows")
  [[ "$cnt" -gt 0 ]] && { (( 1 > _PERF_EXIT )) && _PERF_EXIT=1; }

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    if [[ "$cnt" -gt 0 ]]; then
      json_item "unused_indexes" "warn" "$cnt" "0 scans since last stats reset"
    else
      json_item "unused_indexes" "ok" "0" ""
    fi
    # shellcheck disable=SC2034  # size_pretty only used by the text-mode renderer below
    while IFS='|' read -r schema rel idx bytes size_pretty; do
      [[ -z "${schema// /}" ]] && continue
      json_row "scope=unused_index" "table=${schema// /}.${rel// /}" "index=${idx// /}" \
        "bytes=${bytes// /}"
    done <<< "$rows"
    return
  fi

  if [[ "$cnt" -gt 0 ]]; then
    if [[ -n "$VERBOSE" || "$cnt" -le "$TOP_N" ]]; then
      warn "Unused indexes: $cnt with 0 scans"
    else
      warn "Unused indexes: $cnt with 0 scans (showing top ${TOP_N})"
    fi
  else
    ok "Unused indexes: none found"
  fi

  if [[ -z "$QUIET" && "$cnt" -gt 0 ]]; then
    { echo "TABLE|INDEX|SIZE"
      if [[ -n "$VERBOSE" ]]; then
        awk -F'|' 'NF { print $1"."$2"|"$3"|"$5 }' <<< "$rows"
      else
        awk -F'|' 'NF { print $1"."$2"|"$3"|"$5 }' <<< "$rows" | head -n "$TOP_N"
      fi
    } | column -t -s '|' || true
  fi
}

_perf_wait() {
  hdr "Wait event distribution (active sessions)"
  local rows cnt
  rows=$(ksql_q "
    SELECT wait_event_type, wait_event, count(*),
           round(avg(EXTRACT(EPOCH FROM (now()-query_start))),1)
    FROM sys_stat_activity
    WHERE $_WAIT_EVENT_WHERE
    GROUP BY wait_event_type, wait_event
    ORDER BY 3 DESC;") || rows=""
  cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$rows")
  local total; total=$(awk -F'|' '{ s+=$3 } END { print s+0 }' <<< "$rows")

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_item "waiting_sessions" "ok" "$total" "$cnt distinct wait event(s)"
    while IFS='|' read -r wtype wevent wcnt avgsec; do
      [[ -z "${wtype// /}" ]] && continue
      json_row "scope=wait_event" "type=${wtype// /}" "event=${wevent// /}" \
        "sessions=${wcnt// /}" "avg_sec=${avgsec// /}"
    done <<< "$rows"
    return
  fi

  info "Waiting sessions: $total across $cnt distinct wait event(s)"

  if [[ -z "$QUIET" && "$cnt" -gt 0 ]]; then
    { echo "TYPE|EVENT|SESSIONS|AVG_SEC"
      awk -F'|' 'NF' <<< "$rows" | head -n "$TOP_N"
    } | column -t -s '|' || true
  fi
}

_perf_io() {
  hdr "Table I/O hotspots (by physical reads, hit rate < ${KB_WARN_HITRATE:-90}% flagged)"
  local warn_hit="${KB_WARN_HITRATE:-90}"
  local rows cnt worst
  rows=$(ksql_q "
    SELECT schemaname, relname,
           heap_blks_read, heap_blks_hit,
           CASE WHEN (heap_blks_read+heap_blks_hit)=0 THEN 100
                ELSE round(100.0*heap_blks_hit/(heap_blks_read+heap_blks_hit),1) END AS hit_pct
    FROM sys_statio_user_tables
    WHERE heap_blks_read > 0
    ORDER BY heap_blks_read DESC;") || rows=""
  cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$rows")
  worst=$(awk -F'|' 'BEGIN{m=100} NF { if ($5+0<m) m=$5+0 } END { print m }' <<< "$rows")
  local bad_cnt; bad_cnt=$(awk -F'|' -v t="$warn_hit" 'NF && $5+0<t { c++ } END { print c+0 }' <<< "$rows")
  [[ "$bad_cnt" -gt 0 ]] && { (( 1 > _PERF_EXIT )) && _PERF_EXIT=1; }

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    if [[ "$bad_cnt" -gt 0 ]]; then
      json_item "low_hit_rate_tables" "warn" "$bad_cnt" "hit rate < ${warn_hit}% (worst ${worst}%)"
    else
      json_item "low_hit_rate_tables" "ok" "0" "threshold ${warn_hit}%"
    fi
    while IFS='|' read -r schema rel read_blk hit_blk pct; do
      [[ -z "${schema// /}" ]] && continue
      json_row "scope=table_io" "table=${schema// /}.${rel// /}" \
        "heap_blks_read=${read_blk// /}" "heap_blks_hit=${hit_blk// /}" "hit_pct=${pct// /}"
    done <<< "$rows"
    return
  fi

  if [[ "$cnt" -eq 0 ]]; then
    ok "Table I/O: no tables with physical reads recorded"
  elif [[ "$bad_cnt" -gt 0 ]]; then
    warn "Table I/O: $bad_cnt table(s) with hit rate < ${warn_hit}% (worst ${worst}%)"
  else
    ok "Table I/O: all tables >= ${warn_hit}% hit rate (worst ${worst}%)"
  fi

  if [[ -z "$QUIET" && "$cnt" -gt 0 ]]; then
    { echo "TABLE|HEAP_BLKS_READ|HEAP_BLKS_HIT|HIT_PCT"
      awk -F'|' 'NF { print $1"."$2"|"$3"|"$4"|"$5"%" }' <<< "$rows" | head -n "$TOP_N"
    } | column -t -s '|' || true
  fi
}

_perf_wal() {
  hdr "WAL and checkpoint statistics (cumulative)"
  local row timed req write_s sync_s bufs_ckpt bufs_clean bufs_backend ckpt_sz clean_sz backend_sz
  row=$(bgwriter_stats) || row=""
  IFS='|' read -r timed req write_s sync_s bufs_ckpt bufs_clean bufs_backend ckpt_sz clean_sz backend_sz <<< "$row"
  timed="${timed// /}"; req="${req// /}"

  local req_warn=""
  if [[ "$timed" =~ ^[0-9]+$ && "$req" =~ ^[0-9]+$ && $(( timed + req )) -ge 10 && "$req" -gt "$timed" ]]; then
    req_warn=1
    (( 1 > _PERF_EXIT )) && _PERF_EXIT=1
  fi

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    if [[ -n "$req_warn" ]]; then
      json_item "checkpoint_pressure" "warn" "req=$req/timed=$timed" "more requested than scheduled checkpoints — consider raising max_wal_size"
    else
      json_item "checkpoint_pressure" "ok" "req=$req/timed=$timed" ""
    fi
    json_row "checkpoints_timed=${timed}" "checkpoints_req=${req}" \
      "write_time_s=${write_s// /}" "sync_time_s=${sync_s// /}" \
      "buffers_checkpoint=${bufs_ckpt// /}" "buffers_clean=${bufs_clean// /}" \
      "buffers_backend=${bufs_backend// /}"
    return
  fi

  if [[ -n "$req_warn" ]]; then
    warn "Checkpoints: $req requested vs $timed timed — forced checkpoints may indicate max_wal_size is too small"
  else
    ok "Checkpoints: $timed timed, $req requested"
  fi
  if [[ -z "$QUIET" ]]; then
    printf '  write time: %ss   sync time: %ss\n' "${write_s// /}" "${sync_s// /}"
    printf '  buffers written — checkpoint: %s (%s)   bgwriter: %s (%s)   backend: %s (%s)\n' \
      "${bufs_ckpt// /}" "${ckpt_sz# }" "${bufs_clean// /}" "${clean_sz# }" "${bufs_backend// /}" "${backend_sz# }"
  fi
}

_perf_top() {
  local mode="${1:-buffers}"
  hdr "Top statements by ${mode}"
  case "$mode" in
    buffers|reads|writes|cpu|temp) ;;
    *)
      echo "Unknown top mode: $mode (buffers|reads|writes|cpu|temp)" >&2
      return 1
      ;;
  esac

  if ! _stmt_check_ext >/dev/null 2>&1; then
    info "sys_stat_statements extension not installed — top-statement stats unavailable"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "top_${mode}" "skip" "not installed" ""
    return 0
  fi

  # KingbaseES's sys_stat_activity has no per-session I/O columns (verified against
  # information_schema.columns), so "top" is statement-level via sys_stat_statements,
  # not the live per-session ranking mature Postgres tooling offers.
  local order_expr label
  case "$mode" in
    buffers) order_expr="shared_blks_hit + shared_blks_read"; label="shared_blks_hit+read" ;;
    reads)   order_expr="shared_blks_read + local_blks_read"; label="blks_read" ;;
    writes)  order_expr="shared_blks_written + local_blks_written"; label="blks_written" ;;
    cpu)     order_expr="total_exec_time"; label="total_exec_time_ms" ;;
    temp)    order_expr="temp_blks_read + temp_blks_written"; label="temp_blks" ;;
  esac

  local rows
  rows=$(ksql_q "
    SELECT calls, round(${order_expr}::numeric, 1),
           translate(left(query, 100), E'\n\r|', '   ')
    FROM sys_stat_statements
    WHERE $_STMT_ACTIVE_WHERE
    ORDER BY ${order_expr} DESC
    LIMIT ${TOP_N};" 2>/dev/null) || rows=""
  local cnt; cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$rows")

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_item "top_${mode}" "ok" "$cnt" "statement-level, top ${TOP_N} by ${label}"
    while IFS='|' read -r calls val query; do
      [[ -z "${calls// /}" ]] && continue
      json_row "scope=top_${mode}" "calls=${calls// /}" "${label}=${val// /}" "query=$query"
    done <<< "$rows"
    return
  fi

  info "Top ${TOP_N} statements by ${label} (cumulative, statement-level)"
  if [[ -z "$QUIET" && "$cnt" -gt 0 ]]; then
    local label_hdr; label_hdr=$(echo "$label" | tr '[:lower:]' '[:upper:]')
    { echo "CALLS|${label_hdr}|QUERY"
      if [[ -n "$VERBOSE" ]]; then
        awk -F'|' 'NF' <<< "$rows"
      else
        awk -F'|' 'NF { print $1"|"$2"|"substr($3,1,60) }' <<< "$rows"
      fi
    } | column -t -s '|' || true
  fi
}

cmd_perf() {
  local subcmd="${1:-}"
  shift 2>/dev/null || true
  _PERF_EXIT=0
  local _dispatch_rc=0

  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "perf"

  case "$subcmd" in
    slow)   _perf_slow   ;;
    bloat)  _perf_bloat  ;;
    vacuum) _perf_vacuum ;;
    index)  _perf_index  ;;
    wait)   _perf_wait   ;;
    io)     _perf_io     ;;
    wal)    _perf_wal    ;;
    top)    _perf_top "${1:-buffers}" || _dispatch_rc=$? ;;
    "")
      _perf_slow
      _perf_bloat
      _perf_vacuum
      _perf_index
      _perf_wait
      _perf_io
      _perf_wal
      ;;
    *)
      [[ "$OUTPUT_FMT" == "json" ]] && json_end
      echo "Unknown perf subcommand: $subcmd (slow|bloat|vacuum|index|wait|io|wal|top)" >&2
      return 1
      ;;
  esac

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ "$_dispatch_rc" -ne 0 ]] && return "$_dispatch_rc"
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_PERF_EXIT"
  return 0
}
