#!/usr/bin/env bash
# A session runs a long active query (pg_sleep).
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote <<'EOF'
hold long_query "select pg_sleep(3600);"
wait_for 10 "select exists(select 1 from sys_stat_activity where application_name='kbdiag_inj_long_query' and state='active')"
EOF
  ;;
down)
  remote <<'EOF'
release long_query
wait_for 10 "select not exists(select 1 from sys_stat_activity where application_name='kbdiag_inj_long_query')"
EOF
  ;;
*) usage ;;
esac
