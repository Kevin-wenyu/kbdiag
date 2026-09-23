#!/usr/bin/env bash
# One session holds a lock; a second waits on it. On a primary the lock is AccessExclusiveLock on
# kbdiag_inj_lock; a standby cannot create a table or take that lock, so there both sessions take
# advisory lock 424242 instead.
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote <<'EOF'
if [ "$(sql "select sys_is_in_recovery()")" = t ]; then
  hold lock_holder "select pg_advisory_lock(424242);"
  wait_for 10 "select exists(select 1 from sys_locks l join sys_stat_activity a using(pid) where a.application_name='kbdiag_inj_lock_holder' and l.granted and l.locktype='advisory')"
  hold lock_waiter "select pg_advisory_lock(424242);"
else
  sql "create table if not exists kbdiag_inj_lock(i int)"
  hold lock_holder "begin;
lock table kbdiag_inj_lock in access exclusive mode;"
  wait_for 10 "select exists(select 1 from sys_locks l join sys_stat_activity a using(pid) where a.application_name='kbdiag_inj_lock_holder' and l.granted and l.mode='AccessExclusiveLock')"
  hold lock_waiter "select count(*) from kbdiag_inj_lock;"
fi
wait_for 10 "select exists(select 1 from sys_locks l join sys_stat_activity a using(pid) where a.application_name='kbdiag_inj_lock_waiter' and not l.granted)"
EOF
  ;;
down)
  remote <<'EOF'
release lock_waiter
release lock_holder
wait_for 10 "select not exists(select 1 from sys_stat_activity where application_name like 'kbdiag_inj_lock_%')"
if [ "$(sql "select sys_is_in_recovery()")" = f ]; then
  sql "drop table if exists kbdiag_inj_lock"
fi
EOF
  ;;
*) usage ;;
esac
