cmd_replication() {
  hdr "Replication"

  local is_standby
  is_standby=$(ksql_q "SELECT pg_is_in_recovery()::text;" | tr -d '[:space:]')

  if [[ "$is_standby" == "false" ]]; then
    info "Standby connections:"
    if [[ -n "$VERBOSE" ]]; then
      ksql_qh "
        SELECT client_addr, state, sync_state,
               pg_size_pretty(sent_lsn - replay_lsn) AS replay_lag
        FROM sys_stat_replication;
      " | column -t -s '|' || true
    fi

    local cnt
    cnt=$(ksql_q "SELECT count(*) FROM sys_stat_replication;" | tr -d '[:space:]')
    if [[ "$cnt" -eq 0 ]]; then
      warn "No standbys connected"
    else
      ok "$cnt standby(s) connected"
    fi
  else
    local wal_status
    wal_status=$(ksql_q "SELECT status FROM sys_stat_wal_receiver LIMIT 1;" | tr -d '[:space:]')
    if [[ -z "$wal_status" ]]; then
      warn "WAL receiver not running"
    else
      ok "WAL receiver: $wal_status"
    fi

    local replay_delay receive_lsn replay_lsn
    replay_delay=$(ksql_q "SELECT date_trunc('second', now() - pg_last_xact_replay_timestamp())::text;" | tr -d '[:space:]')
    receive_lsn=$(ksql_q "SELECT pg_last_wal_receive_lsn()::text;" | tr -d '[:space:]')
    replay_lsn=$(ksql_q "SELECT pg_last_wal_replay_lsn()::text;" | tr -d '[:space:]')
    info "Replay delay : $replay_delay"
    info "Receive LSN  : $receive_lsn"
    info "Replay  LSN  : $replay_lsn"
  fi
}
