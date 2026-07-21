# shellcheck shell=bash

_sql_all() {
  local rows cnt
  rows=$(ksql_q "
    SELECT pid, usename, state,
           coalesce(extract(epoch FROM now() - query_start)::int::text, ''),
           coalesce(wait_event_type, ''), coalesce(wait_event, ''),
           translate(query, E'\n\r|', '   ')
    FROM sys_stat_activity
    WHERE state <> 'idle' AND pid <> sys_backend_pid()
      AND backend_type = 'client backend'
    ORDER BY query_start;" 2>/dev/null) || rows=""
  if [[ -z "$rows" ]]; then cnt=0; else cnt=$(echo "$rows" | grep -c '.'); fi

  if [[ "$cnt" -eq 0 ]]; then
    ok "No active sessions"
  else
    ok "${cnt} active session(s)"
  fi

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_item "active_sessions" "ok" "${cnt}" ""
    while IFS='|' read -r pid user state dur wtype wevent query; do
      [[ -z "${pid// /}" ]] && continue
      json_row pid="${pid// /}" user="${user// /}" state="${state// /}" duration_seconds="${dur// /}" wait_event_type="${wtype// /}" wait_event="${wevent// /}" query="$query"
    done <<< "$rows"
    return 0
  fi

  if [[ "$cnt" -gt 0 && -z "$QUIET" ]]; then
    echo
    if [[ -n "$VERBOSE" ]]; then
      { echo "PID|USER|STATE|DURATION_S|WAIT_TYPE|WAIT_EVENT|QUERY"
        echo "$rows"
      } | column -t -s '|' || true
    else
      { echo "PID|USER|STATE|DURATION_S|WAIT|QUERY"
        echo "$rows" | awk -F'|' '{
          w=$5; gsub(/ /,"",w); if (w=="") w="-"
          q=$7; if (length(q) > 60) q = substr(q,1,60) "..."
          print $1"|"$2"|"$3"|"$4"|"w"|"q
        }'
      } | column -t -s '|' || true
    fi
  fi
  return 0
}

_sql_detail() {
  local target="$1"
  local row text state wtype wevent
  row=$(ksql_q "SELECT query, state, coalesce(wait_event_type,''), coalesce(wait_event,'') FROM sys_stat_activity WHERE pid = $target;" 2>/dev/null) || row=""
  IFS='|' read -r text state wtype wevent <<< "$row"

  if [[ -z "${text// /}" ]]; then
    warn "No active session with pid=$target"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "session" "warn" "$target" "not found"
    return 1
  fi

  state="${state// /}"; wtype="${wtype// /}"; wevent="${wevent// /}"
  local wdesc="none"
  [[ -n "$wtype" ]] && wdesc="${wtype}/${wevent}"
  ok "PID $target: active (state=$state, wait=$wdesc)"

  local explain_label explain_sql explain_out
  if [[ -n "$VERBOSE" ]]; then
    explain_label="EXPLAIN (VERBOSE, COSTS)"
    explain_sql="EXPLAIN (VERBOSE, COSTS) $text"
  else
    explain_label="EXPLAIN"
    explain_sql="EXPLAIN $text"
  fi
  explain_out=$("$KSQL" -p "$KB_PORT" -U "$KB_SUPERUSER" "$KB_DB" -AXc "$explain_sql" 2>/dev/null) || explain_out=""

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_item "session" "ok" "$target" "state=$state wait=$wdesc"
    json_row pid="$target" state="$state" wait_event_type="$wtype" wait_event="$wevent" \
      query="$(echo "$text" | tr '\n' ' ')" \
      explain="$(echo "${explain_out:-EXPLAIN failed (query may have ended)}" | tr '\n' ' ')"
    return 0
  fi

  echo
  info "Full query:"
  echo "$text"
  echo
  info "Wait state: state=$state wait=$wdesc"
  echo
  info "$explain_label:"
  if [[ -n "$explain_out" ]]; then
    echo "$explain_out"
  else
    warn "EXPLAIN failed (query may have ended)"
  fi
  return 0
}

cmd_sql() {
  local target="${1:-all}"
  local _exit=0

  hdr "SQL analysis"
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "sql"

  if [[ "$target" == "all" ]]; then
    _sql_all || _exit=1
  elif [[ "$target" =~ ^[0-9]+$ ]]; then
    _sql_detail "$target" || _exit=1
  else
    echo "Invalid pid: $target. Use: sql [pid|all]" >&2
    [[ "$OUTPUT_FMT" == "json" ]] && json_end
    return 1
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}
