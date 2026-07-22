# shellcheck shell=bash
cmd_replication() {
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "replication"
  hdr "Replication"

  local is_standby
  is_standby=$(ksql_q "SELECT pg_is_in_recovery()::text;" | tr -d '[:space:]')

  local _exit=0

  if [[ "$is_standby" == "false" ]]; then
    json_item "role" "ok" "primary" ""
    info "Standby connections:"

    local rows
    rows=$(ksql_q "
      SELECT coalesce(client_addr::text, 'local'), state, sync_state,
             pg_size_pretty(sent_lsn - replay_lsn)
      FROM sys_stat_replication;")

    if [[ -n "$VERBOSE" && -n "$rows" ]]; then
      { echo "client_addr|state|sync_state|replay_lag"; printf '%s\n' "$rows"; } | column -t -s '|'
    fi

    local cnt=0 addr state sync lag
    while IFS='|' read -r addr state sync lag; do
      [[ -z "$addr" ]] && continue
      cnt=$((cnt + 1))
      json_row "client_addr=$addr" "state=$state" "sync_state=$sync" "replay_lag=$lag"
    done <<< "$rows"

    if [[ "$cnt" -eq 0 ]]; then
      warn "No standbys connected"
      json_item "standbys" "warn" "0" "no standbys connected"
      _exit=$(_check_level "$_exit" 1)
    else
      ok "$cnt standby(s) connected"
      json_item "standbys" "ok" "$cnt" ""
    fi
  else
    json_item "role" "ok" "standby" ""

    local wal_status
    wal_status=$(ksql_q "SELECT status FROM sys_stat_wal_receiver LIMIT 1;" | tr -d '[:space:]')
    if [[ -z "$wal_status" ]]; then
      warn "WAL receiver not running"
      json_item "wal_receiver" "warn" "down" "WAL receiver not running"
      _exit=$(_check_level "$_exit" 1)
    else
      ok "WAL receiver: $wal_status"
      json_item "wal_receiver" "ok" "$wal_status" ""
    fi

    local replay_delay receive_lsn replay_lsn
    replay_delay=$(ksql_q "SELECT date_trunc('second', now() - pg_last_xact_replay_timestamp())::text;" | tr -d '[:space:]')
    receive_lsn=$(ksql_q "SELECT pg_last_wal_receive_lsn()::text;" | tr -d '[:space:]')
    replay_lsn=$(ksql_q "SELECT pg_last_wal_replay_lsn()::text;" | tr -d '[:space:]')
    info "Replay delay : $replay_delay"
    info "Receive LSN  : $receive_lsn"
    info "Replay  LSN  : $replay_lsn"
    json_item "replay_delay" "ok" "$replay_delay" ""
    json_item "receive_lsn" "ok" "$receive_lsn" ""
    json_item "replay_lsn" "ok" "$replay_lsn" ""
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}
