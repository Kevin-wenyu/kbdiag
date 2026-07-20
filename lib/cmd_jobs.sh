# shellcheck shell=bash
# cmd_jobs.sh — kdb_schedule scheduled-job health (KingbaseES 特有,
# Oracle DBMS_SCHEDULER 兼容视图).
#
# Three questions a DBA asks about jobs: what exists, what's broken,
# what failed recently.

cmd_jobs() {
  hdr "Scheduler Jobs (kdb_schedule)"
  local _exit=0

  if [[ -z "$(ksql_q "SELECT 1 FROM sys_extension WHERE extname='kdb_schedule';" 2>/dev/null | tr -d '[:space:]')" ]]; then
    info "kdb_schedule extension is not installed — no scheduled jobs to check"
    [[ "$OUTPUT_FMT" == "json" ]] && { json_begin "jobs"; json_item "kdb_schedule" "skip" "not installed" ""; json_end; }
    return 0
  fi

  # capture all three dimensions once; verdicts and tables render from these
  local jobs_rows total enabled
  jobs_rows=$(ksql_q "
    SELECT owner, job_name, enabled::text, coalesce(state, '-'),
           left(coalesce(repeat_interval, '-'), 40)
    FROM kdb_schedule.dba_scheduler_jobs
    ORDER BY owner, job_name;" 2>/dev/null) || jobs_rows=""
  total=$(awk 'NF { c++ } END { print c+0 }' <<< "$jobs_rows")
  enabled=$(awk -F'|' '$3 ~ /true/ { c++ } END { print c+0 }' <<< "$jobs_rows")

  local broken_rows broken_cnt
  broken_rows=$(awk -F'|' 'toupper($4) ~ /BROKEN|FAILED/' <<< "$jobs_rows")
  broken_cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$broken_rows")

  local failed_rows failed_cnt
  failed_rows=$(ksql_q "
    SELECT to_char(log_date, 'MM-DD HH24:MI'), job_name,
           coalesce(status, '-'), left(coalesce(error, ''), 50)
    FROM kdb_schedule.dba_scheduler_job_run_details
    WHERE log_date > now() - interval '7 days'
      AND upper(coalesce(status, '')) NOT IN ('SUCCEEDED', 'COMPLETED')
    ORDER BY log_date DESC LIMIT ${TOP_N};" 2>/dev/null) || failed_rows=""
  failed_cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$failed_rows")

  [[ "$failed_cnt" -gt 0 ]] && _exit=1
  [[ "$broken_cnt" -gt 0 ]] && _exit=2

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_begin "jobs"
    json_item "jobs_defined" "ok" "$total" "$enabled enabled"
    if [[ "$broken_cnt" -gt 0 ]]; then
      json_item "broken_jobs" "fail" "$broken_cnt" "in BROKEN/FAILED state"
    else
      json_item "broken_jobs" "ok" "0" ""
    fi
    if [[ "$failed_cnt" -gt 0 ]]; then
      json_item "failed_runs_7d" "warn" "$failed_cnt" "non-successful runs in last 7 days"
    else
      json_item "failed_runs_7d" "ok" "0" "last 7 days"
    fi
    while IFS='|' read -r owner name en state interval; do
      [[ -z "${owner// /}" ]] && continue
      json_row "scope=job" "owner=${owner// /}" "job_name=${name// /}" \
        "enabled=${en// /}" "state=${state// /}" "repeat_interval=${interval# }"
    done <<< "$jobs_rows"
    while IFS='|' read -r at name status error; do
      [[ -z "${at// /}" ]] && continue
      json_row "scope=failed_run" "at=${at# }" "job_name=${name// /}" \
        "status=${status// /}" "error=${error# }"
    done <<< "$failed_rows"
    json_end
    [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
    return 0
  fi

  if [[ "$total" -eq 0 ]]; then
    ok "Scheduler jobs: none defined"
    [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
    return 0
  fi
  info "Scheduler jobs: $total defined, $enabled enabled"
  if [[ "$broken_cnt" -gt 0 ]]; then
    fail "Broken jobs: $broken_cnt in BROKEN/FAILED state"
  else
    ok "Broken jobs: none (checked $total)"
  fi
  if [[ "$failed_cnt" -gt 0 ]]; then
    warn "Failed runs: $failed_cnt non-successful in last 7 days (showing top ${TOP_N})"
  else
    ok "Failed runs: none in last 7 days"
  fi

  if [[ -z "$QUIET" ]]; then
    echo
    { echo "OWNER|JOB|ENABLED|STATE|INTERVAL"
      if [[ -n "$VERBOSE" ]]; then
        awk -F'|' 'NF' <<< "$jobs_rows"
      else
        awk -F'|' 'NF' <<< "$jobs_rows" | head -n "$TOP_N"
      fi
    } | column -t -s '|' || true
    if [[ "$failed_cnt" -gt 0 ]]; then
      echo
      info "Recent non-successful runs:"
      { echo "AT|JOB|STATUS|ERROR"
        awk -F'|' 'NF' <<< "$failed_rows"
      } | column -t -s '|' || true
    fi
  fi

  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}
