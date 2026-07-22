# shellcheck shell=bash
cmd_cluster() {
  case "${1:-}" in
    ready|--failover-ready) _cluster_ready; return $? ;;
  esac

  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "cluster"
  hdr "Cluster status (repmgr)"

  if ! repmgr_available; then
    warn "repmgr not found — standalone instance"
    json_item "repmgr" "warn" "absent" "standalone instance"
    [[ "$OUTPUT_FMT" == "json" ]] && json_end
    return 0
  fi

  local raw
  raw=$("$REPMGR" -f "$KB_REPMGR_CONF" cluster show 2>&1)
  [[ "$OUTPUT_FMT" != "json" ]] && printf '%s\n' "$raw"

  # data rows only: " ID | Name | Role | Status | ... " — skip header/separator
  local total=0 down=0 id name role status upstream
  while IFS='|' read -r id name role status upstream _; do
    id="$(echo "$id" | tr -d '[:space:]')"
    [[ "$id" =~ ^[0-9]+$ ]] || continue
    name="$(echo "$name" | tr -d '[:space:]')"
    role="$(echo "$role" | tr -d '[:space:]')"
    status="$(echo "$status" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
    upstream="$(echo "$upstream" | tr -d '[:space:]')"
    total=$((total + 1))
    json_row id="$id" name="$name" role="$role" status="$status" upstream="$upstream"
    [[ "$status" == *running* ]] || down=$((down + 1))
  done <<< "$raw"

  local _exit=0
  if [[ "$total" -eq 0 ]]; then
    fail "Cluster: cannot read node list from repmgr"
    json_item "cluster" "fail" "0 nodes" "repmgr cluster show returned no rows"
    _exit=$(_check_level "$_exit" 2)
  elif [[ "$down" -eq 0 ]]; then
    ok "Cluster: $total node(s), all running"
    json_item "cluster" "ok" "$total nodes" ""
  else
    warn "Cluster: $down of $total node(s) not running"
    json_item "cluster" "warn" "$down of $total not running" ""
    _exit=$(_check_level "$_exit" 1)
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}

# ─── failover readiness ──────────────────────────────────────────────────────

# Query the repmgr metadata db. $1=sql, $2=host (empty = local socket)
_repmgr_q() {
  local host_opt=()
  [[ -n "${2:-}" ]] && host_opt=(-h "$2")
  timeout "$KB_QUERY_TIMEOUT" "$KSQL" "${host_opt[@]}" -p "$KB_PORT" \
    -U "$KB_REPMGR_USER" -d "$KB_REPMGR_DB" -AXtc "$1" 2>/dev/null </dev/null
}

# Read one value from repmgr.conf, stripped of quotes
_repconf_val() {
  sed -n "s/^[[:space:]]*$1[[:space:]]*=[[:space:]]*//p" "$KB_REPMGR_CONF" \
    | head -1 | tr -d "'\""
}

# esrep db runs in PG mode (t/f) while the user db may run in Oracle mode
# (true/false) — accept both
_is_true() {
  local v
  v=$(echo "$1" | tr -d '[:space:]')
  [[ "$v" == "t" || "$v" == "true" ]]
}

