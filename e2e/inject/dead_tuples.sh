#!/usr/bin/env bash
# A table past its autovacuum threshold with autovacuum turned off for it
# (vacuum.table_disabled), as in the v0.2 stage 0 capture: 10000 rows, half
# deleted, analyzed so reltuples is 10000 (threshold 50 + 0.2 × 10000 = 2050).
# Primary only. Written in the cloud session: NOT RUN ON A VM YET (plan stage 13).
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote <<'EOF2'
sql "drop table if exists public.kbdiag_inj_dead"
sql "create table public.kbdiag_inj_dead(id int, v text) with (autovacuum_enabled = off)"
sql "insert into public.kbdiag_inj_dead select g, md5(g::text) from generate_series(1, 10000) g"
sql "analyze public.kbdiag_inj_dead"
sql "delete from public.kbdiag_inj_dead where id % 2 = 0"
# the statistics collector reports dead tuples with a short delay
wait_for 30 "select coalesce((select n_dead_tup >= 5000 from sys_stat_user_tables where relid = 'public.kbdiag_inj_dead'::regclass), false)"
EOF2
  ;;
down)
  remote <<'EOF2'
sql "drop table if exists public.kbdiag_inj_dead"
EOF2
  ;;
*) usage ;;
esac
