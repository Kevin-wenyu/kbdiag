# shellcheck shell=bash
# cmd_kill.sh — cancel/terminate sessions by pid, or in bulk by --long /
# --idle-txn filters. This is an action/judgment command (D2): exit codes
# reflect action outcome directly and unconditionally, no --exit-code needed.

_kill_usage() {
  cat <<'EOF'
Usage: kbdiag kill <pid> [pid...] [--terminate|-T]
       kbdiag kill --long N [--user U] [--db D] [--dry-run|--force]
       kbdiag kill --idle-txn N [--user U] [--db D] [--dry-run|--force]

  <pid>          Cancel (or --terminate) the query/session on this pid
  --long N       Batch: sessions with an active query running > N seconds
  --idle-txn N   Batch: sessions idle-in-transaction for > N seconds
  --user/--db    Filter batch mode to a specific user/database
  --dry-run      Batch: list matches only, take no action
  --force        Batch: required to actually cancel/terminate matches
EOF
}

_kill_where_clause() {
  local filter_user="$1" filter_db="$2"
  local extra=""
  [[ -n "$filter_user" ]] && extra="${extra} AND usename = '${filter_user//\'/}'"
  [[ -n "$filter_db"   ]] && extra="${extra} AND datname = '${filter_db//\'/}'"
  echo "$extra"
}

# Render a captured "$rows" result set as a table, capped at $limit rows —
# the query column keeps its internal spaces (only trimmed, not blanket
# space-stripped, unlike the single-token pid/user/db/dur columns).
_kill_show_sessions() {
  local rows="$1" limit="${2:-999999}"
  if [[ -z "$rows" ]]; then
    echo "  (no matching sessions)"
    return
  fi
  local total; total=$(echo "$rows" | grep -c . || true)
  printf "  %-8s %-12s %-8s %-10s %s\n" "PID" "USER" "DB" "DURATION" "QUERY"
  local shown=0
  while IFS='|' read -r pid user db dur query; do
    [[ -z "${pid// /}" ]] && continue
    shown=$((shown + 1))
    [[ "$shown" -gt "$limit" ]] && break
    printf "  %-8s %-12s %-8s %-10s %s\n" \
      "${pid// /}" "${user// /}" "${db// /}" "${dur// /}s" \
      "$(echo "$query" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
  done <<< "$rows"
  if [[ "$total" -gt "$limit" ]]; then
    echo "  ... $((total - limit)) more, use -v to show all"
  fi
}

