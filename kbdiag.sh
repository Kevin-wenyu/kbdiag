#!/usr/bin/env bash
# kbdiag — KingbaseES CLI diagnostic tool
set -euo pipefail

# ─── config ──────────────────────────────────────────────────────────────────
KB_BIN_DIR="${KB_BIN_DIR:-/home/kingbase/cluster/install/kingbase/bin}"
KB_DATA_DIR="${KB_DATA_DIR:-/home/kingbase/cluster/install/kingbase/data}"
KB_REPMGR_CONF="${KB_REPMGR_CONF:-/home/kingbase/cluster/install/kingbase/etc/repmgr.conf}"
KB_PORT="${KB_PORT:-54321}"
KB_SUPERUSER="${KB_SUPERUSER:-system}"
KB_DB="${KB_DB:-test}"

KSQL="$KB_BIN_DIR/ksql"
REPMGR="$KB_BIN_DIR/repmgr"

# ─── color ───────────────────────────────────────────────────────────────────
if [[ -t 1 ]]; then
  RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
  CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'
else
  RED=''; GREEN=''; YELLOW=''; CYAN=''; BOLD=''; RESET=''
fi

ok()   { echo -e "${GREEN}[OK]${RESET}    $*"; }
warn() { echo -e "${YELLOW}[WARN]${RESET}  $*"; }
fail() { echo -e "${RED}[FAIL]${RESET}  $*"; }
info() { echo -e "${CYAN}[INFO]${RESET}  $*"; }
hdr()  { echo -e "\n${BOLD}==> $*${RESET}"; }

# ─── helpers ─────────────────────────────────────────────────────────────────
require_kingbase_user() {
  if [[ "$(whoami)" != "kingbase" ]]; then
    echo "Error: run as kingbase user  (sudo -i -u kingbase)" >&2
    exit 1
  fi
}

ksql_q() {
  # ksql_q <sql>  — returns plain text, no headers/alignment
  "$KSQL" -p "$KB_PORT" -U "$KB_SUPERUSER" "$KB_DB" \
    -AXtc "$1" 2>/dev/null
}

repmgr_available() {
  [[ -x "$REPMGR" && -f "$KB_REPMGR_CONF" ]]
}

# ─── subcommands ─────────────────────────────────────────────────────────────

cmd_status() {
  hdr "Instance status"

  # process
  local pid
  pid=$(pgrep -U kingbase -f "kingbase.*-D $KB_DATA_DIR" | head -1 || true)
  if [[ -n "$pid" ]]; then
    ok "kingbase process running (pid $pid)"
  else
    fail "kingbase process not found"
    return 1
  fi

  # connectivity
  local ver
  if ver=$(ksql_q "SELECT version();" 2>&1); then
    ok "DB connectable — $ver"
  else
    fail "Cannot connect to DB on port $KB_PORT"
    return 1
  fi

  # role
  local is_standby
  is_standby=$(ksql_q "SELECT pg_is_in_recovery()::text;" | tr -d '[:space:]')
  if [[ "$is_standby" == "false" ]]; then
    ok "Role: PRIMARY"
  else
    ok "Role: STANDBY"
  fi

  # uptime
  local uptime
  uptime=$(ksql_q "SELECT date_trunc('second', now() - pg_postmaster_start_time())::text;")
  info "Uptime: $uptime"
}

cmd_cluster() {
  hdr "Cluster status (repmgr)"

  if ! repmgr_available; then
    warn "repmgr not found — standalone instance"
    return 0
  fi

  "$REPMGR" -f "$KB_REPMGR_CONF" cluster show
}

cmd_replication() {
  hdr "Replication"

  local is_standby
  is_standby=$(ksql_q "SELECT pg_is_in_recovery()::text;" | tr -d '[:space:]')

  if [[ "$is_standby" == "false" ]]; then
    # primary: show connected standbys
    info "Standby connections:"
    ksql_q "
      SELECT
        client_addr,
        state,
        sync_state,
        pg_size_pretty(sent_lsn - replay_lsn) AS replay_lag
      FROM sys_stat_replication;
    " | column -t -s '|' || true

    local cnt
    cnt=$(ksql_q "SELECT count(*) FROM sys_stat_replication;")
    if [[ "$cnt" -eq 0 ]]; then
      warn "No standbys connected"
    else
      ok "$cnt standby(s) connected"
    fi
  else
    # standby: show lag from primary
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

cmd_sessions() {
  hdr "Active sessions"

  ksql_q "
    SELECT
      pid,
      usename,
      application_name,
      client_addr,
      state,
      date_trunc('second', now() - query_start)::text AS duration,
      left(query, 60)                                 AS query
    FROM sys_stat_activity
    WHERE state <> 'idle'
      AND pid <> sys_backend_pid()
    ORDER BY query_start;
  " | column -t -s '|' || true

  local cnt
  cnt=$(ksql_q "SELECT count(*) FROM sys_stat_activity WHERE state <> 'idle' AND pid <> sys_backend_pid();")
  info "Non-idle sessions: $cnt"
}

cmd_locks() {
  hdr "Waiting locks"

  local cnt
  cnt=$(ksql_q "SELECT count(*) FROM sys_locks WHERE NOT granted;")

  if [[ "$cnt" -eq 0 ]]; then
    ok "No waiting locks"
    return 0
  fi

  warn "$cnt waiting lock(s)"
  ksql_q "
    SELECT
      w.pid        AS wait_pid,
      w.usename    AS wait_user,
      w.query      AS wait_query,
      b.pid        AS block_pid,
      b.usename    AS block_user,
      b.query      AS block_query
    FROM sys_stat_activity w
    JOIN sys_locks lw ON lw.pid = w.pid AND NOT lw.granted
    JOIN sys_locks lb ON lb.locktype = lw.locktype
                      AND lb.relation IS NOT DISTINCT FROM lw.relation
                      AND lb.granted
    JOIN sys_stat_activity b ON b.pid = lb.pid
    LIMIT 20;
  " | column -t -s '|' || true
}

cmd_all() {
  cmd_status
  cmd_cluster
  cmd_replication
  cmd_sessions
  cmd_locks
}

usage() {
  cat <<EOF
Usage: kbdiag <command>

Commands:
  status       Instance process, connectivity, role, uptime
  cluster      Repmgr cluster topology (skipped if standalone)
  replication  Standby connections (primary) or lag (standby)
  sessions     Non-idle sessions
  locks        Waiting lock chains
  all          Run all checks above

Environment overrides:
  KB_BIN_DIR        binary directory  (default: /home/kingbase/cluster/install/kingbase/bin)
  KB_DATA_DIR       data directory    (default: /home/kingbase/cluster/install/kingbase/data)
  KB_REPMGR_CONF    repmgr.conf path  (default: /home/kingbase/cluster/install/kingbase/etc/repmgr.conf)
  KB_PORT           port              (default: 54321)
  KB_SUPERUSER      DB superuser      (default: system)
  KB_DB             database          (default: test)
EOF
}

# ─── dispatch ────────────────────────────────────────────────────────────────
require_kingbase_user

case "${1:-}" in
  status)      cmd_status      ;;
  cluster)     cmd_cluster     ;;
  replication) cmd_replication ;;
  sessions)    cmd_sessions    ;;
  locks)       cmd_locks       ;;
  all)         cmd_all         ;;
  -h|--help|help|"") usage     ;;
  *)
    echo "Unknown command: $1" >&2
    usage >&2
    exit 1
    ;;
esac
