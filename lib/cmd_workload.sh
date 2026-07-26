# shellcheck shell=bash
# cmd_workload.sh — interval workload-diff report (sys_kwr / sys_stat_sysmetric_history).
# DBA tier: contrasts a time window against the instance's own snapshot/metric
# history, unlike perf (point-in-time snapshot).

# Reuses cmd_logs.sh's relative-duration grammar (Nh/Nm, "N ago") — returns
# minutes, or empty on unparseable input.
_workload_parse_duration() {
  local dur="$1" mins=0 matched=0
  if [[ "$dur" =~ ([0-9]+)h ]]; then mins=$(( mins + BASH_REMATCH[1] * 60 )); matched=1; fi
  if [[ "$dur" =~ ([0-9]+)m ]]; then mins=$(( mins + BASH_REMATCH[1] )); matched=1; fi
  [[ "$matched" -eq 1 ]] && echo "$mins"
}

# sys_kwr report for a window. $1/$2 = minutes-ago for the window start/end
# (empty $1 = "no lower bound", i.e. earliest snapshot; empty $2 = "now").
# Picks the earliest snapshot at/after the window start and the latest
# snapshot at/before the window end — the standard AWR two-boundary diff.
# Sets _WORKLOAD_REPORT.
_workload_kwr_report_window() {
  local from_mins="$1" to_mins="$2"
  local start_id end_id
  if [[ -n "$from_mins" ]]; then
    start_id=$(ksql_q "SELECT snap_id FROM perf.kwr_snapshots WHERE snap_time >= now() - interval '${from_mins} minutes' ORDER BY snap_time ASC LIMIT 1;")
  else
    start_id=$(ksql_q "SELECT snap_id FROM perf.kwr_snapshots ORDER BY snap_time ASC LIMIT 1;")
  fi
  end_id=$(ksql_q "SELECT snap_id FROM perf.kwr_snapshots WHERE snap_time <= now() - interval '${to_mins:-0} minutes' ORDER BY snap_time DESC LIMIT 1;")
  [[ -z "$start_id" || -z "$end_id" || "$start_id" == "$end_id" ]] && return 1
  _WORKLOAD_REPORT=$(ksql_q "SELECT perf.kwr_report(${start_id}, ${end_id}, 'text');")
}

# Default (no --from/--to): most recent two snapshots. Sets _WORKLOAD_REPORT.
_workload_kwr_report_latest() {
  local snap_ids
  snap_ids=$(ksql_q "SELECT snap_id FROM perf.kwr_snapshots ORDER BY snap_id DESC LIMIT 2;")
  local end_id start_id
  end_id=$(echo "$snap_ids" | sed -n '1p')
  start_id=$(echo "$snap_ids" | sed -n '2p')
  [[ -z "$start_id" || -z "$end_id" ]] && return 1
  _WORKLOAD_REPORT=$(ksql_q "SELECT perf.kwr_report(${start_id}, ${end_id}, 'text');")
}