cmd_kill() {
  local terminate=0 dry_run=0 force=0
  local long_n="" idle_txn_n="" filter_user="" filter_db=""
  local target_pids=()

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --terminate|-T) terminate=1 ;;
      --dry-run)      dry_run=1 ;;
      --force)        force=1 ;;
      --long)         long_n="${2:-}"; shift ;;
      --idle-txn)     idle_txn_n="${2:-}"; shift ;;
      --user)         filter_user="${2:-}"; shift ;;
      --db)           filter_db="${2:-}"; shift ;;
      ''|--help|-h)   _kill_usage; return 0 ;;
      [0-9]*)         target_pids+=("$1") ;;
      *)              echo "Unknown option: $1" >&2; _kill_usage >&2; return 1 ;;
    esac
    shift
  done

  local _exit=0
  local extra
  extra=$(_kill_where_clause "$filter_user" "$filter_db")

  # ── single-pid mode ──────────────────────────────────────────────────────
  if [[ ${#target_pids[@]} -gt 0 ]]; then
    [[ "$OUTPUT_FMT" == "json" ]] && json_begin "kill"
    for pid in "${target_pids[@]}"; do
      local fn result meta
      if [[ $terminate -eq 1 ]]; then
        fn="sys_terminate_backend"
      else
        fn="sys_cancel_backend"
      fi
      # Get session metadata before acting
      meta=$(ksql_q "SELECT usename, datname,
               EXTRACT(EPOCH FROM (now()-query_start))::int
             FROM sys_stat_activity WHERE pid=$pid;" | tr -d ' ')
      result=$(ksql_q "SELECT ${fn}(${pid});" | tr -d '[:space:]')
      if [[ "$result" == "t" || "$result" == "true" ]]; then
        local user db dur
        IFS='|' read -r user db dur <<< "$meta"
        ok "Cancelled query on pid $pid (user: ${user:-?}, db: ${db:-?}, duration: ${dur:-?}s)"
        json_row "pid=$pid" "action=cancelled" "user=${user:-}" "duration_s=${dur:-0}" "result=ok"
      else
        warn "pid $pid not found or already terminated"
        json_row "pid=$pid" "action=cancel_attempted" "result=not_found"
        _exit=1
      fi
    done
    [[ "$OUTPUT_FMT" == "json" ]] && json_end
    return $_exit
  fi

  # ── batch mode: --long ───────────────────────────────────────────────────
  if [[ -n "$long_n" ]]; then
    local rows
    # No SQL-side LIMIT: the batch execution loop below must act on every
    # matching session, not just the ones shown in the capped display table.
    rows=$(ksql_q "
      SELECT pid, usename, datname,
             EXTRACT(EPOCH FROM (now()-query_start))::int AS dur,
             left(query, 50) AS q
      FROM sys_stat_activity
      WHERE state = 'active'
        AND now() - query_start > interval '${long_n} seconds'
        AND pid <> sys_backend_pid()
        ${extra}
      ORDER BY dur DESC;" 2>/dev/null || true)

    local cnt=0
    [[ -n "$rows" ]] && cnt=$(echo "$rows" | grep -c . || true)

    [[ "$OUTPUT_FMT" == "json" ]] && json_begin "kill"
    if [[ "$OUTPUT_FMT" != "json" ]]; then
      hdr "Sessions matching --long ${long_n}s"
      local show_limit="$TOP_N"
      [[ -n "$VERBOSE" ]] && show_limit=999999
      _kill_show_sessions "$rows" "$show_limit"
      echo ""
    fi
    if [[ "$cnt" -gt 0 ]]; then
      warn "$cnt session(s) matching --long ${long_n}s"
    else
      ok "$cnt session(s) matching --long ${long_n}s"
    fi
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "long_running" "$([[ "$cnt" -gt 0 ]] && echo warn || echo ok)" "$cnt" "> ${long_n}s"

    if [[ $dry_run -eq 1 ]] || [[ $cnt -eq 0 ]]; then
      if [[ "$OUTPUT_FMT" != "json" ]]; then
        if [[ $cnt -eq 0 ]]; then info "Nothing to cancel."; else info "Run without --dry-run, add --force to cancel."; fi
      fi
      [[ "$OUTPUT_FMT" == "json" ]] && json_end
      return 0
    fi

    if [[ $force -eq 0 ]]; then
      [[ "$OUTPUT_FMT" != "json" ]] && echo "  Use --force to execute, or --dry-run to preview only." >&2
      [[ "$OUTPUT_FMT" == "json" ]] && json_end
      return 1
    fi

    while IFS='|' read -r pid user db dur query; do
      pid="${pid// /}"
      [[ -z "$pid" ]] && continue
      local result
      result=$(ksql_q "SELECT sys_cancel_backend(${pid});" | tr -d '[:space:]')
      if [[ "$result" == "t" || "$result" == "true" ]]; then
        ok "Cancelled pid $pid (user: ${user// /}, duration: ${dur// /}s)"
        json_row "pid=$pid" "action=cancelled" "user=${user// /}" "duration_s=${dur// /}" "result=ok"
      else
        warn "pid $pid already gone"
        json_row "pid=$pid" "action=cancel_attempted" "result=not_found"
        _exit=1
      fi
    done <<< "$rows"
    [[ "$OUTPUT_FMT" == "json" ]] && json_end
    return $_exit
  fi

  # ── batch mode: --idle-txn ───────────────────────────────────────────────
  if [[ -n "$idle_txn_n" ]]; then
    local rows
    rows=$(ksql_q "
      SELECT pid, usename, datname,
             EXTRACT(EPOCH FROM (now()-state_change))::int AS idle_s,
             left(query, 50) AS q
      FROM sys_stat_activity
      WHERE state = 'idle in transaction'
        AND now() - state_change > interval '${idle_txn_n} seconds'
        AND pid <> sys_backend_pid()
        ${extra}
      ORDER BY idle_s DESC;" 2>/dev/null || true)

    local cnt=0
    [[ -n "$rows" ]] && cnt=$(echo "$rows" | grep -c . || true)

    [[ "$OUTPUT_FMT" == "json" ]] && json_begin "kill"
    if [[ "$OUTPUT_FMT" != "json" ]]; then
      hdr "Sessions matching --idle-txn ${idle_txn_n}s"
      local show_limit="$TOP_N"
      [[ -n "$VERBOSE" ]] && show_limit=999999
      _kill_show_sessions "$rows" "$show_limit"
      echo ""
    fi
    if [[ "$cnt" -gt 0 ]]; then
      warn "$cnt session(s) matching --idle-txn ${idle_txn_n}s"
    else
      ok "$cnt session(s) matching --idle-txn ${idle_txn_n}s"
    fi
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "idle_txn" "$([[ "$cnt" -gt 0 ]] && echo warn || echo ok)" "$cnt" "> ${idle_txn_n}s"

    if [[ $dry_run -eq 1 ]] || [[ $cnt -eq 0 ]]; then
      if [[ "$OUTPUT_FMT" != "json" ]]; then
        if [[ $cnt -eq 0 ]]; then info "Nothing to terminate."; else info "Run without --dry-run, add --force to terminate."; fi
      fi
      [[ "$OUTPUT_FMT" == "json" ]] && json_end
      return 0
    fi

    if [[ $force -eq 0 ]]; then
      [[ "$OUTPUT_FMT" != "json" ]] && echo "  Use --force to execute, or --dry-run to preview only." >&2
      [[ "$OUTPUT_FMT" == "json" ]] && json_end
      return 1
    fi

    while IFS='|' read -r pid user db idle_s query; do
      pid="${pid// /}"
      [[ -z "$pid" ]] && continue
      local result
      result=$(ksql_q "SELECT sys_terminate_backend(${pid});" | tr -d '[:space:]')
      if [[ "$result" == "t" || "$result" == "true" ]]; then
        ok "Terminated idle-txn pid $pid (user: ${user// /}, idle: ${idle_s// /}s)"
        json_row "pid=$pid" "action=terminated" "user=${user// /}" "idle_s=${idle_s// /}" "result=ok"
      else
        warn "pid $pid already gone"
        json_row "pid=$pid" "action=terminate_attempted" "result=not_found"
        _exit=1
      fi
    done <<< "$rows"
    [[ "$OUTPUT_FMT" == "json" ]] && json_end
    return $_exit
  fi

  # ── no actionable args ────────────────────────────────────────────────────
  _kill_usage
  return 1
}
