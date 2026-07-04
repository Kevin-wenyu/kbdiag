# shellcheck shell=bash
# cmd_advisor.sh — consolidated DBA advisor with --fix SQL generation

_advisor_get_mem_mb() {
  if [[ -f /proc/meminfo ]]; then
    local kb; kb=$(grep '^MemTotal:' /proc/meminfo | awk '{print $2}')
    echo $(( ${kb:-0} / 1024 ))
  else
    echo 0
  fi
}

_advisor_index() {
  local fix=0 collect=0
  for _arg in "$@"; do
    case "$_arg" in
      --collect) collect=1 ;;
      1) fix=1 ;;
    esac
  done
  [[ $collect -eq 0 ]] && hdr "Advisor: Index"
  local _exit=0

  # Unused indexes count + size (filter definition shared with cmd_idx.sh's _idx_unused)
  local unused_cnt unused_size
  unused_cnt=$(ksql_q "
    SELECT count(*) $_IDX_UNUSED_FROM
    WHERE $_IDX_UNUSED_WHERE;" | tr -d '[:space:]')
  unused_cnt="${unused_cnt:-0}"

  if [[ $unused_cnt -gt 0 ]]; then
    unused_size=$(ksql_q "
      SELECT pg_size_pretty(coalesce(sum(pg_relation_size(sui.indexrelid)),0))
      $_IDX_UNUSED_FROM
      WHERE $_IDX_UNUSED_WHERE;" | tr -d '[:space:]')
    if [[ $collect -eq 1 ]]; then
      _finding "WARN" "idx_unused" "未使用索引\n  症状：${unused_cnt} 个未使用索引，浪费 ${unused_size:-?}\n  建议:\n    · kbdiag advisor index --fix 生成 DROP SQL"
    else
      warn "${unused_cnt} unused index(es) (total ${unused_size:-?}) — run 'kbdiag idx unused' for detail"
      json_item "advisor_idx_unused" "warn" "$unused_cnt" "$unused_size wasted"
    fi
    _exit=1
  else
    if [[ $collect -eq 0 ]]; then
      ok "No unused non-unique indexes found"
      json_item "advisor_idx_unused" "ok" "0" ""
    fi
  fi

  # Duplicate indexes (filter definition shared with cmd_idx.sh's _idx_dup)
  local dup_cnt
  dup_cnt=$(ksql_q "
    SELECT count(*) $_IDX_DUP_FROM
    WHERE $_IDX_DUP_WHERE;" | tr -d '[:space:]')
  dup_cnt="${dup_cnt:-0}"

  if [[ $dup_cnt -gt 0 ]]; then
    if [[ $collect -eq 1 ]]; then
      _finding "WARN" "idx_dup" "重复/冗余索引\n  症状：${dup_cnt} 个重复或冗余索引\n  建议:\n    · kbdiag advisor index --fix 生成 DROP SQL"
    else
      warn "${dup_cnt} duplicate index pair(s) — run 'kbdiag idx dup' for detail"
      json_item "advisor_idx_dup" "warn" "$dup_cnt" ""
    fi
    _exit=1
  else
    if [[ $collect -eq 0 ]]; then
      ok "No duplicate indexes found"
      json_item "advisor_idx_dup" "ok" "0" ""
    fi
  fi

  # Missing FK indexes (filter definition shared with cmd_idx.sh's _idx_missing)
  local missing_cnt
  missing_cnt=$(ksql_q "
    SELECT count(DISTINCT con.oid)
    $_IDX_MISSING_FROM
    WHERE $_IDX_MISSING_WHERE;" | tr -d '[:space:]')
  missing_cnt="${missing_cnt:-0}"

  if [[ $missing_cnt -gt 0 ]]; then
    if [[ $collect -eq 1 ]]; then
      _finding "WARN" "idx_missing" "缺失FK索引\n  症状：${missing_cnt} 个大表中缺失外键索引\n  建议:\n    · kbdiag advisor index --fix 生成 CREATE SQL"
    else
      warn "${missing_cnt} unindexed FK column(s) on large table(s) — run 'kbdiag idx missing'"
      json_item "advisor_idx_missing" "warn" "$missing_cnt" ""
    fi
    _exit=1
  else
    if [[ $collect -eq 0 ]]; then
      ok "No missing FK indexes on large tables"
      json_item "advisor_idx_missing" "ok" "0" ""
    fi
  fi

  [[ $collect -eq 1 ]] && return $_exit

  # --fix: generate DROP INDEX SQL for unused indexes
  if [[ $fix -eq 1 ]]; then
    echo ""
    echo "-- Fix: Index (drop unused indexes — review before executing)"
    local drop_sql
    drop_sql=$(ksql_q "
      SELECT 'DROP INDEX CONCURRENTLY ' || sui.schemaname || '.' || sui.indexrelname || ';'
      $_IDX_UNUSED_FROM
      WHERE $_IDX_UNUSED_WHERE
      ORDER BY pg_relation_size(sui.indexrelid) DESC
      LIMIT ${TOP_N};" 2>/dev/null || true)
    if [[ -n "$drop_sql" ]]; then
      echo "$drop_sql"
    else
      echo "-- (no unused indexes to drop)"
    fi

    echo ""
    echo "-- Fix: Duplicate indexes (drop lower-scanned duplicate)"
    local dup_sql
    dup_sql=$(ksql_q "
      SELECT 'DROP INDEX CONCURRENTLY ' ||
        CASE WHEN a.idx_scan <= b.idx_scan THEN a.schemaname || '.' || a.indexrelname
             ELSE b.schemaname || '.' || b.indexrelname
        END || ';'
      $_IDX_DUP_FROM
      WHERE $_IDX_DUP_WHERE
      LIMIT ${TOP_N};" 2>/dev/null || true)
    if [[ -n "$dup_sql" ]]; then
      echo "$dup_sql"
    else
      echo "-- (no duplicate indexes to drop)"
    fi

    echo ""
    echo "-- Fix: Missing FK indexes (create index on unindexed FK column)"
    local missing_sql
    missing_sql=$(ksql_q "
      SELECT DISTINCT 'CREATE INDEX CONCURRENTLY ON ' || n.nspname || '.' || c.relname ||
        ' (' || a.attname || ');'
      $_IDX_MISSING_FROM
      WHERE $_IDX_MISSING_WHERE
      LIMIT ${TOP_N};" 2>/dev/null || true)
    if [[ -n "$missing_sql" ]]; then
      echo "$missing_sql"
    else
      echo "-- (no missing FK indexes found)"
    fi
  fi

  return $_exit
}

_advisor_vacuum() {
  local fix="${1:-0}"
  hdr "Advisor: Vacuum"
  local _exit=0

  # Tables with high dead tuple ratio (threshold shared with diagnose.sh's _diag_bloat)
  local bloat_rows
  bloat_rows=$(ksql_q "
    SELECT schemaname, relname,
           n_dead_tup,
           round(100.0*n_dead_tup/NULLIF(n_live_tup+n_dead_tup,0),1) AS dead_pct
    FROM sys_stat_user_tables
    WHERE n_live_tup + n_dead_tup > 1000
      AND n_dead_tup::float / NULLIF(n_live_tup+n_dead_tup, 0) > ${KB_WARN_DEAD_PCT}/100.0
    ORDER BY n_dead_tup DESC
    LIMIT ${TOP_N};" 2>/dev/null || true)

  local bloat_cnt; bloat_cnt=$(echo "$bloat_rows" | grep -c '.' 2>/dev/null || echo 0)
  [[ -z "$bloat_rows" ]] && bloat_cnt=0

  if [[ $bloat_cnt -gt 0 ]]; then
    while IFS='|' read -r schema rel dead_tup dead_pct; do
      warn "${schema// /}.${rel// /}: dead_pct=${dead_pct// /}%, ${dead_tup// /} dead tuples — needs VACUUM"
      json_item "advisor_vacuum_bloat" "warn" "${schema// /}.${rel// /}" "dead_pct=${dead_pct// /}%"
    done <<< "$bloat_rows"
    _exit=1
  else
    ok "No tables with >${KB_WARN_DEAD_PCT}% dead tuples"
    json_item "advisor_vacuum_bloat" "ok" "0" ""
  fi

  # Freeze risk: xid_age > 500M
  local freeze_rows
  freeze_rows=$(ksql_q "
    SELECT sut.schemaname, sut.relname,
           age(sc.relfrozenxid) AS xid_age
    FROM sys_stat_user_tables sut
    JOIN sys_class sc ON sc.relname = sut.relname
    JOIN sys_namespace ns ON ns.oid = sc.relnamespace AND ns.nspname = sut.schemaname
    WHERE age(sc.relfrozenxid) > 500000000
    ORDER BY age(sc.relfrozenxid) DESC
    LIMIT ${TOP_N};" 2>/dev/null || true)

  local freeze_cnt; freeze_cnt=$(echo "$freeze_rows" | grep -c '.' 2>/dev/null || echo 0)
  [[ -z "$freeze_rows" ]] && freeze_cnt=0

  if [[ $freeze_cnt -gt 0 ]]; then
    while IFS='|' read -r schema rel xid_age; do
      warn "${schema// /}.${rel// /}: xid_age=${xid_age// /} — freeze risk, run VACUUM FREEZE"
      json_item "advisor_vacuum_freeze" "warn" "${schema// /}.${rel// /}" "xid_age=${xid_age// /}"
    done <<< "$freeze_rows"
    _exit=1
  else
    ok "No freeze risk tables (all xid_age < 500M)"
    json_item "advisor_vacuum_freeze" "ok" "0" ""
  fi

  if [[ $fix -eq 1 ]]; then
    echo ""
    echo "-- Fix: Vacuum"
    if [[ -n "$bloat_rows" ]]; then
      while IFS='|' read -r schema rel dead_tup dead_pct; do
        echo "VACUUM (ANALYZE) ${schema// /}.${rel// /};"
      done <<< "$bloat_rows"
    fi
    if [[ -n "$freeze_rows" ]]; then
      while IFS='|' read -r schema rel xid_age; do
        echo "VACUUM FREEZE ANALYZE ${schema// /}.${rel// /};"
      done <<< "$freeze_rows"
    fi
    [[ -z "$bloat_rows" && -z "$freeze_rows" ]] && echo "-- (no vacuum needed)"
  fi

  return $_exit
}

_advisor_params() {
  local fix=0 collect=0
  for _arg in "$@"; do
    case "$_arg" in
      --collect) collect=1 ;;
      1) fix=1 ;;
    esac
  done
  [[ $collect -eq 0 ]] && hdr "Advisor: Params"
  local _exit=0
  local fix_lines=()

  local mem_mb; mem_mb=$(_advisor_get_mem_mb)

  # shared_buffers — stored in 8KB pages
  local sb_pages sb_mb
  sb_pages=$(ksql_q "SELECT setting FROM sys_settings WHERE name='shared_buffers';" | tr -d '[:space:]')
  sb_mb=$(( ${sb_pages:-0} * 8 / 1024 ))

  if [[ $mem_mb -gt 0 ]]; then
    local rec_sb_mb=$(( mem_mb * 25 / 100 ))
    local min_ok_mb=$(( mem_mb * 20 / 100 ))
    if [[ $sb_mb -lt $min_ok_mb ]]; then
      if [[ $collect -eq 1 ]]; then
        _finding "WARN" "param_shared_buffers" "shared_buffers 设置过低\n  症状：shared_buffers=${sb_mb}MB，仅占 RAM 的 $((sb_mb*100/mem_mb))%\n  建议:\n    · 调整至 ${rec_sb_mb}MB（25% of ${mem_mb}MB RAM）"
      else
        warn "shared_buffers=${sb_mb}MB — recommend ${rec_sb_mb}MB (25% of ${mem_mb}MB RAM)"
        json_item "advisor_param_shared_buffers" "warn" "${sb_mb}MB" "recommend ${rec_sb_mb}MB"
      fi
      fix_lines+=("ALTER SYSTEM SET shared_buffers = '${rec_sb_mb}MB';")
      _exit=1
    else
      if [[ $collect -eq 0 ]]; then
        ok "shared_buffers=${sb_mb}MB (${mem_mb}MB RAM) ok"
        json_item "advisor_param_shared_buffers" "ok" "${sb_mb}MB" ""
      fi
    fi
  else
    if [[ $collect -eq 0 ]]; then
      info "shared_buffers=${sb_mb}MB (cannot read RAM on this OS)"
      json_item "advisor_param_shared_buffers" "ok" "${sb_mb}MB" "RAM unknown"
    fi
  fi

  # checkpoint_completion_target
  local ckpt_target
  ckpt_target=$(ksql_q "SELECT setting FROM sys_settings WHERE name='checkpoint_completion_target';" | tr -d '[:space:]')
  if awk "BEGIN{exit !(${ckpt_target:-0} < 0.7)}" 2>/dev/null; then
    if [[ $collect -eq 1 ]]; then
      _finding "WARN" "param_checkpoint" "checkpoint_completion_target 设置过低\n  症状：checkpoint_completion_target=${ckpt_target}\n  建议:\n    · 调整至 0.9 以减少 WAL 积压"
    else
      warn "checkpoint_completion_target=${ckpt_target} — recommend 0.9"
      json_item "advisor_param_checkpoint_completion_target" "warn" "$ckpt_target" "recommend 0.9"
    fi
    fix_lines+=("ALTER SYSTEM SET checkpoint_completion_target = '0.9';")
    _exit=1
  else
    if [[ $collect -eq 0 ]]; then
      ok "checkpoint_completion_target=${ckpt_target} ok"
      json_item "advisor_param_checkpoint_completion_target" "ok" "$ckpt_target" ""
    fi
  fi

  # work_mem — stored in KB
  local wm_kb wm_mb max_conn rec_wm_kb
  wm_kb=$(ksql_q "SELECT setting FROM sys_settings WHERE name='work_mem';" | tr -d '[:space:]')
  wm_mb=$(( ${wm_kb:-0} / 1024 ))
  max_conn=$(ksql_q "SELECT setting FROM sys_settings WHERE name='max_connections';" | tr -d '[:space:]')

  if [[ $mem_mb -gt 0 && ${max_conn:-0} -gt 0 ]]; then
    rec_wm_kb=$(( mem_mb * 1024 / max_conn / 4 ))
    if [[ ${wm_kb:-0} -lt $(( rec_wm_kb / 2 )) ]]; then
      if [[ $collect -eq 1 ]]; then
        _finding "WARN" "param_work_mem" "work_mem 分配不足\n  症状：work_mem=${wm_mb}MB，${max_conn} 连接占用内存过多\n  建议:\n    · 调整至 ~$(( rec_wm_kb / 1024 ))MB（RAM/${max_conn}conn/4）"
      else
        warn "work_mem=${wm_mb}MB — recommend ~$(( rec_wm_kb / 1024 ))MB (RAM/${max_conn}conn/4)"
        json_item "advisor_param_work_mem" "warn" "${wm_mb}MB" "recommend ~$(( rec_wm_kb / 1024 ))MB"
      fi
      fix_lines+=("ALTER SYSTEM SET work_mem = '${rec_wm_kb}kB';")
      _exit=1
    else
      if [[ $collect -eq 0 ]]; then
        ok "work_mem=${wm_mb}MB ok"
        json_item "advisor_param_work_mem" "ok" "${wm_mb}MB" ""
      fi
    fi
  fi

  [[ $collect -eq 1 ]] && return $_exit

  if [[ $fix -eq 1 ]]; then
    echo ""
    echo "-- Fix: Params (apply with ALTER SYSTEM, then reload)"
    if [[ ${#fix_lines[@]} -gt 0 ]]; then
      for l in "${fix_lines[@]}"; do echo "$l"; done
      echo "SELECT sys_reload_conf();"
    else
      echo "-- (no parameter changes recommended)"
    fi
  fi

  return $_exit
}

_advisor_analyze() {
  local fix="${1:-0}"
  hdr "Advisor: Analyze"
  local _exit=0

  local stale_rows
  stale_rows=$(ksql_q "
    SELECT schemaname, relname,
           coalesce(last_analyze, last_autoanalyze, 'never')::text AS last_analyze,
           round(100.0*(n_live_tup - reltuples::bigint)/NULLIF(reltuples::bigint,0),1) AS drift_pct
    FROM sys_stat_user_tables
    JOIN sys_class sc ON sc.relname = sys_stat_user_tables.relname
    JOIN sys_namespace ns ON ns.oid = sc.relnamespace
      AND ns.nspname = sys_stat_user_tables.schemaname
    WHERE n_live_tup > 1000
      AND (
        coalesce(last_analyze, last_autoanalyze) < now() - interval '7 days'
        OR coalesce(last_analyze, last_autoanalyze) IS NULL
        OR abs(n_live_tup - reltuples::bigint) * 100.0 / NULLIF(reltuples::bigint, 0) > 10
      )
    ORDER BY abs(n_live_tup - reltuples::bigint) DESC
    LIMIT ${TOP_N};" 2>/dev/null || true)

  local stale_cnt; stale_cnt=$(echo "$stale_rows" | grep -c '.' 2>/dev/null || echo 0)
  [[ -z "$stale_rows" ]] && stale_cnt=0

  if [[ $stale_cnt -gt 0 ]]; then
    while IFS='|' read -r schema rel last_ana drift; do
      warn "${schema// /}.${rel// /}: drift=${drift// /}%, last analyze: ${last_ana// /}"
      json_item "advisor_analyze_stale" "warn" "${schema// /}.${rel// /}" "drift=${drift// /}%"
    done <<< "$stale_rows"
    _exit=1
  else
    ok "No tables with stale statistics (all analyzed within 7 days, drift < 10%)"
    json_item "advisor_analyze_stale" "ok" "0" ""
  fi

  if [[ $fix -eq 1 ]]; then
    echo ""
    echo "-- Fix: Analyze"
    if [[ -n "$stale_rows" ]]; then
      while IFS='|' read -r schema rel last_ana drift; do
        echo "ANALYZE ${schema// /}.${rel// /};"
      done <<< "$stale_rows"
    else
      echo "-- (no tables need ANALYZE)"
    fi
  fi

  return $_exit
}

cmd_advisor() {
  local sub="" fix=0

  # Parse all args as flat list — --fix may appear anywhere.
  # Note: dispatch passes "$SUBCMD" "${CMD_ARGS[@]}"; when kbdiag is invoked
  # with only "advisor" (no subcommand), bash's "shift 2" may leave the command
  # name in CMD_ARGS. We silently skip "advisor" here to handle that edge case.
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --fix)                              fix=1 ;;
      index|vacuum|params|analyze)        sub="$1" ;;
      ""|advisor)                         ;;
      *)
        echo "Unknown advisor option: $1. Use: index|vacuum|params|analyze [--fix]" >&2
        return 1
        ;;
    esac
    shift
  done

  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "advisor"

  local _exit=0

  case "$sub" in
    index)   _advisor_index  $fix || _exit=1 ;;
    vacuum)  _advisor_vacuum $fix || _exit=1 ;;
    params)  _advisor_params $fix || _exit=1 ;;
    analyze) _advisor_analyze $fix || _exit=1 ;;
    "")
      _advisor_index  $fix || _exit=1
      _advisor_vacuum $fix || _exit=1
      _advisor_params $fix || _exit=1
      _advisor_analyze $fix || _exit=1
      ;;
  esac

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  return $_exit
}
