#!/usr/bin/env bash
# One session holds AccessExclusiveLock on kbdiag_inj_lock; a second waits on it.
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote <<'EOF'
sql "create table if not exists kbdiag_inj_lock(i int)"
hold lock_holder "begin;
lock table kbdiag_inj_lock in access exclusive mode;"
wait_for 10 "select exists(select 1 from sys_locks l join sys_stat_activity a using(pid) where a.application_name='kbdiag_inj_lock_holder' and l.granted and l.mode='AccessExclusiveLock')"
hold lock_waiter "select count(*) from kbdiag_inj_lock;"
wait_for 10 "select exists(select 1 from sys_locks l join sys_stat_activity a using(pid) where a.application_name='kbdiag_inj_lock_waiter' and not l.granted)"
EOF
  ;;
down)
  remote <<'EOF'
release lock_waiter
release lock_holder
wait_for 10 "select not exists(select 1 from sys_stat_activity where application_name like 'kbdiag_inj_lock_%')"
sql "drop table if exists kbdiag_inj_lock"
EOF
  ;;
*) usage ;;
esac
