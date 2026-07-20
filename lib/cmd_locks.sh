# shellcheck shell=bash
_locks_wait() {
  hdr "Waiting locks"
  local warn_wait="${KB_WARN_LOCK_WAIT:-60}" _exit=0

  # one query captures full blocking chains; verdicts and both tables render from it
  local rows
  rows=$(ksql_q "
    SELECT w.pid, w.usename, b.pid, b.usename, lw.locktype, lw.mode,
           coalesce(extract(epoch FROM now() - w.query_start)::int::text, '0'),
           translate(left(w.query, 100), E'\n\r|', '   '),
           translate(left(b.query, 100), E'\n\r|', '   ')
    FROM sys_locks lw
    JOIN sys_stat_activity w ON lw.pid = w.pid
    JOIN sys_locks lb ON lb.locktype = lw.locktype
      AND lb.database IS NOT DISTINCT FROM lw.database
      AND lb.relation IS NOT DISTINCT FROM lw.relation
      AND lb.page IS NOT DISTINCT FROM lw.page
      AND lb.tuple IS NOT DISTINCT FROM lw.tuple
      AND lb.virtualxid IS NOT DISTINCT FROM lw.virtualxid
      AND lb.transactionid IS NOT DISTINCT FROM lw.transactionid
      AND lb.classid IS NOT DISTINCT FROM lw.classid
      AND lb.objid IS NOT DISTINCT FROM lw.objid
      AND lb.objsubid IS NOT DISTINCT FROM lw.objsubid
      AND lb.pid <> lw.pid AND lb.granted
    JOIN sys_stat_activity b ON lb.pid = b.pid
    WHERE NOT lw.granted
    ORDER BY 7 DESC
    LIMIT ${TOP_N};")

  local cnt max_wait over_cnt
  cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$rows")
  read -r max_wait over_cnt < <(awk -F'|' -v t="$warn_wait" '
    NF { if ($7+0 > m) m = $7+0; if ($7+0 > t) c++ }
    END { print m+0, c+0 }' <<< "$rows")
  [[ "$cnt" -gt 0 ]] && _exit=1

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_begin "locks_wait"
    if [[ "$cnt" -gt 0 ]]; then
      json_item "waiting_locks" "warn" "$cnt" "longest ${max_wait}s, $over_cnt > threshold ${warn_wait}s"
    else
      json_item "waiting_locks" "ok" "0" ""
    fi
    while IFS='|' read -r wpid wuser bpid buser ltype mode wsec wquery bquery; do
      [[ -z "${wpid// /}" ]] && continue
      json_row "wait_pid=${wpid// /}" "wait_user=$wuser" "block_pid=${bpid// /}" \
        "block_user=$buser" "locktype=$ltype" "mode=$mode" \
        "wait_seconds=${wsec// /}" "wait_query=$wquery" "block_query=$bquery"
    done <<< "$rows"
    json_end
    [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
    return 0
  fi

  if [[ "$cnt" -eq 0 ]]; then
    ok "Waiting locks: none"
  else
    warn "Waiting locks: $cnt blocked session(s) (longest ${max_wait}s, showing top ${TOP_N})"
    if [[ "$over_cnt" -gt 0 ]]; then
      warn "Long lock waits: $over_cnt session(s) > ${warn_wait}s"
    else
      ok "Long lock waits: none > ${warn_wait}s"
    fi
  fi

  if [[ -z "$QUIET" && "$cnt" -gt 0 ]]; then
    echo
    if [[ -n "$VERBOSE" ]]; then
      { echo "WAIT_PID|WAIT_USER|BLOCK_PID|BLOCK_USER|LOCKTYPE|MODE|WAIT|WAIT_QUERY|BLOCK_QUERY"
        awk -F'|' "$AWK_FMT_DUR"'NF {
          print $1"|"$2"|"$3"|"$4"|"$5"|"$6"|"fmt($7)"|"$8"|"$9
        }' <<< "$rows"
      } | column -t -s '|' || true
    else
      { echo "WAIT_PID|WAIT_USER|BLOCK_PID|BLOCK_USER|MODE|WAIT|WAIT_QUERY"
        awk -F'|' "$AWK_FMT_DUR"'NF {
          print $1"|"$2"|"$3"|"$4"|"$6"|"fmt($7)"|"substr($8, 1, 50)
        }' <<< "$rows"
      } | column -t -s '|' || true
    fi
  fi

  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}

_locks_hold() {
  hdr "Lock-holding sessions"

  local rows cnt
  rows=$(ksql_q "
    SELECT l.pid, a.usename, l.locktype, l.mode,
           CASE WHEN l.relation IS NULL THEN '-' ELSE l.relation::regclass::text END,
           coalesce(extract(epoch FROM now() - a.xact_start)::int::text, '0'),
           translate(left(a.query, 100), E'\n\r|', '   ')
    FROM sys_locks l
    JOIN sys_stat_activity a ON l.pid = a.pid
    WHERE l.granted AND l.locktype IN ('relation', 'tuple', 'transactionid')
      AND a.pid <> sys_backend_pid()
    ORDER BY 6 DESC
    LIMIT ${TOP_N};")
  cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$rows")

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_begin "locks_hold"
    json_item "lock_holding" "ok" "$cnt" "top ${TOP_N} by transaction age"
    while IFS='|' read -r pid user ltype mode rel xact_s query; do
      [[ -z "${pid// /}" ]] && continue
      json_row "pid=${pid// /}" "user=$user" "locktype=$ltype" "mode=$mode" \
        "relation=${rel// /}" "xact_seconds=${xact_s// /}" "query=$query"
    done <<< "$rows"
    json_end
    return 0
  fi

  if [[ "$cnt" -eq 0 ]]; then
    ok "Lock-holding sessions: none"
  else
    info "Lock-holding sessions: $cnt lock(s) shown (top ${TOP_N} by transaction age)"
  fi

  if [[ -z "$QUIET" && "$cnt" -gt 0 ]]; then
    echo
    if [[ -n "$VERBOSE" ]]; then
      { echo "PID|USER|LOCKTYPE|MODE|RELATION|XACT_AGE|QUERY"
        awk -F'|' "$AWK_FMT_DUR"'NF {
          print $1"|"$2"|"$3"|"$4"|"$5"|"fmt($6)"|"$7
        }' <<< "$rows"
      } | column -t -s '|' || true
    else
      { echo "PID|USER|MODE|RELATION|XACT_AGE|QUERY"
        awk -F'|' "$AWK_FMT_DUR"'NF {
          print $1"|"$2"|"$4"|"$5"|"fmt($6)"|"substr($7, 1, 50)
        }' <<< "$rows"
      } | column -t -s '|' || true
    fi
  fi

  return 0
}

