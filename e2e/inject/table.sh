#!/usr/bin/env bash
# The stage 0 test table for `kbdiag table`: 20000 rows, a quarter deleted,
# a primary key, a plain index and a TOAST table (a text column), analyzed.
# Primary only. Written in the cloud session: NOT RUN ON A VM YET (plan stage 13).
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
up)
  remote <<'EOF2'
sql "drop table if exists public.kbdiag_inj_tbl"
sql "create table public.kbdiag_inj_tbl(id int primary key, n int, v text)"
sql "create index kbdiag_inj_tbl_n on public.kbdiag_inj_tbl(n)"
sql "insert into public.kbdiag_inj_tbl select g, g % 100, md5(g::text) from generate_series(1, 20000) g"
sql "analyze public.kbdiag_inj_tbl"
sql "delete from public.kbdiag_inj_tbl where id % 4 = 0"
wait_for 30 "select coalesce((select n_dead_tup >= 5000 from sys_stat_user_tables where relid = 'public.kbdiag_inj_tbl'::regclass), false)"
EOF2
  ;;
down)
  remote <<'EOF2'
sql "drop table if exists public.kbdiag_inj_tbl"
EOF2
  ;;
*) usage ;;
esac
