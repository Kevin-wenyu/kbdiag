# shellcheck shell=bash
# cmd_partition.sh — partition table health (list/range/hash partitioning)
#
# Answers: are partitioned tables structurally sound — does every table have
# somewhere for new rows to land, and is data spread evenly across siblings?

_partition_strategy_name() {
  case "$1" in
    l) echo "list" ;;
    r) echo "range" ;;
    h) echo "hash" ;;
    *) echo "$1" ;;
  esac
}

_partition_fmt() {
  awk -v b="${1:-0}" 'BEGIN {
    if (b >= 1073741824) printf "%.1fGB", b/1073741824
    else if (b >= 1048576) printf "%.1fMB", b/1048576
    else if (b >= 1024) printf "%.1fKB", b/1024
    else printf "%dB", b
  }'
}

_partition_list() {
  ksql_q "
    SELECT c.oid||'|'||n.nspname||'.'||c.relname||'|'||p.partstrat||'|'
           ||(SELECT count(*) FROM sys_inherits i WHERE i.inhparent=c.oid)
    FROM sys_class c
    JOIN sys_namespace n ON n.oid = c.relnamespace
    JOIN sys_partitioned_table p ON p.partrelid = c.oid
    WHERE c.relkind='p'
    ORDER BY 1;"
}

_partition_children() {
  ksql_q "
    SELECT child.relname||'|'||pg_total_relation_size(child.oid)||'|'
           ||CASE WHEN pg_get_expr(child.relpartbound, child.oid) LIKE '%DEFAULT%' THEN '1' ELSE '0' END
    FROM sys_inherits i
    JOIN sys_class child ON child.oid = i.inhrelid
    WHERE i.inhparent = $1
    ORDER BY pg_total_relation_size(child.oid) DESC;"
}

_partition_analyze_one() {
  local oid="$1" fq="$2" strat_code="$3" nchild="$4"
  local strat; strat=$(_partition_strategy_name "$strat_code")

  if [[ "$nchild" -eq 0 ]]; then
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_item "$fq" "fail" "0 partitions" "every insert will fail (no partition to hold rows, no DEFAULT)"
    else
      fail "$fq has 0 partitions — every insert will fail (no partition to hold rows, and no DEFAULT)"
    fi
    _PART_EXIT=2
    return
  fi

  local rows total=0 max_size=0 default_size=0 has_default=0 n=0
  rows=$(_partition_children "$oid")
  local part bytes is_default
  while IFS='|' read -r part bytes is_default; do
    [[ -z "$part" ]] && continue
    bytes="${bytes:-0}"
    total=$(( total + bytes ))
    if (( bytes > max_size )); then max_size=$bytes; fi
    if [[ "$is_default" == "1" ]]; then
      has_default=1
      default_size=$bytes
    fi
    n=$(( n + 1 ))
  done <<< "$rows"

  # verdict computations shared by text and json renderings
  local no_default="" default_warn="" skew_warn=""
  if [[ "$has_default" -eq 0 ]]; then
    no_default=1
  elif [[ "$total" -gt 10485760 ]]; then
    local pct=$(( default_size * 100 / total ))
    if [[ "$pct" -ge 20 ]]; then
      default_warn="DEFAULT partition holds ${pct}% of table data ($(_partition_fmt "$default_size"))"
    fi
  fi
  if [[ "$n" -gt 1 && "$total" -gt 10485760 ]]; then
    local avg=$(( total / n ))
    if [[ "$avg" -gt 0 && "$max_size" -gt $(( avg * 5 )) ]]; then
      skew_warn="largest partition is $(( max_size / avg ))x the average ($(_partition_fmt "$max_size") vs avg $(_partition_fmt "$avg"))"
    fi
  fi
  if [[ -n "$default_warn" || -n "$skew_warn" ]]; then
    [[ "$_PART_EXIT" -lt 1 ]] && _PART_EXIT=1
  fi

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    local st="ok"
    [[ -n "$default_warn" || -n "$skew_warn" ]] && st="warn"
    json_item "$fq" "$st" "$strat, $nchild partitions, $(_partition_fmt "$total")" \
      "${default_warn}${default_warn:+; }${skew_warn}"
    while IFS='|' read -r part bytes is_default; do
      [[ -z "$part" ]] && continue
      json_row "table=$fq" "partition=$part" "bytes=${bytes:-0}" "is_default=${is_default:-0}"
    done <<< "$rows"
    return
  fi

  ok "$fq ($strat, $nchild partitions, $(_partition_fmt "$total"))"
  [[ -n "$no_default" ]] && \
    info "  no DEFAULT partition — unmatched values are rejected at insert time (may be intentional)"
  [[ -n "$default_warn" ]] && \
    warn "  $default_warn — likely missing explicit partition definitions for new values"
  [[ -n "$skew_warn" ]] && \
    warn "  partition size skew — $skew_warn"

  if [[ -n "$VERBOSE" && -z "$QUIET" && "$n" -gt 0 ]]; then
    { echo "  PARTITION|SIZE|DEFAULT"
      while IFS='|' read -r part bytes is_default; do
        [[ -z "$part" ]] && continue
        echo "  $part|$(_partition_fmt "${bytes:-0}")|$([[ "$is_default" == "1" ]] && echo yes || echo -)"
      done <<< "$rows"
    } | column -t -s '|' || true
  fi
}

cmd_partition() {
  hdr "Partition Health"
  _PART_EXIT=0
  local rows cnt
  rows=$(_partition_list)
  cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$rows")

  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "partition"

  if [[ "$cnt" -eq 0 ]]; then
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_item "partitioned_tables" "ok" "0" "none found"
      json_end
    else
      info "Partitioned tables: none found"
    fi
    return 0
  fi

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_item "partitioned_tables" "ok" "$cnt" "checked"
  else
    info "Partitioned tables: $cnt checked"
  fi

  local oid fq strat nchild
  while IFS='|' read -r oid fq strat nchild; do
    [[ -z "$oid" ]] && continue
    _partition_analyze_one "$oid" "$fq" "$strat" "$nchild"
  done <<< "$rows"

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_PART_EXIT"
  return 0
}
