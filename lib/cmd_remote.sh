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

  # Run command on each node via SSH
  for node in "${nodes[@]}"; do
    echo ""
    echo "=== [$node] ==="
    if ! ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 "$node" \
        "/tmp/kbdiag $rcmd${rargs[*]:+ ${rargs[*]}}" 2>/dev/null; then
      warn "[$node] connection failed or kbdiag not deployed"
    fi
  done
}
