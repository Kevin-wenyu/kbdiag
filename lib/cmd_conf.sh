# shellcheck shell=bash
cmd_conf() {
  local subcmd="${1:-}"
  hdr "Configuration"
  local _exit=0

  if [[ "$subcmd" == "diff" ]]; then
    # Compare this node's parameters with the peer node
    local peer="${KB_PEER_HOST:-}"
    if [[ -z "$peer" ]] && repmgr_available; then
      # Infer peer from repmgr
      peer=$("$REPMGR" -f "$KB_REPMGR_CONF" cluster show 2>/dev/null | \
             awk '/standby|primary/ && !/\*/ {match($0,"host=([^ ]+)",a); if(a[1]) print a[1]; exit}' || true)
    fi
    if [[ -z "$peer" ]]; then
      warn "Config drift: cannot determine peer node (set KB_PEER_HOST=<ip> to enable diff)"
      return 0
    fi
    local local_params peer_params drift drift_cnt
    local_params=$(ksql_q "SELECT name||'='||setting FROM sys_settings WHERE setting<>boot_val ORDER BY name;")
    peer_params=$(ssh -o StrictHostKeyChecking=no "$peer" \
      "sudo -i -u kingbase /home/kingbase/cluster/install/kingbase/bin/ksql -p 54321 -U system test -AXtc \
      'SELECT name||chr(61)||setting FROM sys_settings WHERE setting<>boot_val ORDER BY name;'" 2>/dev/null) || true
    if [[ -z "$peer_params" ]]; then
      warn "Config drift: peer $peer unreachable over ssh — cannot compare"
      return 0
    fi
    drift=$(diff <(echo "$local_params") <(echo "$peer_params") || true)
    drift_cnt=$(grep -c '^[<>]' <<< "$drift" 2>/dev/null || echo 0)

    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_begin "conf_diff"
      if [[ "${drift_cnt:-0}" -gt 0 ]]; then
        json_item "config_drift" "warn" "$drift_cnt" "differing non-default parameter(s) vs peer $peer"
      else
        json_item "config_drift" "ok" "0" "vs peer $peer"
      fi
      while read -r line; do
        [[ "$line" =~ ^[\<\>] ]] || continue
        json_row "side=$([[ "${line:0:1}" == "<" ]] && echo local || echo peer)" \
          "entry=${line:2}"
      done <<< "$drift"
      json_end
      [[ -n "$EXIT_CODE_MODE" && "${drift_cnt:-0}" -gt 0 ]] && return 1
      return 0
    fi

    if [[ "${drift_cnt:-0}" -gt 0 ]]; then
      warn "Config drift: $drift_cnt differing non-default parameter(s) vs peer $peer"
      [[ -z "$QUIET" ]] && { echo; echo "$drift" | head -40; }
      [[ -n "$EXIT_CODE_MODE" ]] && return 1
    else
      ok "Config drift: none (non-default parameters identical to peer $peer)"
    fi
    return 0
  fi

  # conf's job is cross-node comparison (diff) plus the restart-pending signal;
  # the full non-default parameter list lives in `kbdiag params` — don't duplicate it here.
  local pending restart_cnt
  pending=$(ksql_q "
    SELECT name, setting, unit, boot_val
    FROM sys_settings WHERE pending_restart = true ORDER BY name;")
  restart_cnt=$(awk 'NF { c++ } END { print c+0 }' <<< "$pending")
  [[ "$restart_cnt" -gt 0 ]] && _exit=1

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_begin "conf"
    if [[ "$restart_cnt" -gt 0 ]]; then
      json_item "pending_restart" "warn" "$restart_cnt" "parameter(s) need a restart to take effect"
    else
      json_item "pending_restart" "ok" "0" ""
    fi
    while IFS='|' read -r name setting unit boot; do
      [[ -z "${name// /}" ]] && continue
      json_row "name=${name// /}" "setting=${setting// /}" "unit=${unit// /}" "boot_val=${boot// /}"
    done <<< "$pending"
    json_end
    [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
    return 0
  fi

  if [[ "$restart_cnt" -gt 0 ]]; then
    warn "Pending restart: $restart_cnt parameter(s) changed but not yet effective"
    if [[ -z "$QUIET" ]]; then
      echo
      { echo "NAME|SETTING|UNIT|BOOT_VAL"
        awk -F'|' 'NF' <<< "$pending"
      } | column -t -s '|' || true
    fi
  else
    ok "Pending restart: 0 parameters waiting"
  fi
  info "Non-default parameter list: kbdiag params"
  info "Cross-node comparison:      kbdiag conf diff"

  [[ -n "$EXIT_CODE_MODE" ]] && return "$_exit"
  return 0
}
