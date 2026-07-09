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
    snap1=$(ksql_q "SELECT xact_commit, xact_rollback, blks_read, blks_hit, deadlocks FROM sys_stat_database WHERE datname=current_database();")
    sleep "$interval"
    snap2=$(ksql_q "SELECT xact_commit, xact_rollback, blks_read, blks_hit, deadlocks FROM sys_stat_database WHERE datname=current_database();")

    printf "%s\n%s\n" "$snap1" "$snap2" | awk -F'|' -v interval="$interval" '
      NR==1 { c1=$1; r1=$2; br1=$3; bh1=$4; d1=$5 }
      NR==2 { c2=$1; r2=$2; br2=$3; bh2=$4; d2=$5
        printf "TPS commits/s:   %.1f\n", (c2-c1)/interval
        printf "TPS rollbacks/s: %.1f\n", (r2-r1)/interval
        total_blk = (bh2-bh1) + (br2-br1)
        hit_pct = (total_blk > 0) ? 100.0*(bh2-bh1)/total_blk : 100
        printf "Buffer hit rate: %.1f%%\n", hit_pct
        printf "Deadlocks/s:     %.2f\n", (d2-d1)/interval
      }'

    i=$(( i + 1 ))
    if [[ $i -lt $count ]]; then sleep 0; fi
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
    WHERE $_WAIT_EVENT_WHERE
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
  bgwriter_stats \
  | while IFS='|' read -r timed req write_s sync_s bufs _; do
      printf "Checkpoints timed/req: %s / %s\n" \
        "${timed// /}" "${req// /}"
      printf "Write time (s):        %s\n" "${write_s// /}"
      printf "Sync time (s):         %s\n" "${sync_s// /}"
      printf "Buffers written:       %s\n" "${bufs// /}"
    done || true
}

_stat_section_top_sql() {
  _stmt_check_ext || return 0

  printf "\nTop SQL (by total execution time):\n"
  printf "%-8s %-10s %-10s %s\n" "calls" "total_ms" "mean_ms" "query"
  printf "%-8s %-10s %-10s %s\n" "--------" "----------" "----------" "---"
  ksql_q "SELECT calls::text,
    round(total_exec_time::numeric, 1)::text,
    round(mean_exec_time::numeric, 1)::text,
    left(replace(query, E'\n', ' '), 60)
    FROM sys_stat_statements
    WHERE $_STMT_ACTIVE_WHERE
    ORDER BY total_exec_time DESC
    LIMIT 5;" 2>/dev/null \
  | while IFS='|' read -r calls total_ms mean_ms sql; do
      printf "%-8s %-10s %-10s %s\n" \
        "${calls// /}" "${total_ms// /}" "${mean_ms// /}" "${sql// /}"
    done || true
}
