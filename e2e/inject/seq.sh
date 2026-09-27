#!/usr/bin/env bash
# An exhausted sequence (seq.exhausted): maxvalue 3, no cycle, called three
# times, so the next nextval fails. Primary only; only the test sequence is
# touched. Written in the cloud session: NOT RUN ON A VM YET (plan stage 13).
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote <<'EOF2'
sql "drop sequence if exists public.kbdiag_inj_seq"
sql "create sequence public.kbdiag_inj_seq as integer maxvalue 3 no cycle"
for _ in 1 2 3; do sql "select nextval('public.kbdiag_inj_seq')" >/dev/null; done
EOF2
  ;;
down)
  remote <<'EOF2'
sql "drop sequence if exists public.kbdiag_inj_seq"
EOF2
  ;;
*) usage ;;
esac
