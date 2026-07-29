# shellcheck shell=bash
# cmd_instances.sh — inventory of kingbase postmaster processes on this host.
# OPS tier: unlike core.sh's auto-detect / cmd_status.sh (which only ever
# target the current OS user's own instance), this scans *all* OS users —
# its whole purpose is disambiguating multi-instance hosts, including
# deployments that isolate instances behind separate system accounts.

# Port lookup: prefer the actual LISTEN socket for $1, looked up against the
# single `ss -tlnp` snapshot passed in $3 (works without root for same-OS-user
# processes, and is correct even when kingbase.conf leaves the default port
# commented out — the common case). Falls back to parsing kingbase.conf/
# postgresql.conf under $2 for processes ss can't see into (e.g. owned by a
# different OS user), then "?" if neither yields an answer.
_instances_port_for() {
  local pid="$1" data_dir="$2" ss_snapshot="$3" conf p
  p=$(awk -v pid="$pid" '
    $0 ~ ("pid=" pid ",") { n = split($4, a, ":"); print a[n]; exit }' <<< "$ss_snapshot")
  if [[ -z "$p" ]]; then
    for conf in "$data_dir/kingbase.conf" "$data_dir/postgresql.conf"; do
      [[ -r "$conf" ]] || continue
      p=$(grep -E '^[[:space:]]*port[[:space:]]*=' "$conf" 2>/dev/null | tail -1 \
        | sed -E 's/^[[:space:]]*port[[:space:]]*=[[:space:]]*([0-9]+).*/\1/')
      [[ -n "$p" ]] && break
    done
  fi
  echo "${p:-?}"
}

cmd_instances() {
  hdr "Instances"

  local pids
  pids=$(pgrep -f "/kingbase -D " 2>/dev/null || true)

  if [[ -z "$pids" ]]; then
    fail "No kingbase instances detected on this host"
    if [[ "$OUTPUT_FMT" == "json" ]]; then
      json_begin "instances"
      json_item "instance_count" "fail" "0" ""
      json_end
    fi
    return 1
  fi

  local ss_snapshot
  ss_snapshot=$(ss -tlnp 2>/dev/null || true)

  local pid user data_dir bin_dir port exe rows cnt
  rows="" cnt=0
  while IFS= read -r pid; do
    [[ -z "$pid" ]] && continue
    user=$(ps -o user= -p "$pid" 2>/dev/null | tr -d '[:space:]')
    data_dir=$(ps -o args= -p "$pid" 2>/dev/null | sed -n 's/.*-D[[:space:]]\+\([^[:space:]]*\).*/\1/p')
    exe=$(readlink -f "/proc/$pid/exe" 2>/dev/null || true)
    bin_dir="${exe:+$(dirname "$exe")}"
    port=$(_instances_port_for "$pid" "$data_dir" "$ss_snapshot")
    rows="${rows}${pid}|${port}|${data_dir:-?}|${bin_dir:-?}|${user:-?}"$'\n'
    cnt=$((cnt + 1))
  done <<< "$pids"

  if [[ "$OUTPUT_FMT" == "json" ]]; then
    json_begin "instances"
    json_item "instance_count" "ok" "$cnt" ""
    while IFS='|' read -r pid port data_dir bin_dir user; do
      [[ -z "$pid" ]] && continue
      json_row "pid=$pid" "port=$port" "data_dir=$data_dir" "bin_dir=$bin_dir" "user=$user"
    done <<< "$rows"
    json_end
    return 0
  fi

  if [[ "$cnt" -eq 1 ]]; then
    ok "1 kingbase instance detected"
  else
    ok "$cnt kingbase instances detected — set KB_PORT/KB_DATA_DIR (and KB_BIN_DIR if non-standard) to target a specific one"
  fi

  if [[ -z "$QUIET" ]]; then
    echo
    { echo "PID|PORT|DATA_DIR|BIN_DIR|USER"; printf '%s' "$rows"; } | column -t -s '|'
  fi

  return 0
}
