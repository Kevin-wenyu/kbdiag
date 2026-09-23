#!/usr/bin/env bash
# A session turns track_activities off for itself (as ALTER ROLE ... SET would
# for a role) and sits in an open transaction. Others see state='disabled',
# while kbdiag's own connection still has track_activities on.
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote <<'EOF'
hold untracked "set track_activities = off;
begin;
select 1;"
wait_for 15 "select exists(select 1 from sys_stat_activity where application_name='kbdiag_inj_untracked' and state='disabled')"
EOF
  ;;
down)
  remote <<'EOF'
release untracked
wait_for 10 "select not exists(select 1 from sys_stat_activity where application_name='kbdiag_inj_untracked')"
EOF
  ;;
*) usage ;;
esac
