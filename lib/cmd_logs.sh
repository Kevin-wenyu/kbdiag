# shellcheck shell=bash
# cmd_logs.sh — log file analysis (slow queries, errors, fatals). DBA tier:
# scans the server log for slow-query and error/fatal evidence.

# Newest server log file, or empty when none found. Shared by logs and snapshot.
_logs_latest_file() {
  local log_dir
  log_dir=$(ksql_q "SELECT setting FROM sys_settings WHERE name='log_directory';" 2>/dev/null | tr -d '[:space:]') || log_dir=""
  [[ -z "$log_dir" ]] && log_dir="log"
  [[ "${log_dir}" != /* ]] && log_dir="$KB_DATA_DIR/$log_dir"
  # shellcheck disable=SC2012
  ls -t "${log_dir}/"*.log 2>/dev/null | head -1 || true
}

cmd_logs() {
  local log_file="" since=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --file)    log_file="$2"; shift ;;
      --file=*)  log_file="${1#--file=}" ;;
      --since)   since="$2"; shift ;;
      --since=*) since="${1#--since=}" ;;
      --level)   shift ;;
      --level=*) ;;
      "")        ;; # skip empty args from dispatch
    esac
    shift
  done

  # Auto-detect log path if not specified
  [[ -z "$log_file" ]] && log_file=$(_logs_latest_file)

  local _exit=0
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "logs"

  # Missing/unreadable log file is a gated data-availability WARN (logging
  # may simply not be configured yet), not a hard usage error.
  if [[ -z "$log_file" || ! -r "$log_file" ]]; then
    warn "Log file not found or not readable. Set KB_LOG_FILE or use --file."
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_item "log_file" "warn" "${log_file:-}" "not found or not readable"
      json_end
    fi
    [[ -n "$EXIT_CODE_MODE" ]] && return 1
    return 0
  fi

  hdr "Log analysis: $log_file"

  # --since filter: use awk to filter by timestamp
  local target_file="$log_file"
  local tmp_file=""
  if [[ -n "$since" ]]; then
    local mins=0
    [[ "$since" =~ ([0-9]+)h ]] && mins=$(( mins + BASH_REMATCH[1] * 60 ))
    [[ "$since" =~ ([0-9]+)m ]] && mins=$(( mins + BASH_REMATCH[1] ))
    local cutoff
    cutoff=$(date -d "-${mins} minutes" '+%Y-%m-%d %H:%M:%S' 2>/dev/null || \
             date -v "-${mins}M" '+%Y-%m-%d %H:%M:%S' 2>/dev/null || true)
    if [[ -n "$cutoff" ]]; then
      tmp_file=$(mktemp "${TMPDIR:-/tmp}/kbdiag_logs.XXXXXX")
      awk -v cut="$cutoff" '$0 >= cut' "$log_file" > "$tmp_file" || true
      target_file="$tmp_file"
    fi
  fi

  # 1. Slow queries — one grep+awk pass; verdict + Top N table rendered from
  # the same captured output (D6).
  local slow_out slow_cnt=0 slow_warn="${KB_SLOW_THRESHOLD}"
  slow_out=$(grep -E "duration: [0-9]+\.[0-9]+ ms" "$target_file" 2>/dev/null | \
    awk -F'duration: ' -v thresh="${slow_warn}" \
      '{split($2,a," "); ms=a[1]+0; if(ms > thresh*1000) print ms" ms: "$0}' | \
    sort -rn || true)
  [[ -n "$slow_out" ]] && slow_cnt=$(echo "$slow_out" | wc -l | tr -d '[:space:]')
  if [[ "$slow_cnt" -gt 0 ]]; then
    warn "Slow queries: ${slow_cnt} found (> ${slow_warn}s)"
    _exit=1
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "slow_queries" "warn" "$slow_cnt" "> ${slow_warn}s"
  else
    ok "Slow queries: none found (> ${slow_warn}s threshold)"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "slow_queries" "ok" "0" ""
  fi
  local slow_limit="$TOP_N"
  [[ -n "$VERBOSE" ]] && slow_limit="$slow_cnt"
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    if [[ "$slow_cnt" -gt 0 ]]; then
      # Process substitution, not a pipe: `cmd | while read` runs the loop in
      # a subshell, and json_row's writes to _JSON_ROWS would be lost on exit.
      while IFS= read -r line; do
        json_row check=slow_query line="$line"
      done < <(echo "$slow_out" | head -"${slow_limit:-0}")
    fi
  else
    info "Top ${slow_limit} slow queries:"
    if [[ "$slow_cnt" -gt 0 ]]; then
      echo "$slow_out" | head -"$slow_limit"
      [[ -z "$VERBOSE" && "$slow_cnt" -gt "$slow_limit" ]] && info "... $((slow_cnt - slow_limit)) more, use -v to show all"
    else
      info "(none)"
    fi
    echo ""
  fi

  # 2. ERROR/FATAL — one grep pass; verdict + by-hour + top-messages all
  # rendered from the same captured lines (D6: one scan, multiple renderings).
  local err_lines err_cnt=0
  # No leading-space assumption: log_line_prefix commonly ends right at the
  # last field (e.g. "...client=%h") with no separator before "ERROR:"/"FATAL:".
  err_lines=$(grep -E "ERROR:|FATAL:" "$target_file" 2>/dev/null || true)
  [[ -n "$err_lines" ]] && err_cnt=$(echo "$err_lines" | wc -l | tr -d '[:space:]')
  if [[ "$err_cnt" -gt 0 ]]; then
    warn "ERROR/FATAL: ${err_cnt} entries found"
    [[ "$_exit" -lt 1 ]] && _exit=1
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "errors" "warn" "$err_cnt" ""
  else
    ok "ERROR/FATAL: none found"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "errors" "ok" "0" ""
  fi

  local hour_limit=24
  [[ -n "$VERBOSE" ]] && hour_limit=999999
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    if [[ "$err_cnt" -gt 0 ]]; then
      while IFS='|' read -r hr cnt; do
        json_row check=error_hour hour="$hr" count="$cnt"
      done < <(echo "$err_lines" | awk '{match($0,/[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}/,a); h[a[0]]++} END {for(k in h) print k"|"h[k]}' | sort)
    fi
  else
    info "ERROR/FATAL by hour:"
    if [[ "$err_cnt" -gt 0 ]]; then
      echo "$err_lines" | awk '{match($0,/[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}/,a); h[a[0]]++} END {for(k in h) print k,h[k]}' | \
        sort | tail -"$hour_limit"
    else
      info "(none)"
    fi
    echo ""
  fi

  local msg_limit="$TOP_N"
  [[ -n "$VERBOSE" ]] && msg_limit=999999
  if [[ "$OUTPUT_FMT" == "json" ]]; then
    if [[ "$err_cnt" -gt 0 ]]; then
      while read -r cnt msg; do
        json_row check=error_message count="$cnt" message="$msg"
      done < <(echo "$err_lines" | sed 's/.*ERROR: //; s/.*FATAL: //' | sort | uniq -c | sort -rn | head -"$msg_limit")
    fi
  else
    info "Top ${msg_limit} error messages:"
    if [[ "$err_cnt" -gt 0 ]]; then
      echo "$err_lines" | sed 's/.*ERROR: //; s/.*FATAL: //' | sort | uniq -c | sort -rn | head -"$msg_limit"
    else
      info "(none)"
    fi
  fi

  # Cleanup temp file
  if [[ -n "$tmp_file" && -f "$tmp_file" ]]; then rm -f "$tmp_file"; fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}
