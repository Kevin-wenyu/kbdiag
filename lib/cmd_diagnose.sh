# shellcheck shell=bash
# cmd_diagnose.sh — RCA root-cause diagnostic report

# ── 快速模式子函数 ─────────────────────────────────────────────────────────────

_diag_long_txn() {
  local rows
  rows=$(ksql_q "
    SELECT pid,
           usename,
           application_name,
           coalesce(client_addr::text, 'local') AS addr,
           state,
           date_trunc('second', now()-xact_start)::text AS duration,
           EXTRACT(EPOCH FROM (now()-xact_start))::int AS dur_sec,
           left(query, 100) AS query
    FROM sys_stat_activity
    WHERE state <> 'idle'
      AND xact_start IS NOT NULL
      AND pid <> sys_backend_pid()
      AND EXTRACT(EPOCH FROM (now()-xact_start)) > ${KB_WARN_OLDEST_TXN}
    ORDER BY xact_start
    LIMIT 10;" 2>/dev/null || true)

  local prep_rows
  prep_rows=$(ksql_q "
    SELECT gid,
           date_trunc('second', now()-prepared)::text AS duration,
           EXTRACT(EPOCH FROM (now()-prepared))::int AS dur_sec
    FROM sys_prepared_xacts
    ORDER BY prepared
    LIMIT 5;" 2>/dev/null || true)

  [[ -z "$rows" && -z "$prep_rows" ]] && return

  local level="WARN"
  local cnt=0
  local evidence=""

  if [[ -n "$rows" ]]; then
    while IFS='|' read -r pid user app addr state duration dur_sec query; do
      pid="${pid// /}"; [[ -z "$pid" ]] && continue
      cnt=$(( cnt + 1 ))
      [[ "${dur_sec// /}" -ge "${KB_FAIL_OLDEST_TXN}" ]] && level="CRITICAL"

      local safe_note=""
      if [[ "${state// /}" == "idle in transaction" ]]; then
        local blocks_count
        blocks_count=$(ksql_q "
          SELECT count(*) FROM sys_locks lw
          WHERE NOT lw.granted
            AND lw.pid <> ${pid}
            AND EXISTS (
              SELECT 1 FROM sys_locks lb
              WHERE lb.pid=${pid} AND lb.granted
                AND lb.locktype=lw.locktype
                AND lb.relation IS NOT DISTINCT FROM lw.relation
            );" 2>/dev/null | tr -d '[:space:]')
        local has_excl
        has_excl=$(ksql_q "
          SELECT count(*) FROM sys_locks
          WHERE pid=${pid} AND granted=true
            AND locktype IN ('relation','extend','tuple')
            AND mode LIKE '%Exclusive%';" 2>/dev/null | tr -d '[:space:]')
        [[ "${blocks_count:-0}" -eq 0 && "${has_excl:-0}" -eq 0 ]] && safe_note=" [可安全终止]"
      fi

      evidence="${evidence}    · pid ${pid} | user: ${user// /} | app: ${app// /} | addr: ${addr// /}\n"
      evidence="${evidence}      状态: ${state// /} | 时长: ${duration// /}${safe_note}\n"
      evidence="${evidence}      SQL: $(echo "$query" | xargs 2>/dev/null || echo "$query")\n"
    done <<< "$rows"
  fi

  if [[ -n "$prep_rows" ]]; then
    evidence="${evidence}  Prepared 事务:\n"
    while IFS='|' read -r gid duration dur_sec; do
      [[ -z "${gid// /}" ]] && continue
      evidence="${evidence}    · gid=${gid// /} | 时长: ${duration// /}\n"
    done <<< "$prep_rows"
  fi

  local block_rows
  block_rows=$(ksql_q "
    SELECT DISTINCT lb.pid AS block_pid, lw.pid AS wait_pid
    FROM sys_locks lw
    JOIN sys_locks lb ON lb.locktype=lw.locktype
      AND lb.relation IS NOT DISTINCT FROM lw.relation
      AND lb.granted
    WHERE NOT lw.granted AND lw.pid <> lb.pid
    LIMIT 10;" 2>/dev/null || true)

  local block_info=""
  if [[ -n "$block_rows" ]]; then
    block_info="  阻塞链:\n"
    while IFS='|' read -r block_pid wait_pid; do
      [[ -z "${block_pid// /}" ]] && continue
      block_info="${block_info}    · pid ${block_pid// /} 正在阻塞 pid ${wait_pid// /}\n"
    done <<< "$block_rows"
  fi

  local body
  body="长事务\n  症状：${cnt} 个事务持续 > ${KB_WARN_OLDEST_TXN}s\n  证据:\n${evidence}${block_info}  建议:\n    · [可安全终止] 标注的 pid 满足安全终止三条件\n      SELECT pg_terminate_backend(<pid>);\n    · 其余请先用 kbdiag sql <pid> 确认 SQL 性质"
  _finding "$level" "long_txn" "$body"
}

_diag_connections() {
  local cur_conn max_conn
  cur_conn=$(ksql_q "SELECT count(*) FROM sys_stat_activity;" | tr -d '[:space:]')
  max_conn=$(ksql_q "SELECT setting FROM sys_settings WHERE name='max_connections';" | tr -d '[:space:]')

  [[ -z "$cur_conn" || -z "$max_conn" || "${max_conn:-0}" -eq 0 ]] && return
  local pct=$(( cur_conn * 100 / max_conn ))
  [[ $pct -lt $KB_WARN_CONN ]] && return

  local level="WARN"
  [[ $pct -ge $KB_FAIL_CONN ]] && level="CRITICAL"

  local breakdown
  breakdown=$(ksql_q "
    SELECT state, count(*)
    FROM sys_stat_activity
    GROUP BY state ORDER BY count(*) DESC LIMIT 5;" 2>/dev/null | tr '|' ':' || true)

  local top_apps
  top_apps=$(ksql_q "
    SELECT application_name, count(*)
    FROM sys_stat_activity
    WHERE application_name <> ''
    GROUP BY application_name ORDER BY count(*) DESC LIMIT 3;" 2>/dev/null | tr '|' ':' || true)

  local body="连接数饱和\n  症状：当前 ${cur_conn}/${max_conn} 连接（${pct}%）\n  按状态分解:\n"
  while IFS=':' read -r state cnt; do
    [[ -z "${state// /}" ]] && continue
    body="${body}    · ${state// /}: ${cnt// /}\n"
  done <<< "$breakdown"
  body="${body}  Top 应用:\n"
  while IFS=':' read -r app cnt; do
    [[ -z "${app// /}" ]] && continue
    body="${body}    · ${app// /}: ${cnt// /}\n"
  done <<< "$top_apps"
  body="${body}  建议:\n    · 检查连接池配置（PgBouncer/应用侧）\n    · kbdiag sessions 查看详细会话"
  _finding "$level" "connections" "$body"
}

_diag_locks() {
  local wait_rows
  wait_rows=$(ksql_q "
    SELECT w.pid AS wait_pid, w.usename,
           left(w.query,60) AS wait_query,
           b.pid AS block_pid, b.usename AS block_user,
           lw.locktype, lw.mode AS wait_mode,
           date_trunc('second', now()-w.query_start)::text AS waiting_for
    FROM sys_stat_activity w
    JOIN sys_locks lw ON lw.pid=w.pid AND NOT lw.granted
    JOIN sys_locks lb ON lb.locktype=lw.locktype
      AND lb.relation IS NOT DISTINCT FROM lw.relation AND lb.granted
    JOIN sys_stat_activity b ON b.pid=lb.pid
    LIMIT ${TOP_N};" 2>/dev/null || true)

  [[ -z "$wait_rows" ]] && return

  local cnt=0 evidence=""
  while IFS='|' read -r wait_pid user query block_pid block_user locktype mode waiting_for; do
    [[ -z "${wait_pid// /}" ]] && continue
    cnt=$(( cnt + 1 ))
    evidence="${evidence}    · pid ${wait_pid// /} (${user// /}) 等待 ${mode// /} on ${locktype// /}\n"
    evidence="${evidence}      被 pid ${block_pid// /} (${block_user// /}) 阻塞，已等待 ${waiting_for// /}\n"
    evidence="${evidence}      SQL: $(echo "$query" | xargs 2>/dev/null || echo "$query")\n"
  done <<< "$wait_rows"

  local body="等待锁\n  症状：${cnt} 个会话等待锁\n  证据:\n${evidence}  建议:\n    · kbdiag locks wait 查看完整阻塞链\n    · kbdiag sql <block_pid> 确认持锁 SQL"
  _finding "WARN" "locks" "$body"
}

_diag_slow_queries() {
  local rows
  rows=$(ksql_q "
    SELECT pid, usename,
           date_trunc('second', now()-query_start)::text AS dur_sec,
           coalesce(wait_event_type,'') AS wait_event_type,
           coalesce(wait_event,'') AS wait_event,
           left(query, 80) AS query,
           coalesce(query_id::text, '') AS query_id
    FROM sys_stat_activity
    WHERE state='active'
      AND now()-query_start > interval '${KB_SLOW_THRESHOLD} seconds'
      AND pid <> sys_backend_pid()
    ORDER BY query_start
    LIMIT ${TOP_N};" 2>/dev/null || true)

  [[ -z "$rows" ]] && return

  local cnt=0 evidence="" pid_list=""
  while IFS='|' read -r pid user dur_sec wtype wevent query qid; do
    [[ -z "${pid// /}" ]] && continue
    cnt=$(( cnt + 1 ))
    evidence="${evidence}    · pid ${pid// /} (${user// /}) | 时长: ${dur_sec// /} | wait: ${wtype// /}/${wevent// /}\n"
    evidence="${evidence}      SQL: $(echo "$query" | xargs 2>/dev/null || echo "$query")\n"
    [[ -n "${qid// /}" ]] && pid_list="${pid_list:+${pid_list},}${pid// /}"
  done <<< "$rows"

  local hist_note=""
  local stmt_avail
  stmt_avail=$(ksql_q "SELECT count(*) FROM sys_catalog.sys_class WHERE relname='sys_stat_statements';" 2>/dev/null | tr -d '[:space:]')
  if [[ "${stmt_avail:-0}" -gt 0 && -n "$pid_list" ]]; then
    local qid_rows
    qid_rows=$(ksql_q "
      SELECT s.query_id::text,
             round(ss.mean_exec_time::numeric, 1)::text AS hist_mean_ms,
             round(ss.stddev_exec_time::numeric, 1)::text AS hist_stddev_ms,
             ss.calls::text
      FROM sys_stat_activity s
      JOIN sys_stat_statements ss ON ss.queryid = s.query_id
      WHERE s.pid = ANY(ARRAY[${pid_list}]);" 2>/dev/null || true)
    if [[ -n "$qid_rows" ]]; then
      while IFS='|' read -r qid hist_mean_ms hist_stddev_ms call_cnt; do
        [[ -z "${qid// /}" ]] && continue
        local hist_mean_s hist_stddev_s
        hist_mean_s="${hist_mean_ms// /}"
        hist_stddev_s="${hist_stddev_ms// /}"
        local hist_mean_fmt hist_stddev_fmt
        hist_mean_fmt=$(printf "%.1f" "$(echo "${hist_mean_s}/1000" | bc -l 2>/dev/null || echo 0)")s
        hist_stddev_fmt=$(printf "%.1f" "$(echo "${hist_stddev_s:-0}/1000" | bc -l 2>/dev/null || echo 0)")s
        local pattern
        if awk "BEGIN{exit !(${hist_stddev_s:-0} > ${hist_mean_s:-0} * 0.5)}"; then
          pattern="高波动（stddev ${hist_stddev_fmt} > mean×0.5），执行计划不稳定"
        else
          pattern="一贯慢（历史均值 ${hist_mean_fmt}，calls: ${call_cnt// /}）"
        fi
        hist_note="${hist_note}    · queryid ${qid// /}: 历史均值 ${hist_mean_fmt} | ${pattern}\n"
      done <<< "$qid_rows"
    fi
  fi

  local hist_section=""
  [[ -n "$hist_note" ]] && hist_section="\n  历史均值（sys_stat_statements）:\n${hist_note}"
  local body="慢查询\n  症状：当前 ${cnt} 条查询运行超过 ${KB_SLOW_THRESHOLD}s\n  证据:\n${evidence}${hist_section}  建议:\n    · kbdiag sql <pid> 查看执行计划\n    · kbdiag stmt <queryid> 查看历史统计"
  _finding "INFO" "slow_queries" "$body"
}

_diag_replication() {
  local is_standby
  is_standby=$(ksql_q "SELECT sys_is_in_recovery();" | tr -d '[:space:]')
  [[ "$is_standby" == "true" ]] && return

  local lag1
  lag1=$(ksql_q "
    SELECT coalesce(max(EXTRACT(EPOCH FROM write_lag)::int), -1)
    FROM sys_stat_replication;" 2>/dev/null | tr -d '[:space:]')
  [[ "${lag1:-}" == "-1" || -z "$lag1" ]] && return

  sleep "${KB_LAG_SAMPLE_INTERVAL:-3}"

  local lag2
  lag2=$(ksql_q "
    SELECT coalesce(max(EXTRACT(EPOCH FROM write_lag)::int), -1)
    FROM sys_stat_replication;" 2>/dev/null | tr -d '[:space:]')
  [[ "${lag2:-}" == "-1" ]] && lag2="$lag1"

  local delta=$(( ${lag2:-0} - ${lag1:-0} ))
  local direction="稳定"
  [[ $delta -gt 0 ]] && direction="增大中（+${delta}s/3s）"
  [[ $delta -lt 0 ]] && direction="收敛中（${delta}s/3s）"

  local level="OK"
  [[ "${lag1:-0}" -ge "${KB_WARN_LAG}" ]] && level="WARN"
  [[ "${lag1:-0}" -ge "${KB_FAIL_LAG}" ]] && level="CRITICAL"

  local slot_rows
  slot_rows=$(ksql_q "
    SELECT slot_name,
           pg_size_pretty(pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn)) AS lag_size,
           active
    FROM sys_replication_slots
    WHERE restart_lsn IS NOT NULL
    ORDER BY pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn) DESC
    LIMIT 5;" 2>/dev/null || true)

  local slot_warn=0
  local slot_info=""
  if [[ -n "$slot_rows" ]]; then
    slot_info="  复制槽:\n"
    while IFS='|' read -r slot_name lag_size active; do
      [[ -z "${slot_name// /}" ]] && continue
      slot_info="${slot_info}    · ${slot_name// /}: lag=${lag_size// /}, active=${active// /}\n"
      [[ "${active// /}" == "false" ]] && slot_warn=1
    done <<< "$slot_rows"
    [[ $slot_warn -eq 1 && "$level" == "OK" ]] && level="WARN"
  fi

  [[ "$level" == "OK" ]] && return

  local body="复制延迟\n  症状：standby write_lag=${lag1}s，${direction}\n${slot_info}  建议:\n    · kbdiag replication 查看详情"
  _finding "$level" "replication" "$body"
}

_diag_disk() {
  local data_dir="${KB_DATA_DIR}"
  local disk_info
  disk_info=$(df -h "$data_dir" 2>/dev/null | tail -1 || true)
  [[ -z "$disk_info" ]] && return

  local used_pct
  used_pct=$(echo "$disk_info" | awk '{for(i=1;i<=NF;i++) if($i~/^[0-9]+%$/) {gsub(/%/,"",$i); print $i; exit}}')
  local db_size
  db_size=$(ksql_q "SELECT pg_size_pretty(pg_database_size(current_database()));" | tr -d '[:space:]')

  local level="OK"
  [[ "${used_pct:-0}" -ge 70 ]] && level="WARN"
  [[ "${used_pct:-0}" -ge 85 ]] && level="CRITICAL"

  local archive_path="${KB_DATA_DIR}/sys_wal/archive_status"
  local ready_cnt=0
  # shellcheck disable=SC2012
  [[ -d "$archive_path" ]] && ready_cnt=$(ls "$archive_path"/*.ready 2>/dev/null | wc -l | tr -d '[:space:]') || ready_cnt=0

  [[ "$level" == "OK" && "${ready_cnt:-0}" -lt 10 ]] && return

  local body="磁盘空间\n  症状：data 目录使用 ${used_pct:-?}% | DB 大小: ${db_size}\n  磁盘详情: $disk_info\n"
  [[ "${ready_cnt:-0}" -ge 10 ]] && body="${body}  归档积压: ${ready_cnt} 个 ready 文件待归档\n"
  body="${body}  建议:\n    · kbdiag space 查看表/WAL 占用详情"
  _finding "$level" "disk" "$body"
}

# WAL 归档失败根因链：症状（WAL 积压）→ 证据（archiver 失败计数/最后失败段）
# → 根因（archive_command 执行失败）→ 建议（用 kbdiag backup 验证）。
# 复用 cmd_backup.sh 的 _backup_archiver_row。
_diag_archiver() {
  local arch_mode
  arch_mode=$(ksql_q "SELECT setting FROM sys_settings WHERE name='archive_mode';" | tr -d '[:space:]')
  [[ -z "$arch_mode" || "$arch_mode" == "off" ]] && return

  local row
  row=$(_backup_archiver_row) || return
  local verdict failed last_ok last_fail fail_wal
  IFS='|' read -r verdict _ failed last_ok last_fail fail_wal <<< "$row"
  verdict="${verdict// /}"
  [[ "$verdict" != "failing" ]] && return

  local ready_cnt arch_cmd
  ready_cnt=$(find "${KB_DATA_DIR}/sys_wal/archive_status" -name '*.ready' 2>/dev/null | wc -l | tr -d '[:space:]')
  arch_cmd=$(ksql_q "SELECT setting FROM sys_settings WHERE name='archive_command';")

  local level="WARN"
  [[ "${ready_cnt:-0}" -ge "${KB_FAIL_WAL_READY:-100}" ]] && level="CRITICAL"

  local body="WAL 归档失败\n"
  body="${body}  症状：${ready_cnt:-0} 个 WAL 段待归档（.ready 积压），持续增长会撑满磁盘\n"
  body="${body}  证据：archiver 累计失败 ${failed} 次 | 最后失败段: ${fail_wal} @ ${last_fail} | 最后成功: ${last_ok}\n"
  body="${body}  根因：archive_command 执行失败: ${arch_cmd# }\n"
  body="${body}  建议:\n"
  body="${body}    · kbdiag backup 查看完整归档/备份链检查\n"
  body="${body}    · 以 kingbase 用户手工执行 archive_command（替换 %p 为任一 WAL 段路径）看真实报错\n"
  body="${body}    · sys_rman 场景：检查 config/stanza/仓库目录权限与磁盘空间"
  _finding "$level" "wal_archiving" "$body"
}

_diag_xid_age() {
  local max_age
  max_age=$(ksql_q "
    SELECT max(age(relfrozenxid))
    FROM sys_class
    WHERE relkind IN ('r','t') AND relfrozenxid != 0;" | tr -d '[:space:]')

  [[ -z "$max_age" || "${max_age:-0}" -lt "${KB_WARN_XID}" ]] && return

  local level="WARN"
  [[ "${max_age}" -ge "${KB_FAIL_XID}" ]] && level="CRITICAL"
  local remaining=$(( 2000000000 - max_age ))

  local top_tables
  top_tables=$(ksql_q "
    SELECT relname, age(relfrozenxid) AS xid_age
    FROM sys_class
    WHERE relkind IN ('r','t') AND relfrozenxid != 0
    ORDER BY age(relfrozenxid) DESC
    LIMIT 5;" 2>/dev/null | tr '|' ':' || true)

  local body="XID Wraparound 风险\n  症状：最大 xid_age=${max_age}，距 wraparound 还有约 ${remaining} 个事务\n  高风险表:\n"
  while IFS=':' read -r rel age; do
    [[ -z "${rel// /}" ]] && continue
    body="${body}    · ${rel// /}: age=${age// /}\n"
  done <<< "$top_tables"
  body="${body}  建议:\n    · VACUUM FREEZE <table>;\n    · kbdiag advisor vacuum 查看冻结建议"
  _finding "$level" "xid_age" "$body"
}

# ── 完整模式追加子函数 ──────────────────────────────────────────────────────────

_diag_indexes() {
  _advisor_index --collect || true
}

_diag_params() {
  _advisor_params --collect || true
}

# Merges the old separate vacuum_debt(>20%)/bloat(>30%) checks — both queried
# the same dead-tuple ratio on the same tables and could double-report a
# single table under two findings. Now one query, severity escalates at
# KB_FAIL_DEAD_PCT instead of firing a second finding for the same table.
_diag_bloat() {
  local rows
  rows=$(ksql_q "
    SELECT schemaname, relname, n_dead_tup,
           pg_size_pretty(pg_relation_size(schemaname||'.'||relname)) AS size,
           round(100.0*n_dead_tup/NULLIF(n_live_tup+n_dead_tup,0),1) AS dead_pct,
           coalesce(last_autovacuum::date::text, last_vacuum::date::text, 'never') AS last_vac
    FROM sys_stat_user_tables
    WHERE n_live_tup+n_dead_tup > 1000
      AND n_dead_tup::float/NULLIF(n_live_tup+n_dead_tup,0) > ${KB_WARN_DEAD_PCT}/100.0
    ORDER BY n_dead_tup DESC
    LIMIT ${TOP_N};" 2>/dev/null || true)

  [[ -z "$rows" ]] && return

  local cnt=0 warn_evidence="" fail_evidence=""
  while IFS='|' read -r schema rel dead_tup size dead_pct last_vac; do
    [[ -z "${schema// /}" ]] && continue
    cnt=$(( cnt + 1 ))
    local pct="${dead_pct// /}"
    if awk "BEGIN{exit !(${pct} > ${KB_FAIL_DEAD_PCT})}"; then
      fail_evidence="${fail_evidence}    · ${schema// /}.${rel// /}: ${size// /}, dead=${pct}% (${dead_tup// /} tuples), last vacuum: ${last_vac// /}\n"
    else
      warn_evidence="${warn_evidence}    · ${schema// /}.${rel// /}: dead=${pct}% (${dead_tup// /} tuples), last vacuum: ${last_vac// /}\n"
    fi
  done <<< "$rows"

  if [[ -n "$fail_evidence" ]]; then
    _finding "CRITICAL" "bloat" "表膨胀严重\n  症状：${cnt} 张表 dead tuple 率 > ${KB_WARN_DEAD_PCT}%，其中部分已超过 ${KB_FAIL_DEAD_PCT}%\n  证据:\n${fail_evidence}${warn_evidence}  建议:\n    · VACUUM (ANALYZE) <table>;\n    · 考虑 pg_repack 在线整理\n    · kbdiag advisor vacuum --fix 生成修复 SQL"
  else
    _finding "WARN" "bloat" "Autovacuum 积压\n  症状：${cnt} 张表 dead tuple 率 > ${KB_WARN_DEAD_PCT}%\n  证据:\n${warn_evidence}  建议:\n    · VACUUM (ANALYZE) <table>;\n    · kbdiag advisor vacuum --fix 生成修复 SQL"
  fi
}

_diag_buffer_hit() {
  local hit_rate
  hit_rate=$(ksql_q "
    SELECT CASE WHEN (blks_hit+blks_read)=0 THEN 100
                ELSE round(100.0*blks_hit/(blks_hit+blks_read),1) END
    FROM sys_stat_database WHERE datname=current_database();" | tr -d '[:space:]')

  [[ -z "$hit_rate" ]] && return
  if awk "BEGIN{exit !(${hit_rate} < ${KB_FAIL_HIT})}"; then
    _finding "CRITICAL" "buffer_hit" "Buffer 命中率低\n  症状：${hit_rate}% < ${KB_FAIL_HIT}%\n  建议:\n    · 增大 shared_buffers\n    · kbdiag advisor params --fix 生成建议"
  elif awk "BEGIN{exit !(${hit_rate} < ${KB_WARN_HIT})}"; then
    _finding "WARN" "buffer_hit" "Buffer 命中率偏低\n  症状：${hit_rate}% < ${KB_WARN_HIT}%\n  建议:\n    · 考虑增大 shared_buffers"
  fi
}

_diag_checkpoint() {
  local req_ckpt
  req_ckpt=$(ksql_q "SELECT checkpoints_req FROM sys_stat_bgwriter;" | tr -d '[:space:]')
  [[ "${req_ckpt:-0}" -eq 0 ]] && return

  _finding "INFO" "checkpoint" "强制 Checkpoint\n  症状：已发生 ${req_ckpt} 次强制 checkpoint（checkpoints_req > 0）\n  建议:\n    · 检查 max_wal_size 是否过小\n    · kbdiag perf wal 查看 WAL/checkpoint 统计"
}

_diag_temp() {
  local temp_bytes
  temp_bytes=$(ksql_q "
    SELECT coalesce(sum(temp_bytes), 0)
    FROM sys_stat_database;" | tr -d '[:space:]')

  [[ "${temp_bytes:-0}" -lt "${KB_WARN_TEMP}" ]] && return

  local temp_pretty
  temp_pretty=$(ksql_q "
    SELECT pg_size_pretty(coalesce(sum(temp_bytes),0))
    FROM sys_stat_database;" | tr -d '[:space:]')

  local level="WARN"
  [[ "${temp_bytes:-0}" -ge "${KB_FAIL_TEMP}" ]] && level="CRITICAL"

  _finding "$level" "temp_files" "临时文件使用量大\n  症状：累计临时文件 ${temp_pretty}\n  建议:\n    · 增大 work_mem 减少排序溢出\n    · kbdiag temp 查看具体会话"
}

_diag_stmt_top() {
  _stmt_check_ext || return

  local stats_reset
  stats_reset=$(ksql_q "SELECT coalesce(stats_reset::date::text,'unknown') FROM sys_stat_bgwriter;" 2>/dev/null | tr -d '[:space:]')

  local rows
  rows=$(ksql_q "
    SELECT queryid::text,
           round((total_exec_time/1000.0)::numeric,1)::text AS total_s,
           round(100.0*total_exec_time/NULLIF(sum(total_exec_time) OVER(),0),1)::text AS pct,
           calls::text,
           round(mean_exec_time::numeric,1)::text AS mean_ms,
           left(replace(query, E'\\n', ' '), 70) AS summary
    FROM sys_stat_statements
    WHERE $_STMT_ACTIVE_WHERE
    ORDER BY total_exec_time DESC
    LIMIT 3;" 2>/dev/null || true)

  [[ -z "$rows" ]] && return

  local evidence=""
  while IFS='|' read -r qid total_s pct calls mean_ms summary; do
    [[ -z "${qid// /}" ]] && continue
    evidence="${evidence}  · queryid=${qid// /}  总耗时: ${total_s// /}s (${pct// /}%)  calls: ${calls// /}  mean: ${mean_ms// /}ms\n"
    evidence="${evidence}    ${summary}\n"
  done <<< "$rows"

  local body="历史 SQL 负担 Top 3（累计总耗时，stats_reset: ${stats_reset}）\n${evidence}  验证: kbdiag stmt（查看完整 AWR 报告）"
  _finding "INFO" "stmt_top" "$body"
}

# ── 渲染 ────────────────────────────────────────────────────────────────────────

_diag_render() {
  local full_mode="$1" elapsed="$2"
  local mode_label="快速模式"
  [[ "$full_mode" -eq 1 ]] && mode_label="完整模式"

  local critical=0 warn_ct=0 info_ct=0
  local i
  for (( i=0; i<${#FINDINGS_LEVEL[@]}; i++ )); do
    case "${FINDINGS_LEVEL[$i]}" in
      CRITICAL) critical=$(( critical + 1 )) ;;
      WARN)     warn_ct=$(( warn_ct + 1 )) ;;
      INFO)     info_ct=$(( info_ct + 1 )) ;;
    esac
  done

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    for (( i=0; i<${#FINDINGS_LEVEL[@]}; i++ )); do
      json_finding "${FINDINGS_LEVEL[$i]}" "${FINDINGS_CATEGORY[$i]}" "${FINDINGS_BODY[$i]}"
    done
    json_diagnose_end "$( [[ "$full_mode" -eq 1 ]] && echo full || echo fast )"
    return
  fi

  # shellcheck disable=SC2059
  printf "\n${BOLD}==> Diagnose  [${mode_label}，耗时 ${elapsed}s]${RESET}\n"
  echo ""

  if [[ $critical -eq 0 && $warn_ct -eq 0 && $info_ct -eq 0 ]]; then
    # shellcheck disable=SC2059
    printf "${GREEN}● 未发现异常${RESET}\n"
    [[ "$full_mode" -eq 0 ]] && echo "  加 --full 可运行完整检查（预计约 90s）"
    return
  fi

  echo "● 发现 ${critical} 个严重问题，${warn_ct} 个需关注，${info_ct} 条信息"
  echo ""

  for level_filter in CRITICAL WARN INFO; do
    for (( i=0; i<${#FINDINGS_LEVEL[@]}; i++ )); do
      [[ "${FINDINGS_LEVEL[$i]}" != "$level_filter" ]] && continue
      case "$level_filter" in
        CRITICAL) printf "${RED}[CRITICAL]${RESET} %b\n\n" "${FINDINGS_BODY[$i]}" ;;
        WARN)     printf "${YELLOW}[WARN]${RESET}  %b\n\n"     "${FINDINGS_BODY[$i]}" ;;
        INFO)     printf "${CYAN}[INFO]${RESET}  %b\n\n"       "${FINDINGS_BODY[$i]}" ;;
      esac
    done
  done

  [[ "$full_mode" -eq 0 ]] && echo "  加 --full 可运行完整检查（预计约 90s）"
}

# ── 入口 ─────────────────────────────────────────────────────────────────────────

cmd_diagnose() {
  local full_mode=0
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --full) full_mode=1 ;;
    esac
    shift
  done

  FINDINGS_LEVEL=()
  FINDINGS_CATEGORY=()
  FINDINGS_BODY=()
  _JSON_FINDINGS=""

  local t_start; t_start=$(date +%s)

  _diag_long_txn
  _diag_connections
  _diag_locks
  _diag_slow_queries
  _diag_replication
  _diag_disk
  _diag_archiver
  _diag_xid_age

  if [[ $full_mode -eq 1 ]]; then
    _diag_indexes
    _diag_params
    _diag_bloat
    _diag_buffer_hit
    _diag_checkpoint
    _diag_temp
    _diag_stmt_top
  fi

  local t_end; t_end=$(date +%s)
  _diag_render "$full_mode" $(( t_end - t_start ))
}
