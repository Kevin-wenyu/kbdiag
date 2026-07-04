# shellcheck shell=bash
cmd_conf() {
  local subcmd="${1:-}"
  hdr "Configuration"

  if [[ "$subcmd" == "diff" ]]; then
    # Compare this node's parameters with the peer node
    local peer="${KB_PEER_HOST:-}"
    if [[ -z "$peer" ]]; then
      # Infer peer from repmgr
      peer=$(repmgr_available && "$REPMGR" -f "$KB_REPMGR_CONF" cluster show 2>/dev/null | \
             awk '/standby|primary/ && !/\*/ {match($0,"host=([^ ]+)",a); if(a[1]) print a[1]; exit}' || true)
    fi
    if [[ -z "$peer" ]]; then
      warn "Cannot determine peer node. Set KB_PEER_HOST=<ip> to enable diff."
      return 0
    fi
    info "Comparing parameters with peer: $peer"
    local local_params peer_params
    local_params=$(ksql_q "SELECT name||'='||setting FROM sys_settings WHERE setting<>boot_val ORDER BY name;")
    peer_params=$(ssh -o StrictHostKeyChecking=no "$peer" \
      "sudo -i -u kingbase /home/kingbase/cluster/install/kingbase/bin/ksql -p 54321 -U system test -AXtc \
      'SELECT name||chr(61)||setting FROM sys_settings WHERE setting<>boot_val ORDER BY name;'" 2>/dev/null) || true
    diff <(echo "$local_params") <(echo "$peer_params") | head -40 || true
    return 0
  fi

  # conf's job is cross-node comparison (diff) plus the restart-pending signal;
  # the full non-default parameter list lives in `kbdiag params` — don't duplicate it here.
  local restart_cnt
  restart_cnt=$(ksql_q "SELECT count(*) FROM sys_settings WHERE pending_restart=true;" | tr -d '[:space:]')
  if [[ "${restart_cnt:-0}" -gt 0 ]]; then
    warn "$restart_cnt parameter(s) pending restart"
    ksql_qh "
      SELECT name, setting, unit, boot_val
      FROM sys_settings WHERE pending_restart=true ORDER BY name;" | column -t -s '|' || true
  else
    ok "No restart-pending parameters"
  fi
  info "Non-default parameter list: kbdiag params"
  info "Cross-node comparison:      kbdiag conf diff"
}
