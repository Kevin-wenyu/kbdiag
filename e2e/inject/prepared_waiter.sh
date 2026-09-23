#!/usr/bin/env bash
# A session waits for AccessExclusiveLock on kbdiag_inj_2pc, which the prepared transaction of
# prepared.sh holds a lock on: its blocker has no session (sys_blocking_pids reports 0).
# Needs prepared.sh up first. Run on the primary.
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote <<'EOF2'
hold prepared_waiter "begin;
lock table kbdiag_inj_2pc in access exclusive mode;"
wait_for 10 "select exists(select 1 from sys_locks l join sys_stat_activity a using(pid) where a.application_name='kbdiag_inj_prepared_waiter' and not l.granted)"
EOF2
  ;;
down)
  remote <<'EOF2'
release prepared_waiter
wait_for 10 "select not exists(select 1 from sys_stat_activity where application_name='kbdiag_inj_prepared_waiter')"
EOF2
  ;;
*) usage ;;
esac
