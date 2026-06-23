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
}