cmd_workload() {
  local from="" to="" no_snapshot=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --from)        from="$2"; shift ;;
      --from=*)      from="${1#--from=}" ;;
      --to)          to="$2"; shift ;;
      --to=*)        to="${1#--to=}" ;;
      --no-snapshot) no_snapshot=1 ;;
      "")            ;; # skip empty args from dispatch
    esac
    shift
  done

  hdr "Workload"

  # Hard usage errors (malformed args) are unconditional, non-zero regardless
  # of --exit-code — mirrors cmd_explain.sh's missing-required-arg precedent,
  # but FAIL(2) not WARN(1): these are contradictory flags, not just absent ones.
  if [[ -n "$to" && -z "$from" ]]; then
    warn "Usage: --to requires --from"
    return 2
  fi

  local from_mins="" to_mins=""
  [[ -n "$from" ]] && from_mins=$(_workload_parse_duration "$from")
  [[ -n "$to" ]] && to_mins=$(_workload_parse_duration "$to")

  if [[ -n "$from_mins" && -n "$to_mins" && "$from_mins" -lt "$to_mins" ]]; then
    warn "Usage: --from must be earlier than --to"
    return 2
  fi

  local _exit=0
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "workload"

  local has_kwr
  has_kwr=$(ksql_q "SELECT extname FROM sys_extension WHERE extname='sys_kwr';" | tr -d '[:space:]')

  if [[ "$has_kwr" == "sys_kwr" ]]; then
    local _WORKLOAD_REPORT="" got_report=1
    if [[ -n "$from" ]]; then
      _workload_kwr_report_window "$from_mins" "$to_mins" && got_report=0
    else
      _workload_kwr_report_latest && got_report=0
    fi

    if [[ "$got_report" -eq 0 && -n "$_WORKLOAD_REPORT" ]]; then
      ok "Workload report generated (sys_kwr)"
      if [[ "$OUTPUT_FMT" == "json" ]]; then
        json_item "report" "ok" "$_WORKLOAD_REPORT" "source=sys_kwr"
      else
        printf '\n%s\n' "$_WORKLOAD_REPORT"
      fi
    else
      # Insufficient snapshots in the requested window. On a writable primary
      # (and unless --no-snapshot was passed) supplement with one snapshot so
      # the *default* "latest 2" report has something to diff next time — a
      # standby can never do this (perf.create_snapshot() is a write, and
      # replicas are read-only), so it always degrades straight to the WARN.
      local is_standby
      is_standby=$(ksql_q "SELECT pg_is_in_recovery()::text;" | tr -d '[:space:]')

      if [[ "$is_standby" == "true" ]]; then
        _exit=1
        warn "当前节点是备库（只读），无法自动生成快照；如需完整历史对比，请到主库执行 kbdiag workload 触发快照，或手动到主库执行 perf.create_snapshot()"
        [[ "$OUTPUT_FMT" == "json" ]] && json_item "report" "warn" "" "standby_no_snapshot"
      elif [[ -n "$no_snapshot" ]]; then
        _exit=1
        warn "可用快照不足，未启用自动补快照（--no-snapshot），请手动执行 perf.create_snapshot() 后重试"
        [[ "$OUTPUT_FMT" == "json" ]] && json_item "report" "warn" "" "insufficient_snapshots"
      else
        ksql_q "SELECT perf.create_snapshot();" > /dev/null
        _exit=1
        if _workload_kwr_report_latest && [[ -n "$_WORKLOAD_REPORT" ]]; then
          warn "当前区间基于本次自动生成的快照，历史对比数据不足，建议之后再跑一次"
          if [[ "$OUTPUT_FMT" == "json" ]]; then
            json_item "report" "warn" "$_WORKLOAD_REPORT" "source=sys_kwr,auto_snapshot=1"
          else
            printf '\n%s\n' "$_WORKLOAD_REPORT"
          fi
        else
          warn "自动生成快照后仍无法生成报告，请检查 sys_kwr 状态"
          [[ "$OUTPUT_FMT" == "json" ]] && json_item "report" "warn" "" "auto_snapshot_failed"
        fi
      fi
    fi
  else
    # sys_kwr not installed — fall back to the lightweight rolling-metric
    # view. No discrete snapshots here, so the window is fixed at the last
    # 15 minutes regardless of --from/--to (per spec: this is a best-effort
    # fallback, not a second AWR-style diff engine).
    local metric_rows
    metric_rows=$(ksql_q "SELECT metric_name, metric_value, metric_unit, begin_time FROM sys_stat_sysmetric_history WHERE begin_time >= now() - interval '15 minutes' ORDER BY metric_name;")

    if [[ -n "$metric_rows" ]]; then
      _exit=1
      warn "sys_kwr 扩展未安装，使用 sys_stat_sysmetric_history 轻量指标回退（固定最近 15 分钟）"
      if [[ "$OUTPUT_FMT" == "json" ]]; then
        while IFS='|' read -r name value unit ts; do
          [[ -z "$name" ]] && continue
          json_row metric="$name" value="$value" unit="$unit" ts="$ts"
        done <<< "$metric_rows"
      else
        printf '\n%-40s %15s %10s %s\n' "METRIC" "VALUE" "UNIT" "TIME"
        while IFS='|' read -r name value unit ts; do
          [[ -z "$name" ]] && continue
          printf '%-40s %15s %10s %s\n' "$name" "$value" "$unit" "$ts"
        done <<< "$metric_rows"
      fi
    else
      _exit=1
      warn "sys_kwr 扩展未安装，且 sys_stat_sysmetric_history 近 15 分钟无数据；两个数据源均不可用"
      [[ "$OUTPUT_FMT" == "json" ]] && json_item "metrics" "warn" "" "no_data_source"
    fi
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}
