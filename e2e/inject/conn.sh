#!/usr/bin/env bash
# Fill every connection ordinary users may open (max_connections less the superuser
# reserve) with idle kbdiag_ro sessions over TCP. The server turns away the surplus, so
# the count settles at the limit however many other sessions come and go; superusers
# (kbdiag's own system connection) still get in through the reserve.
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote <<'EOF2'
usable="current_setting('max_connections')::int - current_setting('superuser_reserved_connections')::int"
n=$(sql "select $usable - (select sum(numbackends) from sys_stat_database)")
for _ in $(seq 1 $((n + 5))); do
  setsid bash -c 'exec sleep 3600 | ksql "host=127.0.0.1 port=54321 dbname=test user=kbdiag_ro password=kbdiag_ro_T3st application_name=kbdiag_inj_conn" -q' \
    </dev/null >/dev/null 2>&1 &
  echo $! >>"$PIDDIR/conn.pids"
done
wait_for 30 "select sum(numbackends) >= $usable from sys_stat_database"
EOF2
  ;;
down)
  remote <<'EOF2'
if [ -f "$PIDDIR/conn.pids" ]; then
  while read -r p; do kill -- "-$p" 2>/dev/null || true; done <"$PIDDIR/conn.pids"
  rm -f "$PIDDIR/conn.pids"
fi
sql "select count(pg_terminate_backend(pid)) from sys_stat_activity where application_name='kbdiag_inj_conn'" >/dev/null
wait_for 10 "select not exists(select 1 from sys_stat_activity where application_name='kbdiag_inj_conn')"
EOF2
  ;;
*) usage ;;
esac
