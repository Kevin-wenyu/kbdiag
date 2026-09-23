#!/usr/bin/env bash
# Make the standby's physical slot inactive while keeping its xmin, the way a dead standby does.
# The standby walreceiver is paused (SIGSTOP) rather than stopping the instance: kbha restarts a
# stopped instance, and cutting the network could trigger a repmgr failover.
# The slot turns inactive after wal_sender_timeout (30s on the lab), so `up` takes ~30-40s.
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote "$STANDBY_NODE" <<'EOF'
pid=$(sql "select pid from sys_stat_wal_receiver")
[ -n "$pid" ] || { echo "no walreceiver on standby" >&2; exit 1; }
kill -STOP "$pid"
echo "$pid" >"$PIDDIR/walreceiver.pid"
EOF
  remote <<'EOF'
wait_for 90 "select exists(select 1 from sys_replication_slots where slot_type='physical' and not active and xmin is not null)"
EOF
  ;;
down)
  remote "$STANDBY_NODE" <<'EOF'
if [ -f "$PIDDIR/walreceiver.pid" ]; then
  kill -CONT "$(cat "$PIDDIR/walreceiver.pid")" 2>/dev/null || true
  rm -f "$PIDDIR/walreceiver.pid"
fi
EOF
  remote <<'EOF'
wait_for 90 "select not exists(select 1 from sys_replication_slots where slot_type='physical' and not active)"
EOF
  ;;
*) usage ;;
esac
