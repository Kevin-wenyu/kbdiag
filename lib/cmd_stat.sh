# shellcheck shell=bash
cmd_stat() {
  local interval=5 count=1
  local i=0

  # Parse arguments (skip empty strings from dispatch)
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --interval=*) interval="${1#--interval=}" ;;
      --interval)   shift; interval="${1:-5}" ;;
      --count=*)    count="${1#--count=}" ;;
      --count)      shift; count="${1:-1}" ;;
      "")           ;; # skip empty arg from dispatch
    esac
    shift
  done

  hdr "Instance throughput (${interval}s sample)"

  while [[ $i -lt $count ]]; do
    local snap1 snap2
    snap1=$(ksql_q "SELECT xact_commit, xact_rollback, blks_read, blks_hit, temp_bytes, deadlocks FROM sys_stat_database WHERE datname=current_database();")
    sleep "$interval"
    snap2=$(ksql_q "SELECT xact_commit, xact_rollback, blks_read, blks_hit, temp_bytes, deadlocks FROM sys_stat_database WHERE datname=current_database();")

    printf "%s\n%s\n" "$snap1" "$snap2" | awk -F'|' -v interval="$interval" '
      NR==1 { c1=$1; r1=$2; br1=$3; bh1=$4; t1=$5; d1=$6 }
      NR==2 { c2=$1; r2=$2; br2=$3; bh2=$4; t2=$5; d2=$6
        printf "TPS commits/s:   %.1f\n", (c2-c1)/interval
        printf "TPS rollbacks/s: %.1f\n", (r2-r1)/interval
        total_blk = (bh2-bh1) + (br2-br1)
        hit_pct = (total_blk > 0) ? 100.0*(bh2-bh1)/total_blk : 100
        printf "Buffer hit rate: %.1f%%\n", hit_pct
        printf "Temp bytes/s:    %.0f\n", (t2-t1)/interval
        printf "Deadlocks/s:     %.2f\n", (d2-d1)/interval
      }'

    i=$(( i + 1 ))
    [[ $i -lt $count ]] && sleep 0 || true
  done

  _stat_section_wait_events "$interval"
  _stat_section_temp_files "$interval"
  _stat_section_checkpoints
  _stat_section_top_sql
}

_stat_section_wait_events() {
  local interval="${1:-5}"
  printf "\nWait events (top 5 during %ss sample):\n" "$interval"
  printf "%-20s %-20s %s\n" "wait_event_type" "wait_event" "count"
  printf "%-20s %-20s %s\n" "--------------------" "--------------------" "-----"
  ksql_q "SELECT wait_event_type, wait_event, count(*) AS cnt
    FROM sys_stat_activity
    WHERE wait_event IS NOT NULL AND state != 'idle'
    GROUP BY wait_event_type, wait_event
    ORDER BY cnt DESC
    LIMIT 5;" 2>/dev/null \
  | while IFS='|' read -r wtype wevent cnt; do
      printf "%-20s %-20s %s\n" \
        "${wtype// /}" "${wevent// /}" "${cnt// /}"
    done || true
}

_stat_section_temp_files() {
  local interval="${1:-5}"
  printf "\nTemp files (%ss delta):\n" "$interval"
  local s1 s2
  s1=$(ksql_q "SELECT temp_files, temp_bytes FROM sys_stat_database WHERE datname=current_database();" 2>/dev/null)
  sleep "$interval"
  s2=$(ksql_q "SELECT temp_files, temp_bytes FROM sys_stat_database WHERE datname=current_database();" 2>/dev/null)
  printf "%s\n%s\n" "$s1" "$s2" | awk -F'|' -v interval="$interval" '
    NR==1 { f1=$1; b1=$2 }
    NR==2 { f2=$1; b2=$2
      printf "Temp files/s:    %.2f\n", (f2-f1)/interval
      printf "Temp bytes/s:    %.0f\n", (b2-b1)/interval
    }' || true
}

_stat_section_checkpoints() {
  printf "\nCheckpoint activity:\n"
  ksql_q "SELECT checkpoints_timed, checkpoints_req,
    round(checkpoint_write_time/1000.0, 1) AS write_s,
    round(checkpoint_sync_time/1000.0, 1)  AS sync_s,
    buffers_checkpoint
    FROM sys_stat_bgwriter;" 2>/dev/null \
  | while IFS='|' read -r timed req write_s sync_s bufs; do
      printf "Checkpoints timed/req: %s / %s\n" \
        "${timed// /}" "${req// /}"
      printf "Write time (s):        %s\n" "${write_s// /}"
      printf "Sync time (s):         %s\n" "${sync_s// /}"
      printf "Buffers written:       %s\n" "${bufs// /}"
    done || true
}

_stat_section_top_sql() {
  # Check if sys_stat_statements is loaded
  local stmt_cnt
  stmt_cnt=$(ksql_q "SELECT count(*) FROM sys_catalog.sys_class WHERE relname='sys_stat_statements';" 2>/dev/null | tr -d '[:space:]')
  if [[ "${stmt_cnt:-0}" -eq 0 ]]; then
    return 0
  fi

  printf "\nTop SQL (by total execution time):\n"
  printf "%-20s %-10s %-10s %s\n" "queryid" "calls" "total_s" "SQL"
  printf "%-20s %-10s %-10s %s\n" "--------------------" "----------" "----------" "---"
  ksql_q "SELECT queryid::text,
    calls::text,
    round((total_exec_time/1000.0)::numeric, 1)::text,
    left(replace(query, E'\n', ' '), 60)
    FROM sys_stat_statements
    WHERE calls > 0
    ORDER BY total_exec_time DESC
    LIMIT 5;" 2>/dev/null \
  | while IFS='|' read -r queryid calls total_s sql; do
      printf "%-20s %-10s %-10s %s\n" \
        "${queryid// /}" "${calls// /}" "${total_s// /}" "${sql// /}"
    done || true
}
