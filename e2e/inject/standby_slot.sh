#!/usr/bin/env bash
# An inactive physical slot kbdiag_inj_slot that reserves WAL, created on a standby: retained WAL
# must then be measured from the replay LSN (sys_current_wal_lsn() fails during recovery).
# Run on the standby.
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote <<'EOF2'
[ "$(sql "select sys_is_in_recovery()")" = t ] || { echo "standby_slot.sh runs on a standby" >&2; exit 1; }
sql "select sys_create_physical_replication_slot('kbdiag_inj_slot', true)" >/dev/null
wait_for 10 "select exists(select 1 from sys_replication_slots where slot_name='kbdiag_inj_slot' and restart_lsn is not null)"
EOF2
  ;;
down)
  remote <<'EOF2'
if [ "$(sql "select count(*) from sys_replication_slots where slot_name='kbdiag_inj_slot'")" != 0 ]; then
  sql "select sys_drop_replication_slot('kbdiag_inj_slot')" >/dev/null
fi
EOF2
  ;;
*) usage ;;
esac
