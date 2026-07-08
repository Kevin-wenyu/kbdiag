# shellcheck shell=bash
# cmd_jobs.sh — kdb_schedule scheduled-job health (KingbaseES 特有,
# Oracle DBMS_SCHEDULER 兼容视图).
#
# Three questions a DBA asks about jobs: what exists, what's broken,
# what failed recently.

_jobs_ext_installed() {
  ksql_q "SELECT 1 FROM sys_extension WHERE extname='kdb_schedule';" 2>/dev/null | tr -d '[:space:]'
}

_jobs_inventory() {
  local total enabled
  total=$(ksql_q "SELECT count(*) FROM kdb_schedule.dba_scheduler_jobs;" 2>/dev/null | tr -d '[:space:]') || total=""
  if [[ -z "$total" ]]; then
    warn "kdb_schedule installed but dba_scheduler_jobs is not readable"
    return 1
  fi
  if [[ "$total" -eq 0 ]]; then
    info "No scheduler jobs defined"
    return 1
  fi

  enabled=$(ksql_q "SELECT count(*) FROM kdb_schedule.dba_scheduler_jobs WHERE lower(enabled::text)='true';" 2>/dev/null | tr -d '[:space:]')
  ok "$total job(s) defined, ${enabled:-?} enabled"
  ksql_qh "SELECT owner, job_name, enabled::text, coalesce(state,'-'),
                  left(coalesce(repeat_interval,'-'), 40) AS interval
           FROM kdb_schedule.dba_scheduler_jobs
           ORDER BY owner, job_name LIMIT $TOP_N;" | column -t -s '|' || true
  return 0
}

_jobs_broken() {
  local cnt
  cnt=$(ksql_q "SELECT count(*) FROM kdb_schedule.dba_scheduler_jobs
                WHERE upper(coalesce(state,'')) IN ('BROKEN','FAILED');" 2>/dev/null | tr -d '[:space:]') || return 0
  if [[ "${cnt:-0}" -gt 0 ]]; then
    fail "$cnt job(s) in BROKEN/FAILED state:"
    ksql_qh "SELECT owner, job_name, state
             FROM kdb_schedule.dba_scheduler_jobs
             WHERE upper(coalesce(state,'')) IN ('BROKEN','FAILED')
             ORDER BY owner, job_name LIMIT $TOP_N;" | column -t -s '|' || true
  else
    ok "No jobs in BROKEN/FAILED state"
  fi
}

_jobs_recent_failures() {
  local cnt
  cnt=$(ksql_q "SELECT count(*) FROM kdb_schedule.dba_scheduler_job_run_details
                WHERE log_date > now() - interval '7 days'
                  AND upper(coalesce(status,'')) NOT IN ('SUCCEEDED','COMPLETED');" 2>/dev/null | tr -d '[:space:]') || return 0
  if [[ "${cnt:-0}" -gt 0 ]]; then
    warn "$cnt non-successful run(s) in the last 7 days:"
    ksql_qh "SELECT to_char(log_date,'MM-DD HH24:MI') AS at, job_name,
                    coalesce(status,'-'), left(coalesce(error,''),50) AS error
             FROM kdb_schedule.dba_scheduler_job_run_details
             WHERE log_date > now() - interval '7 days'
               AND upper(coalesce(status,'')) NOT IN ('SUCCEEDED','COMPLETED')
             ORDER BY log_date DESC LIMIT $TOP_N;" | column -t -s '|' || true
  else
    ok "No failed runs in the last 7 days"
  fi
}

cmd_jobs() {
  hdr "Scheduler Jobs (kdb_schedule)"
  if [[ -z "$(_jobs_ext_installed)" ]]; then
    info "kdb_schedule extension is not installed — no scheduled jobs to check"
    return 0
  fi
  if _jobs_inventory; then
    _jobs_broken
    _jobs_recent_failures
  fi
}
