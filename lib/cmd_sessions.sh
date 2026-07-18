# shellcheck shell=bash
cmd_sessions() {
  hdr "Sessions"

  local warn_idle="${KB_WARN_IDLE_TXN:-300}" warn_conn="${KB_WARN_CONN:-80}"
  local _exit=0

  # one query captures all columns; verdicts and both table renderings
  # (default/-v) are computed from this same captured result
  local rows
  rows=$(ksql_q "
    SELECT pid, usename, coalesce(application_name, ''),
           coalesce(client_addr::text, 'local'), state,
           coalesce(extract(epoch FROM now() - xact_start)::int::text, ''),
           coalesce(extract(epoch FROM now() - query_start)::int::text, ''),
           coalesce(wait_event_type, ''), coalesce(wait_event, ''),
           translate(left(query, 120), E'\n\r|', '   ')
    FROM sys_stat_activity
    WHERE state <> 'idle' AND pid <> sys_backend_pid()
      AND backend_type = 'client backend'
    ORDER BY query_start;")

  local cnt idle_cnt idle_max wait_cnt
  cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$rows")
  read -r idle_cnt idle_max < <(awk -F'|' -v t="$warn_idle" '
    $5 ~ /idle in transaction/ && $6+0 > t { c++; if ($6+0 > m) m = $6+0 }
    END { print c+0, m+0 }' <<< "$rows")
  wait_cnt=$(awk -F'|' '$8 ~ /Lock/ { c++ } END { print c+0 }' <<< "$rows")

  local conn_row used max_conn pct
  conn_row=$(ksql_q "SELECT count(*), (SELECT setting FROM pg_settings WHERE name = 'max_connections') FROM sys_stat_activity;")
  IFS='|' read -r used max_conn <<< "$conn_row"
  used=${used// /}; max_conn=${max_conn// /}
  pct=$(( used * 100 / max_conn ))
  [[ "$idle_cnt" -gt 0 || "$wait_cnt" -gt 0 || "$pct" -gt "$warn_conn" ]] && _exit=1

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_begin "sessions"
    if [[ "$idle_cnt" -gt 0 ]]; then
      json_item "idle_in_txn" "warn" "$idle_cnt" "oldest ${idle_max}s > threshold ${warn_idle}s"
    else
      json_item "idle_in_txn" "ok" "0" "threshold ${warn_idle}s"
    fi
    if [[ "$wait_cnt" -gt 0 ]]; then
      json_item "lock_waits" "warn" "$wait_cnt" "sessions blocked on locks"
    else
      json_item "lock_waits" "ok" "0" ""
    fi
    if [[ "$pct" -gt "$warn_conn" ]]; then
      json_item "connections" "warn" "${used}/${max_conn}" "${pct}% > threshold ${warn_conn}%"
    else
      json_item "connections" "ok" "${used}/${max_conn}" "${pct}% of max"
    fi
    json_item "non_idle_sessions" "ok" "$cnt" ""
    # detail rows are always full-column in json, regardless of -v
    while IFS='|' read -r pid user app addr state xact_s dur wtype wevent query; do
      [[ -z "${pid// /}" ]] && continue
      json_row "pid=${pid// /}" "user=$user" "application=$app" \
        "client_addr=$addr" "state=$state" "xact_seconds=${xact_s// /}" \
        "duration=$dur" "wait_event_type=$wtype" "wait_event=$wevent" "query=$query"
    done <<< "$rows"
    json_end
    [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
    return 0
  fi

  # verdict summary — every line carries measured values and thresholds
  if [[ "$idle_cnt" -gt 0 ]]; then
    warn "Idle-in-transaction: $idle_cnt session(s) idle > ${warn_idle}s (oldest ${idle_max}s)"
  else
    ok "Idle-in-transaction: none > ${warn_idle}s"
  fi
  if [[ "$wait_cnt" -gt 0 ]]; then
    warn "Lock waits: $wait_cnt session(s) blocked on locks"
  else
    ok "Lock waits: none"
  fi
  if [[ "$pct" -gt "$warn_conn" ]]; then
    warn "Connections: $used/$max_conn (${pct}% > threshold ${warn_conn}%)"
  else
    ok "Connections: $used/$max_conn (${pct}%, threshold ${warn_conn}%)"
  fi
  info "Non-idle sessions: $cnt"

  if [[ "$OUTPUT_FMT" != "json" && -z "$QUIET" && ( "$cnt" -gt 0 || -n "$VERBOSE" ) ]]; then
    echo
    local _fmt_dur='function fmt(s) {
        if (s ~ /^[ ]*$/) return "-"
        s += 0; return sprintf("%d:%02d:%02d", s/3600, (s%3600)/60, s%60)
      }'
    if [[ -n "$VERBOSE" ]]; then
      { echo "PID|USER|APP|ADDR|STATE|XACT_S|DURATION|WAIT_TYPE|WAIT|QUERY"
        awk -F'|' "$_fmt_dur"'NF {
          print $1"|"$2"|"$3"|"$4"|"$5"|"$6"|"fmt($7)"|"$8"|"$9"|"$10
        }' <<< "$rows"
      } | column -t -s '|' || true
    else
      { echo "PID|USER|STATE|DURATION|WAIT|QUERY"
        awk -F'|' "$_fmt_dur"'NF {
          w = $9; gsub(/ /, "", w); if (w == "") w = "-"
          print $1 "|" $2 "|" $5 "|" fmt($7) "|" w "|" substr($10, 1, 60)
        }' <<< "$rows"
      } | column -t -s '|' || true
    fi
  fi

  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}