_locks_deadlock() {
  hdr "Deadlock statistics"
  local _exit=0

  local rows total db_cnt
  rows=$(ksql_q "
    SELECT datname, deadlocks FROM sys_stat_database
    WHERE deadlocks > 0 ORDER BY deadlocks DESC;")
  read -r db_cnt total < <(awk -F'|' '
    NF { n++; t += $2 } END { print n+0, t+0 }' <<< "$rows")
  [[ "$total" -gt 0 ]] && _exit=1

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_begin "locks_deadlock"
    if [[ "$total" -gt 0 ]]; then
      json_item "deadlocks" "warn" "$total" "across $db_cnt database(s) since stats reset"
    else
      json_item "deadlocks" "ok" "0" "since stats reset"
    fi
    while IFS='|' read -r db dl; do
      [[ -z "${db// /}" ]] && continue
      json_row "datname=${db// /}" "deadlocks=${dl// /}"
    done <<< "$rows"
    json_end
    [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
    return 0
  fi

  if [[ "$total" -eq 0 ]]; then
    ok "Deadlocks: none recorded since stats reset"
  else
    warn "Deadlocks: $total across $db_cnt database(s) (since stats reset)"
  fi

  if [[ -z "$QUIET" && "$total" -gt 0 ]]; then
    echo
    { echo "DATABASE|DEADLOCKS"
      awk -F'|' 'NF' <<< "$rows"
    } | column -t -s '|' || true
  fi

  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}

cmd_locks() {
  local subcmd="${1:-}"
  case "$subcmd" in
    hold)     _locks_hold     ;;
    deadlock) _locks_deadlock ;;
    wait|"")  _locks_wait     ;;
    *)
      echo "Unknown locks subcommand: $subcmd (wait|hold|deadlock)" >&2
      return 1
      ;;
  esac
}
