#!/usr/bin/env bash
# A parameter changed and reloaded that only takes effect after a restart
# (params.pending_restart): ALTER SYSTEM on a postmaster-level parameter,
# then a reload. max_files_per_process is raised by one: it has no standby
# constraint and nothing else notices. Nothing is restarted.
# In a PG12 kernel a reload clears the pending flag only when the file's
# value equals the running one; removing the entry does not. So `down` first
# writes the running value back, reloads and waits for the flag to clear,
# then removes the entry. Primary only; refuses to touch an existing entry.
# Run on kes-node1 2026-09-28: up sets the flag, down clears it.
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote <<'EOF2'
[ "$(sql "select sys_is_in_recovery()")" = "f" ] || { echo "not a primary" >&2; exit 1; }
[ "$(sql "select count(*) from sys_file_settings where name = 'max_files_per_process' and sourcefile like '%kingbase.auto.conf'")" = "0" ] ||
  { echo "max_files_per_process is already set with ALTER SYSTEM; not touching it" >&2; exit 1; }
cur=$(sql "select setting from sys_settings where name = 'max_files_per_process'")
echo "$cur" >"$PIDDIR/max_files_per_process"
sql "alter system set max_files_per_process = $((cur + 1))"
sql "select sys_reload_conf()" >/dev/null
wait_for 30 "select pending_restart from sys_settings where name = 'max_files_per_process'"
EOF2
  ;;
down)
  remote <<'EOF2'
if [ -f "$PIDDIR/max_files_per_process" ]; then
  sql "alter system set max_files_per_process = $(cat "$PIDDIR/max_files_per_process")"
  sql "select sys_reload_conf()" >/dev/null
  wait_for 30 "select not pending_restart from sys_settings where name = 'max_files_per_process'"
  sql "alter system reset max_files_per_process"
  sql "select sys_reload_conf()" >/dev/null
  rm -f "$PIDDIR/max_files_per_process"
fi
EOF2
  ;;
*) usage ;;
esac
