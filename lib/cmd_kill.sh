_kill_usage() {
  echo "Usage: kbdiag kill [options] [pid...]"
  echo ""
  echo "  <pid>              Cancel query on pid (soft, keeps connection)"
  echo "  --terminate <pid>  Terminate connection (hard)"
  echo "  --long N           Cancel all queries running > N seconds"
  echo "  --idle-txn N       Terminate sessions idle-in-transaction > N seconds"
  echo "  --user <name>      Filter by username (combine with --long/--idle-txn)"
  echo "  --db <name>        Filter by database name"
  echo "  --dry-run          List targets without executing"
  echo "  --force            Skip confirmation for batch operations"
}

_kill_where_clause() {
  local filter_user="$1" filter_db="$2"
  local extra=""
  [[ -n "$filter_user" ]] && extra="${extra} AND usename = '${filter_user//\'/}'"
  [[ -n "$filter_db"   ]] && extra="${extra} AND datname = '${filter_db//\'/}'"
  echo "$extra"
}

_kill_show_sessions() {
  local rows="$1"
  if [[ -z "$rows" ]]; then
    echo "  (no matching sessions)"
    return
  fi
  printf "  %-8s %-12s %-8s %-10s %s\n" "PID" "USER" "DB" "DURATION" "QUERY"
  while IFS='|' read -r pid user db dur query; do
    printf "  %-8s %-12s %-8s %-10s %s\n" \
      "${pid// /}" "${user// /}" "${db// /}" "${dur// /}s" "${query// /}"
  done <<< "$rows"
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
    rows=$(ksql_q "
      SELECT pid, usename, datname,
             EXTRACT(EPOCH FROM (now()-query_start))::int AS dur,
             left(query, 50) AS q
      FROM sys_stat_activity
      WHERE state = 'active'
        AND now() - query_start > interval '${long_n} seconds'
        AND pid <> sys_backend_pid()
        ${extra}
      ORDER BY dur DESC
      LIMIT ${TOP_N};" 2>/dev/null || true)

    local cnt; cnt=$(echo "$rows" | grep -c '.' 2>/dev/null || echo 0)
    [[ -z "$rows" ]] && cnt=0

    if [[ "$OUTPUT_FMT" != "json" ]]; then
      hdr "Sessions matching --long ${long_n}s"
      _kill_show_sessions "$rows"
      echo ""
      echo "  ${cnt} session(s) matching."
    fi

    if [[ $dry_run -eq 1 ]] || [[ $cnt -eq 0 ]]; then
      if [[ "$OUTPUT_FMT" != "json" ]]; then
        [[ $cnt -eq 0 ]] && echo "  Nothing to cancel." || echo "  Run without --dry-run --force to cancel."
      else
        json_begin "kill"; json_end
      fi
      return 0
    fi

    if [[ $force -eq 0 ]]; then
      echo "  Use --force to execute, or --dry-run to preview only." >&2
      return 1
    fi

    local _exit=0
    [[ "$OUTPUT_FMT" == "json" ]] && json_begin "kill"
    while IFS='|' read -r pid user db dur query; do
      pid=$(echo "$pid" | tr -d '[:space:]')
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
      ORDER BY idle_s DESC
      LIMIT ${TOP_N};" 2>/dev/null || true)

    local cnt; cnt=$(echo "$rows" | grep -c '.' 2>/dev/null || echo 0)
    [[ -z "$rows" ]] && cnt=0

    if [[ "$OUTPUT_FMT" != "json" ]]; then
      hdr "Sessions matching --idle-txn ${idle_txn_n}s"
      _kill_show_sessions "$rows"
      echo ""
      echo "  ${cnt} session(s) matching."
    fi

    if [[ $dry_run -eq 1 ]] || [[ $cnt -eq 0 ]]; then
      if [[ "$OUTPUT_FMT" != "json" ]]; then
        [[ $cnt -eq 0 ]] && echo "  Nothing to terminate." || echo "  Run without --dry-run --force to terminate."
      else
        json_begin "kill"; json_end
      fi
      return 0
    fi

    if [[ $force -eq 0 ]]; then
      echo "  Use --force to execute, or --dry-run to preview only." >&2
      return 1
    fi

    local _exit=0
    [[ "$OUTPUT_FMT" == "json" ]] && json_begin "kill"
    while IFS='|' read -r pid user db idle_s query; do
      pid=$(echo "$pid" | tr -d '[:space:]')
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
