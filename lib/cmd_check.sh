# shellcheck shell=bash
_check_level() {
  # $1=current_exit $2=new_level(1=warn,2=fail)
  [[ $2 -gt $1 ]] && echo "$2" || echo "$1"
}

cmd_check() {
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "check"
  hdr "Health check"
  local _exit=0   # 0=OK, 1=WARN, 2=FAIL

  # 1. 连接数占比
  local conn_used max_conn conn_pct
  conn_used=$(ksql_q "SELECT count(*) FROM sys_stat_activity;" | tr -d '[:space:]')
  max_conn=$(ksql_q "SELECT setting::int FROM sys_settings WHERE name='max_connections';" | tr -d '[:space:]')
  conn_used=${conn_used:-0}
  max_conn=${max_conn:-1}
  conn_pct=$(( conn_used * 100 / max_conn ))
  if [[ $conn_pct -ge $KB_FAIL_CONN ]]; then
    fail "Connections: ${conn_pct}% (${conn_used}/${max_conn}) >= FAIL threshold ${KB_FAIL_CONN}%"
    json_item "connections" "fail" "${conn_used}/${max_conn} (${conn_pct}%)" ">= FAIL threshold ${KB_FAIL_CONN}%"
    _exit=$(_check_level "$_exit" 2)
  elif [[ $conn_pct -ge $KB_WARN_CONN ]]; then
    warn "Connections: ${conn_pct}% (${conn_used}/${max_conn}) >= WARN threshold ${KB_WARN_CONN}%"
    json_item "connections" "warn" "${conn_used}/${max_conn} (${conn_pct}%)" ">= WARN threshold ${KB_WARN_CONN}%"
    _exit=$(_check_level "$_exit" 1)
  else
    ok "Connections: ${conn_pct}% (${conn_used}/${max_conn})"
    json_item "connections" "ok" "${conn_used}/${max_conn} (${conn_pct}%)" ""
  fi
  if [[ -n "$VERBOSE" ]]; then
    ksql_qh "SELECT usename, count(*) FROM sys_stat_activity GROUP BY usename ORDER BY 2 DESC LIMIT 10;" \
      | column -t -s '|' || true
  fi

  # 2. 复制延迟（仅 standby）
  local is_standby
  is_standby=$(ksql_q "SELECT pg_is_in_recovery()::text;" | tr -d '[:space:]')
  if [[ "$is_standby" == "true" ]]; then
    local lag_sec
    lag_sec=$(ksql_q "SELECT EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp()))::int;" | tr -d '[:space:]')
    lag_sec=${lag_sec:-0}
    if [[ $lag_sec -ge $KB_FAIL_LAG ]]; then
      fail "Replication lag: ${lag_sec}s >= FAIL threshold ${KB_FAIL_LAG}s"
      json_item "replication_lag" "fail" "${lag_sec}s" ">= FAIL threshold ${KB_FAIL_LAG}s"
      _exit=$(_check_level "$_exit" 2)
    elif [[ $lag_sec -ge $KB_WARN_LAG ]]; then
      warn "Replication lag: ${lag_sec}s >= WARN threshold ${KB_WARN_LAG}s"
      json_item "replication_lag" "warn" "${lag_sec}s" ">= WARN threshold ${KB_WARN_LAG}s"
      _exit=$(_check_level "$_exit" 1)
    else
      ok "Replication lag: ${lag_sec}s"
      json_item "replication_lag" "ok" "${lag_sec}s" ""
    fi
  else
    json_item "replication_lag" "ok" "N/A" "primary node"
  fi

  # 3. 长事务
  local long_fail long_warn
  long_fail=$(ksql_q "
    SELECT count(*) FROM sys_stat_activity
    WHERE state = 'idle in transaction'
      AND now() - xact_start > interval '${KB_FAIL_TXN} seconds';" | tr -d '[:space:]')
  long_warn=$(ksql_q "
    SELECT count(*) FROM sys_stat_activity
    WHERE state IN ('active','idle in transaction')
      AND now() - xact_start > interval '${KB_WARN_TXN} seconds';" | tr -d '[:space:]')
  if [[ ${long_fail:-0} -gt 0 ]]; then
    fail "Long transactions: ${long_fail} exceeding FAIL threshold ${KB_FAIL_TXN}s"
    json_item "long_transactions" "fail" "${long_fail}" "exceeding FAIL threshold ${KB_FAIL_TXN}s"
    _exit=$(_check_level "$_exit" 2)
  elif [[ ${long_warn:-0} -gt 0 ]]; then
    warn "Long transactions: ${long_warn} exceeding WARN threshold ${KB_WARN_TXN}s"
    json_item "long_transactions" "warn" "${long_warn}" "exceeding WARN threshold ${KB_WARN_TXN}s"
    _exit=$(_check_level "$_exit" 1)
  else
    ok "Long transactions: none"
    json_item "long_transactions" "ok" "0" ""
  fi
  if [[ -n "$VERBOSE" && ${long_warn:-0} -gt 0 ]]; then
    ksql_qh "
      SELECT pid, usename, state,
             date_trunc('second', now()-xact_start)::text AS duration,
             left(query,80) AS query
      FROM sys_stat_activity
      WHERE state IN ('active','idle in transaction')
        AND now() - xact_start > interval '${KB_WARN_TXN} seconds';" \
      | column -t -s '|' || true
  fi

  # 4. 等待锁
  local lock_cnt
  lock_cnt=$(ksql_q "SELECT count(*) FROM sys_locks WHERE NOT granted;" | tr -d '[:space:]')
  if [[ ${lock_cnt:-0} -gt 0 ]]; then
    warn "Waiting locks: ${lock_cnt}"
    json_item "waiting_locks" "warn" "${lock_cnt}" "sessions waiting on locks"
    _exit=$(_check_level "$_exit" 1)
    if [[ -n "$VERBOSE" ]]; then
      ksql_qh "
        SELECT w.pid, w.usename, left(w.query,60) AS wait_query,
               b.pid AS block_pid, left(b.query,60) AS block_query
        FROM sys_stat_activity w
        JOIN sys_locks lw ON lw.pid=w.pid AND NOT lw.granted
        JOIN sys_locks lb ON lb.locktype=lw.locktype
                          AND lb.relation IS NOT DISTINCT FROM lw.relation
                          AND lb.granted
        JOIN sys_stat_activity b ON b.pid=lb.pid LIMIT 10;" \
        | column -t -s '|' || true
    fi
  else
    ok "Waiting locks: none"
    json_item "waiting_locks" "ok" "0" ""
  fi

  # 5. 归档失败
  local arch_failed arch_status
  arch_failed=$(ksql_q "SELECT failed_count FROM sys_stat_archiver;" | tr -d '[:space:]')
  arch_status=$(ksql_q "SELECT 'archived='||archived_count||' failed='||failed_count||' last_failed='||coalesce(last_failed_wal,'none') FROM sys_stat_archiver;")
  if [[ ${arch_failed:-0} -gt 0 ]]; then
    warn "Archiver: ${arch_failed} failed file(s)"
    json_item "archiver" "warn" "${arch_failed}" "failed WAL file(s)"
    _exit=$(_check_level "$_exit" 1)
    [[ -n "$VERBOSE" ]] && info "$arch_status"
  else
    ok "Archiver: no failures"
    json_item "archiver" "ok" "${arch_failed:-0}" ""
  fi

  # 6. autovacuum 积压
  local vac_cnt
  vac_cnt=$(ksql_q "
    SELECT count(*) FROM sys_stat_user_tables
    WHERE n_dead_tup > ${KB_WARN_DEAD_TUPLES};" | tr -d '[:space:]')
  if [[ ${vac_cnt:-0} -gt 0 ]]; then
    warn "Autovacuum backlog: ${vac_cnt} table(s) with dead tuples > ${KB_WARN_DEAD_TUPLES}"
    json_item "autovacuum_backlog" "warn" "${vac_cnt}" "tables with dead_tup > ${KB_WARN_DEAD_TUPLES}"
    _exit=$(_check_level "$_exit" 1)
    if [[ -n "$VERBOSE" ]]; then
      ksql_qh "
        SELECT schemaname, relname, n_dead_tup, last_autovacuum
        FROM sys_stat_user_tables
        WHERE n_dead_tup > ${KB_WARN_DEAD_TUPLES}
        ORDER BY n_dead_tup DESC LIMIT 10;" \
        | column -t -s '|' || true
    fi
  else
    ok "Autovacuum backlog: none"
    json_item "autovacuum_backlog" "ok" "0" ""
  fi

  # 7. 缓冲命中率
  local hit_rate hit_rate_int
  hit_rate=$(ksql_q "SELECT CASE WHEN (blks_hit + blks_read) = 0 THEN 100 ELSE round(100.0 * blks_hit / (blks_hit + blks_read), 1) END FROM sys_stat_database WHERE datname = current_database();" | tr -d '[:space:]')
  hit_rate=${hit_rate:-100}
  hit_rate_int=$(echo "$hit_rate" | cut -d. -f1)
  if [[ $hit_rate_int -lt $KB_FAIL_HIT ]]; then
    fail "Buffer hit rate: ${hit_rate}% < FAIL threshold ${KB_FAIL_HIT}%"
    _exit=$(_check_level "$_exit" 2)
    json_item "buffer_hit" "fail" "${hit_rate}%" "< FAIL threshold ${KB_FAIL_HIT}%"
  elif [[ $hit_rate_int -lt $KB_WARN_HIT ]]; then
    warn "Buffer hit rate: ${hit_rate}% < WARN threshold ${KB_WARN_HIT}%"
    _exit=$(_check_level "$_exit" 1)
    json_item "buffer_hit" "warn" "${hit_rate}%" "< WARN threshold ${KB_WARN_HIT}%"
  else
    ok "Buffer hit rate: ${hit_rate}%"
    json_item "buffer_hit" "ok" "${hit_rate}%" ""
  fi

  # 8. Checkpoint 频率（checkpoints_req > KB_FAIL_CKPT 即 WARN）
  local ckpt_req
  ckpt_req=$(ksql_q "SELECT checkpoints_req FROM sys_stat_bgwriter;" | tr -d '[:space:]')
  ckpt_req=${ckpt_req:-0}
  if [[ $ckpt_req -ge $KB_WARN_CKPT ]]; then
    warn "Checkpoint pressure: ${ckpt_req} requested checkpoints >= WARN threshold ${KB_WARN_CKPT}"
    _exit=$(_check_level "$_exit" 1)
    json_item "checkpoint" "warn" "${ckpt_req}" ">= WARN threshold ${KB_WARN_CKPT}"
  else
    ok "Checkpoint pressure: ${ckpt_req} requested checkpoints"
    json_item "checkpoint" "ok" "${ckpt_req}" ""
  fi

  # 9. 临时文件使用（当前累计字节数）
  local temp_bytes
  temp_bytes=$(ksql_q "SELECT temp_bytes FROM sys_stat_database WHERE datname = current_database();" | tr -d '[:space:]')
  temp_bytes=${temp_bytes:-0}
  if [[ $temp_bytes -ge $KB_FAIL_TEMP ]]; then
    fail "Temp file usage: ${temp_bytes} bytes >= FAIL threshold ${KB_FAIL_TEMP}"
    _exit=$(_check_level "$_exit" 2)
    json_item "temp_files" "fail" "${temp_bytes}" ">= FAIL threshold ${KB_FAIL_TEMP}"
  elif [[ $temp_bytes -ge $KB_WARN_TEMP ]]; then
    warn "Temp file usage: ${temp_bytes} bytes >= WARN threshold ${KB_WARN_TEMP}"
    _exit=$(_check_level "$_exit" 1)
    json_item "temp_files" "warn" "${temp_bytes}" ">= WARN threshold ${KB_WARN_TEMP}"
  else
    ok "Temp file usage: ${temp_bytes} bytes"
    json_item "temp_files" "ok" "${temp_bytes}" ""
  fi

  # 10. 死锁计数
  local deadlocks
  deadlocks=$(ksql_q "SELECT deadlocks FROM sys_stat_database WHERE datname = current_database();" | tr -d '[:space:]')
  deadlocks=${deadlocks:-0}
  if [[ $deadlocks -gt 0 ]]; then
    warn "Deadlocks: ${deadlocks} since last reset"
    _exit=$(_check_level "$_exit" 1)
    json_item "deadlocks" "warn" "${deadlocks}" "since last reset"
  else
    ok "Deadlocks: none"
    json_item "deadlocks" "ok" "0" ""
  fi

  # 11. 复制槽延迟（最大 slot lag）
  local slot_lag_pretty slot_lag_bytes
  slot_lag_bytes=$(ksql_q "SELECT COALESCE(max(pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn)), 0) FROM sys_replication_slots WHERE active = false OR restart_lsn IS NOT NULL;" | tr -d '[:space:]')
  slot_lag_bytes=${slot_lag_bytes:-0}
  slot_lag_pretty=$(ksql_q "SELECT pg_size_pretty(COALESCE(max(pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn)), 0)) FROM sys_replication_slots WHERE active = false OR restart_lsn IS NOT NULL;" | tr -d '[:space:]')
  slot_lag_pretty=${slot_lag_pretty:-0}
  if [[ $slot_lag_bytes -ge $KB_FAIL_SLOT ]]; then
    fail "Replication slot lag: ${slot_lag_pretty} >= FAIL threshold $(( KB_FAIL_SLOT / 1048576 ))MB"
    _exit=$(_check_level "$_exit" 2)
    json_item "slot_lag" "fail" "${slot_lag_pretty}" ">= FAIL threshold"
  elif [[ $slot_lag_bytes -ge $KB_WARN_SLOT ]]; then
    warn "Replication slot lag: ${slot_lag_pretty} >= WARN threshold $(( KB_WARN_SLOT / 1048576 ))MB"
    _exit=$(_check_level "$_exit" 1)
    json_item "slot_lag" "warn" "${slot_lag_pretty}" ">= WARN threshold"
  else
    ok "Replication slot lag: ${slot_lag_pretty}"
    json_item "slot_lag" "ok" "${slot_lag_pretty}" ""
  fi

  # 12. bgwriter 压力（backend 直写 buffer 占比）
  local bgw_pct
  bgw_pct=$(ksql_q "SELECT CASE WHEN (buffers_clean + buffers_backend + buffers_checkpoint) = 0 THEN 0 ELSE round(100.0 * buffers_backend / (buffers_clean + buffers_backend + buffers_checkpoint)) END FROM sys_stat_bgwriter;" | tr -d '[:space:]')
  bgw_pct=${bgw_pct:-0}
  if [[ $bgw_pct -ge $KB_FAIL_BGW ]]; then
    fail "BGWriter pressure: ${bgw_pct}% backend writes >= FAIL threshold ${KB_FAIL_BGW}%"
    _exit=$(_check_level "$_exit" 2)
    json_item "bgwriter" "fail" "${bgw_pct}%" ">= FAIL threshold ${KB_FAIL_BGW}%"
  elif [[ $bgw_pct -ge $KB_WARN_BGW ]]; then
    warn "BGWriter pressure: ${bgw_pct}% backend writes >= WARN threshold ${KB_WARN_BGW}%"
    _exit=$(_check_level "$_exit" 1)
    json_item "bgwriter" "warn" "${bgw_pct}%" ">= WARN threshold ${KB_WARN_BGW}%"
  else
    ok "BGWriter pressure: ${bgw_pct}% backend writes"
    json_item "bgwriter" "ok" "${bgw_pct}%" ""
  fi

  # 13. XID age（最大表级 frozen xid age）
  local xid_age
  xid_age=$(ksql_q "SELECT max(age(relfrozenxid)) FROM sys_class WHERE relkind IN ('r','t') AND relfrozenxid != 0;" | tr -d '[:space:]')
  xid_age=${xid_age:-0}
  if [[ $xid_age -ge $KB_FAIL_XID ]]; then
    fail "XID age: ${xid_age} >= FAIL threshold ${KB_FAIL_XID}"
    _exit=$(_check_level "$_exit" 2)
    json_item "xid_age" "fail" "${xid_age}" ">= FAIL threshold ${KB_FAIL_XID}"
  elif [[ $xid_age -ge $KB_WARN_XID ]]; then
    warn "XID age: ${xid_age} >= WARN threshold ${KB_WARN_XID}"
    _exit=$(_check_level "$_exit" 1)
    json_item "xid_age" "warn" "${xid_age}" ">= WARN threshold ${KB_WARN_XID}"
  else
    ok "XID age: ${xid_age}"
    json_item "xid_age" "ok" "${xid_age}" ""
  fi

  # 14. 最老活跃事务年龄（秒）
  local oldest_txn
  oldest_txn=$(ksql_q "SELECT COALESCE(EXTRACT(EPOCH FROM (now() - min(xact_start)))::int, 0) FROM sys_stat_activity WHERE state <> 'idle' AND xact_start IS NOT NULL AND pid <> sys_backend_pid();" | tr -d '[:space:]')
  oldest_txn=${oldest_txn:-0}
  if [[ $oldest_txn -ge $KB_FAIL_OLDEST_TXN ]]; then
    fail "oldest active transaction: ${oldest_txn}s >= FAIL threshold ${KB_FAIL_OLDEST_TXN}s"
    _exit=$(_check_level "$_exit" 2)
    json_item "oldest_txn" "fail" "${oldest_txn}s" ">= FAIL threshold ${KB_FAIL_OLDEST_TXN}s"
  elif [[ $oldest_txn -ge $KB_WARN_OLDEST_TXN ]]; then
    warn "oldest active transaction: ${oldest_txn}s >= WARN threshold ${KB_WARN_OLDEST_TXN}s"
    _exit=$(_check_level "$_exit" 1)
    json_item "oldest_txn" "warn" "${oldest_txn}s" ">= WARN threshold ${KB_WARN_OLDEST_TXN}s"
  else
    ok "oldest active transaction: ${oldest_txn}s"
    json_item "oldest_txn" "ok" "${oldest_txn}s" ""
  fi

  # 15. WAL 归档状态（archive_mode=off 时视为 ok，是否该开归档由 backup 命令提示）
  local arch_mode arch_row
  arch_mode=$(ksql_q "SELECT setting FROM sys_settings WHERE name='archive_mode';" | tr -d '[:space:]')
  if [[ -z "$arch_mode" || "$arch_mode" == "off" ]]; then
    ok "WAL archiving: disabled (archive_mode=off)"
    json_item "wal_archiving" "ok" "disabled" ""
  elif arch_row=$(_backup_archiver_row); then
    local arch_verdict arch_failed arch_fail_time ready_cnt
    IFS='|' read -r arch_verdict _ arch_failed _ arch_fail_time _ <<< "$arch_row"
    arch_verdict="${arch_verdict// /}"
    ready_cnt=$(find "${KB_DATA_DIR}/sys_wal/archive_status" -name '*.ready' 2>/dev/null | wc -l | tr -d '[:space:]')
    if [[ "$arch_verdict" == "failing" && "${ready_cnt:-0}" -ge "${KB_FAIL_WAL_READY:-100}" ]]; then
      fail "WAL archiving: FAILING with ${ready_cnt} segments backlogged — disk will fill; run: kbdiag backup"
      _exit=$(_check_level "$_exit" 2)
      json_item "wal_archiving" "fail" "failing" "${arch_failed} failures, ${ready_cnt} pending"
    elif [[ "$arch_verdict" == "failing" ]]; then
      warn "WAL archiving: failing (${arch_failed} failures, last: ${arch_fail_time}) — run: kbdiag backup"
      _exit=$(_check_level "$_exit" 1)
      json_item "wal_archiving" "warn" "failing" "${arch_failed} failures"
    else
      ok "WAL archiving: ${arch_verdict}"
      json_item "wal_archiving" "ok" "${arch_verdict}" ""
    fi
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  return "$_exit"
}
