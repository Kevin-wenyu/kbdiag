# cmd_logs.sh — log file analysis (slow queries, errors, fatals)

cmd_logs() {
  local log_file="" since="" level="all"
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --file)    log_file="$2"; shift ;;
      --file=*)  log_file="${1#--file=}" ;;
      --since)   since="$2"; shift ;;
      --since=*) since="${1#--since=}" ;;
      --level)   level="$2"; shift ;;
      --level=*) level="${1#--level=}" ;;
      "")        ;; # skip empty args from dispatch
    esac
    shift
  done

  # Auto-detect log path if not specified
  if [[ -z "$log_file" ]]; then
    local log_dir log_fn
    log_dir=$(ksql_q "SELECT setting FROM sys_settings WHERE name='log_directory';" 2>/dev/null | tr -d '[:space:]') || log_dir=""
    if [[ -z "$log_dir" ]]; then
      log_dir="log"
    fi
    [[ "${log_dir}" != /* ]] && log_dir="$KB_DATA_DIR/$log_dir"
    log_file=$(ls -t "${log_dir}/"*.log 2>/dev/null | head -1 || true)
  fi

  if [[ -z "$log_file" || ! -r "$log_file" ]]; then
    warn "Log file not found or not readable. Set KB_LOG_FILE or use --file."
    return 0
  fi

  hdr "Log analysis: $log_file"

  # --since filter: use awk to filter by timestamp
  local target_file="$log_file"
  local tmp_file=""
  if [[ -n "$since" ]]; then
    local mins=0
    [[ "$since" =~ ([0-9]+)h ]] && mins=$(( mins + ${BASH_REMATCH[1]} * 60 ))
    [[ "$since" =~ ([0-9]+)m ]] && mins=$(( mins + ${BASH_REMATCH[1]} ))
    local cutoff
    cutoff=$(date -d "-${mins} minutes" '+%Y-%m-%d %H:%M:%S' 2>/dev/null || \
             date -v "-${mins}M" '+%Y-%m-%d %H:%M:%S' 2>/dev/null || true)
    if [[ -n "$cutoff" ]]; then
      tmp_file=$(mktemp /tmp/kbdiag_logs.XXXXXX)
      awk -v cut="$cutoff" '$0 >= cut' "$log_file" > "$tmp_file" || true
      target_file="$tmp_file"
    fi
  fi

  # Slow queries Top N
  info "Slow queries (> ${KB_SLOW_THRESHOLD}s) Top $TOP_N:"
  local slow_out
  slow_out=$(grep -E "duration: [0-9]+\.[0-9]+ ms" "$target_file" 2>/dev/null | \
    awk -F'duration: ' -v thresh="${KB_SLOW_THRESHOLD}" \
      '{split($2,a," "); ms=a[1]+0; if(ms > thresh*1000) print ms" ms: "$0}' | \
    sort -rn | head -"$TOP_N" || true)
  if [[ -n "$slow_out" ]]; then
    echo "$slow_out"
  else
    info "No slow queries found"
  fi

  # ERROR/FATAL count by hour
  info "ERROR/FATAL by hour:"
  grep -E " ERROR:| FATAL:" "$target_file" 2>/dev/null | \
    awk '{match($0,/[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}/,a); h[a[0]]++} \
         END {for(k in h) print k,h[k]}' | \
    sort | tail -24 || true

  # Most frequent error messages
  info "Top $TOP_N error messages:"
  grep -E " ERROR:| FATAL:" "$target_file" 2>/dev/null | \
    sed 's/.*\(ERROR\|FATAL\): //' | sort | uniq -c | sort -rn | head -"$TOP_N" || true

  # Cleanup temp file
  [[ -n "$tmp_file" && -f "$tmp_file" ]] && rm -f "$tmp_file" || true
}
