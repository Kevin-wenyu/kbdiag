_check_level() {
  # $1=current_exit $2=new_level(1=warn,2=fail)
  [[ $2 -gt $1 ]] && echo $2 || echo $1
}

cmd_check() {
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
    _exit=$(_check_level $_exit 2)
  elif [[ $conn_pct -ge $KB_WARN_CONN ]]; then
    warn "Connections: ${conn_pct}% (${conn_used}/${max_conn}) >= WARN threshold ${KB_WARN_CONN}%"
    _exit=$(_check_level $_exit 1)
  else
    ok "Connections: ${conn_pct}% (${conn_used}/${max_conn})"
  fi
  if [[ -n "$VERBOSE" ]]; then
    ksql_q "SELECT usename, count(*) FROM sys_stat_activity GROUP BY usename ORDER BY 2 DESC LIMIT 10;" \
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
      _exit=$(_check_level $_exit 2)
    elif [[ $lag_sec -ge $KB_WARN_LAG ]]; then
      warn "Replication lag: ${lag_sec}s >= WARN threshold ${KB_WARN_LAG}s"
      _exit=$(_check_level $_exit 1)
    else
      ok "Replication lag: ${lag_sec}s"
    fi
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
    _exit=$(_check_level $_exit 2)
  elif [[ ${long_warn:-0} -gt 0 ]]; then
    warn "Long transactions: ${long_warn} exceeding WARN threshold ${KB_WARN_TXN}s"
    _exit=$(_check_level $_exit 1)
  else
    ok "Long transactions: none"
  fi
  if [[ -n "$VERBOSE" && ${long_warn:-0} -gt 0 ]]; then
    ksql_q "
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
    _exit=$(_check_level $_exit 1)
    if [[ -n "$VERBOSE" ]]; then
      ksql_q "
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
  fi

  # 5. 归档失败
  local arch_failed arch_status
  arch_failed=$(ksql_q "SELECT failed_count FROM sys_stat_archiver;" | tr -d '[:space:]')
  arch_status=$(ksql_q "SELECT 'archived='||archived_count||' failed='||failed_count||' last_failed='||coalesce(last_failed_wal,'none') FROM sys_stat_archiver;")
  if [[ ${arch_failed:-0} -gt 0 ]]; then
    warn "Archiver: ${arch_failed} failed file(s)"
    _exit=$(_check_level $_exit 1)
    [[ -n "$VERBOSE" ]] && info "$arch_status"
  else
    ok "Archiver: no failures"
  fi

  # 6. autovacuum 积压
  local vac_cnt
  vac_cnt=$(ksql_q "
    SELECT count(*) FROM sys_stat_user_tables
    WHERE n_dead_tup > ${KB_WARN_DEAD_TUPLES};" | tr -d '[:space:]')
  if [[ ${vac_cnt:-0} -gt 0 ]]; then
    warn "Autovacuum backlog: ${vac_cnt} table(s) with dead tuples > ${KB_WARN_DEAD_TUPLES}"
    _exit=$(_check_level $_exit 1)
    if [[ -n "$VERBOSE" ]]; then
      ksql_q "
        SELECT schemaname, relname, n_dead_tup, last_autovacuum
        FROM sys_stat_user_tables
        WHERE n_dead_tup > ${KB_WARN_DEAD_TUPLES}
        ORDER BY n_dead_tup DESC LIMIT 10;" \
        | column -t -s '|' || true
    fi
  else
    ok "Autovacuum backlog: none"
  fi

  return $_exit
}