_cluster_ready() {
  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "cluster_ready"
  hdr "Failover readiness (repmgr)"
  local _exit=0

  if ! repmgr_available; then
    warn "repmgr not found — standalone instance, nothing to check"
    [[ "$OUTPUT_FMT" == "json" ]] && json_item "repmgr" "warn" "absent" "standalone instance" && json_end
    return 0
  fi

  # ── shared facts ───────────────────────────────────────────────────────────
  # node list: id|name|type|priority|active|host
  local nodes
  nodes=$(_repmgr_q "SELECT node_id||'|'||node_name||'|'||type||'|'||priority||'|'||active::text||'|'||
      substring(conninfo from 'host=([^ ]+)')
    FROM repmgr.nodes ORDER BY node_id;")
  if [[ -z "$nodes" ]]; then
    fail "Cannot read repmgr.nodes (db=$KB_REPMGR_DB user=$KB_REPMGR_USER)"
    json_item "repmgr_metadata" "fail" "unreachable" "cannot read repmgr.nodes"
    [[ "$OUTPUT_FMT" == "json" ]] && json_end
    return 2
  fi

  local total_cnt primary_cnt standby_cnt witness_cnt inactive
  total_cnt=$(echo "$nodes" | grep -c .)
  primary_cnt=$(echo "$nodes" | awk -F'|' '$3=="primary"' | grep -c .)
  standby_cnt=$(echo "$nodes" | awk -F'|' '$3=="standby"' | grep -c .)
  witness_cnt=$(echo "$nodes" | awk -F'|' '$3=="witness"' | grep -c .)
  inactive=$(echo "$nodes" | awk -F'|' '$5!="t" && $5!="true" {print $2}' | paste -sd, -)

  local local_role="standby"
  _is_true "$(ksql_q "SELECT pg_is_in_recovery()::text;")" || local_role="primary"

  # 1. topology: exactly one primary, all nodes active
  if [[ $primary_cnt -ne 1 ]]; then
    fail "Topology: $primary_cnt primaries registered (expected 1)"
    json_item "topology" "fail" "${primary_cnt} primaries" "expected exactly 1"
    _exit=$(_check_level "$_exit" 2)
  elif [[ -n "$inactive" ]]; then
    warn "Topology: inactive node(s): $inactive"
    json_item "topology" "warn" "$inactive" "registered but inactive"
    _exit=$(_check_level "$_exit" 1)
  else
    ok "Topology: 1 primary + $standby_cnt standby (all active)"
    json_item "topology" "ok" "1 primary + $standby_cnt standby" ""
  fi

  # 2. promotion candidate: at least one standby with priority > 0
  local candidates
  candidates=$(echo "$nodes" | awk -F'|' '$3=="standby" && $4>0' | grep -c .)
  if [[ $standby_cnt -eq 0 ]]; then
    fail "Promotion: no standby registered — nothing to fail over to"
    json_item "promotion_candidate" "fail" "0 standby" "no failover target"
    _exit=$(_check_level "$_exit" 2)
  elif [[ $candidates -eq 0 ]]; then
    fail "Promotion: all standbys have priority=0 — none can be promoted"
    json_item "promotion_candidate" "fail" "0 of $standby_cnt eligible" "all priority=0"
    _exit=$(_check_level "$_exit" 2)
  else
    ok "Promotion: $candidates of $standby_cnt standby(s) eligible (priority>0)"
    json_item "promotion_candidate" "ok" "$candidates of $standby_cnt" ""
  fi

  # 3. split-brain arbitration: 2-node cluster needs witness or trusted_servers
  local trusted
  trusted=$(_repconf_val trusted_servers)
  if [[ $witness_cnt -gt 0 ]]; then
    ok "Arbitration: witness node present"
    json_item "arbitration" "ok" "witness" ""
  elif [[ $total_cnt -eq 2 && -z "$trusted" ]]; then
    warn "Arbitration: 2-node cluster without witness or trusted_servers — split-brain risk"
    json_item "arbitration" "warn" "none" "2 nodes, no witness/trusted_servers"
    _exit=$(_check_level "$_exit" 1)
  else
    ok "Arbitration: trusted_servers=${trusted:-n/a (>2 nodes)}"
    json_item "arbitration" "ok" "${trusted:-multi-node}" ""
  fi

  # 4. repmgrd on every node: not running or paused = automatic failover is off
  # daemon status --csv: f1=id f2=name f3=role f5=repmgrd(1/0) f7=paused(1/0)
  local daemon_bad
  daemon_bad=$("$REPMGR" -f "$KB_REPMGR_CONF" daemon status --csv 2>/dev/null | \
    awk -F, '$5!=1 {print $2"(not running)"} $5==1 && $7!=0 {print $2"(paused)"}' | paste -sd, -)
  if [[ -z "$daemon_bad" ]]; then
    ok "repmgrd: running on all $total_cnt node(s), none paused"
    json_item "repmgrd" "ok" "$total_cnt running" ""
  else
    fail "repmgrd: $daemon_bad — automatic failover is DISABLED"
    json_item "repmgrd" "fail" "$daemon_bad" "automatic failover disabled"
    _exit=$(_check_level "$_exit" 2)
  fi

  # 5. failover mode in local repmgr.conf
  local fo_mode promote_cmd
  fo_mode=$(_repconf_val failover)
  promote_cmd=$(_repconf_val promote_command)
  if [[ -z "$promote_cmd" ]]; then
    fail "Failover mode: promote_command missing in $KB_REPMGR_CONF"
    json_item "failover_mode" "fail" "no promote_command" ""
    _exit=$(_check_level "$_exit" 2)
  elif [[ "$fo_mode" != "automatic" ]]; then
    warn "Failover mode: failover='$fo_mode' (manual intervention required)"
    json_item "failover_mode" "warn" "$fo_mode" "not automatic"
    _exit=$(_check_level "$_exit" 1)
  else
    ok "Failover mode: automatic, promote_command set"
    json_item "failover_mode" "ok" "automatic" ""
  fi

  # 6. standbys attached & streaming (queried on the primary)
  local primary_host="" streaming
  primary_host=$(echo "$nodes" | awk -F'|' '$3=="primary" {print $6; exit}')
  if [[ "$local_role" == "primary" ]]; then
    streaming=$(ksql_q "SELECT count(*) FROM sys_stat_replication WHERE state='streaming';" | tr -d '[:space:]')
  else
    streaming=$(_repmgr_q "SELECT count(*) FROM sys_stat_replication WHERE state='streaming';" "$primary_host" | tr -d '[:space:]')
  fi
  if [[ -z "$streaming" ]]; then
    fail "Standby attach: cannot query primary ($primary_host)"
    json_item "standby_attached" "fail" "primary unreachable" ""
    _exit=$(_check_level "$_exit" 2)
  elif [[ $streaming -lt $standby_cnt ]]; then
    fail "Standby attach: $streaming of $standby_cnt standby(s) streaming"
    json_item "standby_attached" "fail" "$streaming/$standby_cnt" "standby detached or not streaming"
    _exit=$(_check_level "$_exit" 2)
  else
    ok "Standby attach: $streaming of $standby_cnt standby(s) streaming"
    json_item "standby_attached" "ok" "$streaming/$standby_cnt" ""
  fi

  # 7. replication slots: inactive slot retains WAL and fills the disk
  local bad_slots
  bad_slots=$(ksql_q "SELECT slot_name FROM sys_replication_slots WHERE NOT active;" | paste -sd, -)
  if [[ -n "$bad_slots" ]]; then
    warn "Slots: inactive slot(s): $bad_slots — WAL retention risk"
    json_item "slots" "warn" "$bad_slots" "inactive, retains WAL"
    _exit=$(_check_level "$_exit" 1)
  else
    ok "Slots: all replication slots active"
    json_item "slots" "ok" "all active" ""
  fi

  # 8. archive backlog (shared thresholds with backup)
  local ready_cnt warn_thr="${KB_WARN_WAL_READY:-10}" fail_thr="${KB_FAIL_WAL_READY:-100}"
  ready_cnt=$(_pending_wal_count)
  if [[ -z "$ready_cnt" ]]; then
    ok "Archive backlog: n/a (archiving off)"
    json_item "archive_backlog" "ok" "n/a" "archiving off"
  elif (( ready_cnt >= fail_thr )); then
    fail "Archive backlog: $ready_cnt WAL pending (>= $fail_thr)"
    json_item "archive_backlog" "fail" "$ready_cnt" ">= $fail_thr"
    _exit=$(_check_level "$_exit" 2)
  elif (( ready_cnt >= warn_thr )); then
    warn "Archive backlog: $ready_cnt WAL pending (>= $warn_thr)"
    json_item "archive_backlog" "warn" "$ready_cnt" ">= $warn_thr"
    _exit=$(_check_level "$_exit" 1)
  else
    ok "Archive backlog: $ready_cnt WAL pending"
    json_item "archive_backlog" "ok" "$ready_cnt" ""
  fi

  # 9. each standby is promotable: reachable, hot_standby on, no apply delay,
  #    replay not paused
  local sb_name sb_type sb_host sb_row sb_hot sb_delay sb_paused
  while IFS='|' read -r _ sb_name sb_type _ _ sb_host; do
    [[ "$sb_type" == "standby" ]] || continue
    sb_row=$(_repmgr_q "SELECT current_setting('hot_standby')||'|'||current_setting('recovery_min_apply_delay')||'|'||pg_is_wal_replay_paused()::text;" "$sb_host")
    if [[ -z "$sb_row" ]]; then
      fail "Standby $sb_name ($sb_host): unreachable via SQL"
      json_item "standby_${sb_name}" "fail" "unreachable" ""
      _exit=$(_check_level "$_exit" 2)
      continue
    fi
    IFS='|' read -r sb_hot sb_delay sb_paused <<< "$sb_row"
    if [[ "$sb_hot" != "on" ]]; then
      fail "Standby $sb_name: hot_standby=$sb_hot"
      json_item "standby_${sb_name}" "fail" "hot_standby=$sb_hot" ""
      _exit=$(_check_level "$_exit" 2)
    elif _is_true "$sb_paused"; then
      fail "Standby $sb_name: WAL replay is PAUSED — promotion would stall"
      json_item "standby_${sb_name}" "fail" "replay paused" ""
      _exit=$(_check_level "$_exit" 2)
    elif [[ "$sb_delay" != "0" ]]; then
      warn "Standby $sb_name: recovery_min_apply_delay=$sb_delay delays promotion"
      json_item "standby_${sb_name}" "warn" "apply_delay=$sb_delay" ""
      _exit=$(_check_level "$_exit" 1)
    else
      ok "Standby $sb_name: promotable (hot_standby=on, no delay, replay active)"
      json_item "standby_${sb_name}" "ok" "promotable" ""
    fi
  done <<< "$nodes"

  # 10. VIP binding (local node only — no remote shell access by design)
  local vip vip_bound
  vip=$(_repconf_val virtual_ip)
  if [[ -n "$vip" ]]; then
    vip_bound=no
    { ip addr 2>/dev/null || /sbin/ip addr 2>/dev/null; } | grep -qw "$vip" && vip_bound=yes
    if [[ "$local_role" == "primary" && "$vip_bound" == "no" ]]; then
      fail "VIP: $vip not bound on this primary — clients cannot connect"
      json_item "vip" "fail" "$vip unbound" "local node is primary"
      _exit=$(_check_level "$_exit" 2)
    elif [[ "$local_role" == "standby" && "$vip_bound" == "yes" ]]; then
      fail "VIP: $vip bound on this standby — address conflict with primary"
      json_item "vip" "fail" "$vip bound on standby" ""
      _exit=$(_check_level "$_exit" 2)
    else
      ok "VIP: $vip correctly $([[ "$vip_bound" == "yes" ]] && echo bound || echo unbound) (local role: $local_role)"
      json_item "vip" "ok" "$vip $vip_bound on $local_role" ""
    fi
  else
    json_item "vip" "ok" "n/a" "no virtual_ip configured"
  fi

  # context, no verdict
  local sync_mode ev_cnt
  sync_mode=$(_repconf_val synchronous)
  ev_cnt=$(_repmgr_q "SELECT count(*) FROM repmgr.events
    WHERE event_timestamp > now() - interval '7 days'
      AND event IN ('standby_promote','standby_follow','repmgrd_failover_promote','child_node_disconnect');")
  info "Sync mode: ${sync_mode:-async}; failover/disconnect events last 7d: ${ev_cnt:-0}"
  info "Cross-check: repmgr node check / kbdiag replication / kbdiag conf diff"

  [[ "$OUTPUT_FMT" == "json" ]] && json_end
  return "$_exit"
}
