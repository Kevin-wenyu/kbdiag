# shellcheck shell=bash
cmd_remote() {
  local nodes_arg="${1:-}"; shift || true
  local rcmd="${1:-}"; shift || true
  local rargs=("$@")

  if [[ -z "$nodes_arg" || -z "$rcmd" ]]; then
    echo "Usage: kbdiag remote <node1,node2|--all> <command> [args]" >&2; return 1
  fi

  # Parse node list
  local nodes=()
  if [[ "$nodes_arg" == "--all" ]]; then
    # Try repmgr first
    if repmgr_available; then
      while IFS= read -r line; do
        local host; host=$(echo "$line" | grep -oE 'host=[^ ]+' | cut -d= -f2 || true)
        [[ -n "$host" ]] && nodes+=("$host")
      done < <("$REPMGR" -f "$KB_REPMGR_CONF" cluster show 2>/dev/null | grep -E 'primary|standby' || true)
    fi
    # Fallback: ~/.kbdiag/nodes.conf
    local conf="$HOME/.kbdiag/nodes.conf"
    if [[ ${#nodes[@]} -eq 0 && -f "$conf" ]]; then
      while IFS= read -r line; do
        [[ "$line" =~ ^[[:space:]]*# ]] && continue
        [[ -z "${line// }" ]] && continue
        nodes+=("$line")
      done < "$conf"
    fi
  else
    IFS=',' read -ra nodes <<< "$nodes_arg"
  fi

  if [[ ${#nodes[@]} -eq 0 ]]; then
    warn "No nodes found. Use repmgr or create ~/.kbdiag/nodes.conf"
    return 0
  fi

  [[ "$OUTPUT_FMT" == "json" ]] && json_begin "remote"

  # Run command on each node via SSH, capture once, render both the
  # per-node text block and the JSON item from the same result (D6).
  local ok_cnt=0 fail_cnt=0
  for node in "${nodes[@]}"; do
    local out rc=0
    out=$(ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 "$node" \
        "/tmp/kbdiag $rcmd${rargs[*]:+ ${rargs[*]}}" 2>/dev/null) || rc=$?
    if [[ "$rc" -eq 0 ]]; then
      ok_cnt=$((ok_cnt + 1))
      json_item "$node" "ok" "" ""
    else
      fail_cnt=$((fail_cnt + 1))
      json_item "$node" "fail" "" "connection failed or kbdiag not deployed"
    fi
    if [[ "$OUTPUT_FMT" != "json" ]]; then
      echo ""
      echo "=== [$node] ==="
      if [[ "$rc" -ne 0 ]]; then
        warn "[$node] connection failed or kbdiag not deployed"
      else
        printf '%s\n' "$out"
      fi
    fi
  done

  if [[ "$fail_cnt" -eq 0 ]]; then
    ok "Remote: $ok_cnt/${#nodes[@]} node(s) reached"
  else
    warn "Remote: $ok_cnt/${#nodes[@]} node(s) reached, $fail_cnt failed"
  fi
  [[ "$OUTPUT_FMT" == "json" ]] && json_end

  if [[ -n "$EXIT_CODE_MODE" && "$fail_cnt" -gt 0 ]]; then
    return 1
  fi
  return 0
}
