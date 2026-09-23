#!/usr/bin/env bash
# Turn track_activities off instance-wide (ALTER SYSTEM + reload).
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote <<'EOF'
sql "alter system set track_activities = off"
sql "select pg_reload_conf()" >/dev/null
wait_for 10 "select current_setting('track_activities') = 'off'"
EOF
  ;;
down)
  remote <<'EOF'
sql "alter system reset track_activities"
sql "select pg_reload_conf()" >/dev/null
wait_for 10 "select current_setting('track_activities') = 'on'"
EOF
  ;;
*) usage ;;
esac
