#!/usr/bin/env bash
# A session sits idle in transaction; on a primary it also holds an xid
# (a standby cannot assign one: txid_current() fails during recovery).
# The sleeps put about 1s between backend_start, xact_start, query_start and
# state_change, so an age computed from the wrong column is caught.
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote <<'EOF'
if [ "$(sql "select sys_is_in_recovery()")" = t ]; then
  hold idle_txn "select pg_sleep(1);
begin;
select pg_sleep(1);
select 1 as kbdiag_last, pg_sleep(1);"
  wait_for 15 "select exists(select 1 from sys_stat_activity where application_name='kbdiag_inj_idle_txn' and state='idle in transaction' and query like '%kbdiag_last%')"
else
  hold idle_txn "select pg_sleep(1);
begin;
select pg_sleep(1);
select txid_current() as kbdiag_last, pg_sleep(1);"
  wait_for 15 "select exists(select 1 from sys_stat_activity where application_name='kbdiag_inj_idle_txn' and state='idle in transaction' and query like '%kbdiag_last%')"
fi
EOF
  ;;
down)
  remote <<'EOF'
release idle_txn
wait_for 10 "select not exists(select 1 from sys_stat_activity where application_name='kbdiag_inj_idle_txn')"
EOF
  ;;
*) usage ;;
esac
