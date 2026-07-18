# shellcheck shell=bash

# Shared with cmd_perf.sh's _perf_wait and cmd_stat.sh's _stat_section_wait_events —
# edit only here to change what counts as "currently waiting".
# Activity-type events are background processes' normal idle loops (WalSenderMain,
# BgWriterMain, ...) and Client-type events mean waiting on the client, not the DB
# (idle-in-txn is sessions' concern) — both excluded per ecosystem convention
_WAIT_EVENT_WHERE="wait_event IS NOT NULL AND state <> 'idle' AND wait_event_type NOT IN ('Activity', 'Client')"

cmd_wait() {
  hdr "Wait event analysis"
  local _exit=0

  # one query captures all waiting sessions; verdicts, the default distribution
  # and the -v per-session table all derive from this same captured result
  local rows
  rows=$(ksql_q "
    SELECT coalesce(wait_event_type, ''), coalesce(wait_event, ''), pid,
           coalesce(extract(epoch FROM now() - query_start)::int::text, '0'),
           translate(left(query, 100), E'\n\r|', '   ')
    FROM sys_stat_activity
    WHERE $_WAIT_EVENT_WHERE
    ORDER BY wait_event_type, wait_event, pid;")

  local cnt lock_cnt max_wait
  cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$rows")
  read -r lock_cnt max_wait < <(awk -F'|' '
    NF { if ($4+0 > m) m = $4+0 }
    $1 ~ /Lock/ { c++ }
    END { print c+0, m+0 }' <<< "$rows")
  [[ "$lock_cnt" -gt 0 ]] && _exit=1

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_begin "wait"
    if [[ "$lock_cnt" -gt 0 ]]; then
      json_item "lock_waits" "warn" "$lock_cnt" "longest ${max_wait}s"
    else
      json_item "lock_waits" "ok" "0" ""
    fi
    json_item "waiting_sessions" "ok" "$cnt" "longest ${max_wait}s"
    while IFS='|' read -r wtype wevent pid wait_s query; do
      [[ -z "${pid// /}" ]] && continue
      json_row "wait_event_type=$wtype" "wait_event=$wevent" "pid=${pid// /}" \
        "wait_seconds=${wait_s// /}" "query=$query"
    done <<< "$rows"
    json_end
    [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
    return 0
  fi

  if [[ "$lock_cnt" -gt 0 ]]; then
    warn "Lock waits: $lock_cnt session(s) waiting on locks (longest ${max_wait}s)"
  else
    ok "Lock waits: none"
  fi
  if [[ "$cnt" -gt 0 ]]; then
    info "Waiting sessions: $cnt (longest wait ${max_wait}s)"
  else
    ok "Waiting sessions: none"
  fi

  if [[ -z "$QUIET" && "$cnt" -gt 0 ]]; then
    echo
    if [[ -n "$VERBOSE" ]]; then
      { echo "TYPE|EVENT|PID|WAIT_S|QUERY"
        awk -F'|' 'NF' <<< "$rows"
      } | column -t -s '|' || true
    else
      { echo "TYPE|EVENT|SESSIONS|AVG_WAIT_S"
        awk -F'|' 'NF {
          k = $1 "|" $2; c[k]++; s[k] += $4 + 0
        } END {
          for (k in c) printf "%s|%d|%.1f\n", k, c[k], s[k] / c[k]
        }' <<< "$rows" | sort -t '|' -k3,3nr | head -n "$TOP_N"
      } | column -t -s '|' || true
    fi
  fi

  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}
