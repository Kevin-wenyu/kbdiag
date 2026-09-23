#!/usr/bin/env bash
# A prepared (2PC) transaction kbdiag_inj_2pc is left uncommitted. Run on the primary.
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote <<'EOF'
sql "create table if not exists kbdiag_inj_2pc(i int)"
printf '%s\n' "begin;" "insert into kbdiag_inj_2pc values (1);" "prepare transaction 'kbdiag_inj_2pc';" |
  ksql -d test -U system -p 54321 -v ON_ERROR_STOP=1 -q >/dev/null
wait_for 10 "select exists(select 1 from sys_prepared_xacts where gid='kbdiag_inj_2pc')"
EOF
  ;;
down)
  remote <<'EOF'
if [ "$(sql "select count(*) from sys_prepared_xacts where gid='kbdiag_inj_2pc'")" != 0 ]; then
  sql "rollback prepared 'kbdiag_inj_2pc'"
fi
sql "drop table if exists kbdiag_inj_2pc"
EOF
  ;;
*) usage ;;
esac
