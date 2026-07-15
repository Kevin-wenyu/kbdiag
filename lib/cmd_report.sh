# shellcheck shell=bash
# report — inspection report for handover/delivery: verdict-first Markdown
# file assembled entirely from existing commands' output. Zero new judgment
# logic lives here; every OK/WARN/FAIL comes from the underlying command.

# Run one section and fold its output into the report.
# $1=section title, $2...=command. Uses _RPT_* globals.
_report_section() {
  local title="$1"; shift
  local out c_ok c_warn c_fail line level msg
  # sections must render as plain text regardless of the report's own
  # --format/--quiet flags — the counters parse [OK]/[WARN]/[FAIL] markers
  out=$(OUTPUT_FMT="" QUIET="" "$@" 2>&1) || true

  c_ok=$(echo "$out" | grep -c '^\[OK\]') || true
  c_warn=$(echo "$out" | grep -c '^\[WARN\]') || true
  c_fail=$(echo "$out" | grep -c '^\[FAIL\]') || true
  _RPT_OK=$((_RPT_OK + c_ok))
  _RPT_WARN=$((_RPT_WARN + c_warn))
  _RPT_FAIL=$((_RPT_FAIL + c_fail))

  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    level=$(echo "$line" | sed -E 's/^\[(WARN|FAIL)\].*/\1/')
    msg=$(echo "$line" | sed -E 's/^\[(WARN|FAIL)\][[:space:]]*//' | tr '|' '/')
    _RPT_SUMMARY+="| $title | $level | $msg |"$'\n'
  done < <(echo "$out" | grep -E '^\[(WARN|FAIL)\]' || true)

  _RPT_BODY+="## $title"$'\n\n''```text'$'\n'"$out"$'\n''```'$'\n\n'

  local status="ok"
  [[ $c_warn -gt 0 ]] && status="warn"
  [[ $c_fail -gt 0 ]] && status="fail"
  local slug
  slug=$(echo "$title" | tr '[:upper:] ' '[:lower:]_' | tr -cd 'a-z0-9_')
  json_item "$slug" "$status" "ok=$c_ok warn=$c_warn fail=$c_fail" ""
  info "  section done: $title (ok=$c_ok warn=$c_warn fail=$c_fail)"
}

cmd_report() {
  local outfile="${1:-}"
  local host
  host=$(hostname -s 2>/dev/null || hostname)
  [[ -z "$outfile" ]] && outfile="kbdiag-report-${host}-$(date +%Y%m%d-%H%M%S).md"

  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "report"
  hdr "Inspection report"

  # the report is a Markdown file — ANSI codes must not leak into it
  # shellcheck disable=SC2034  # consumed by _init_colors in core.sh
  NO_COLOR=1; _init_colors

  local db_version role="primary" generated
  generated=$(date '+%Y-%m-%d %H:%M:%S')
  db_version=$(ksql_q "SELECT split_part(version(), ',', 1);" | head -1)
  _is_true "$(ksql_q 'SELECT pg_is_in_recovery()::text;')" && role="standby"

  _RPT_BODY="" _RPT_SUMMARY="" _RPT_OK=0 _RPT_WARN=0 _RPT_FAIL=0

  _report_section "Instance status"     cmd_status
  _report_section "License"             cmd_license
  _report_section "Health check"        cmd_check
  _report_section "Failover readiness"  cmd_cluster ready
  _report_section "Replication"         cmd_replication
  _report_section "Capacity"            cmd_space
  _report_section "Backup & archiving"  cmd_backup
  _report_section "Throughput"          cmd_stat
  _report_section "Index health"        cmd_idx
  _report_section "Security audit"      cmd_audit
  _report_section "Recommendations"     cmd_advisor

  local verdict="OK" _exit=0
  if [[ $_RPT_FAIL -gt 0 ]]; then verdict="FAIL"; _exit=2
  elif [[ $_RPT_WARN -gt 0 ]]; then verdict="WARN"; _exit=1
  fi
  [[ -z "$_RPT_SUMMARY" ]] && _RPT_SUMMARY="| — | — | no warnings or failures |"$'\n'

  {
    echo "# KingbaseES Inspection Report — $host"
    echo
    echo "| | |"
    echo "|---|---|"
    echo "| Host | $host ($role) |"
    echo "| Database | ${db_version:-unreachable} |"
    echo "| Generated | $generated |"
    echo "| Tool | kbdiag ${KBDIAG_VERSION:-dev} |"
    echo
    echo "## Executive summary"
    echo
    echo "**Overall: $verdict** — ${_RPT_OK} OK / ${_RPT_WARN} WARN / ${_RPT_FAIL} FAIL"
    echo
    echo "| Section | Level | Finding |"
    echo "|---------|-------|---------|"
    printf '%s' "$_RPT_SUMMARY"
    echo
    printf '%s' "$_RPT_BODY"
    echo "---"
    echo "Thresholds are tunable via KB_WARN_*/KB_FAIL_* environment variables."
    echo "Cross-check any finding with the matching kbdiag command (see \`kbdiag --help\`)."
  } > "$outfile"

  json_item "report_file" "ok" "$outfile" "verdict: $verdict"
  if [[ $_exit -eq 2 ]]; then
    fail "Report: $verdict (${_RPT_WARN} WARN, ${_RPT_FAIL} FAIL) -> $outfile"
  elif [[ $_exit -eq 1 ]]; then
    warn "Report: $verdict (${_RPT_WARN} WARN) -> $outfile"
  else
    ok "Report: $verdict -> $outfile"
  fi
  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  return "$_exit"
}
