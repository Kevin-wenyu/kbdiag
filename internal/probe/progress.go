package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// progressListSQL is progress.list: the four progress views of KES V8R6 in
// one set of columns. There is no ANALYZE or base backup view (stage 0
// capture progress_*_views). CREATE INDEX counts blocks while scanning and
// tuples while loading, so whichever has a total is used. The checkpoint
// view is KES's own: only its column names are captured, so its values are
// cast. Running time is from the query's start (the checkpoint's own
// start_time for CHECKPOINT). kbdiag_ro may query every view (stage 0);
// a NULL phase is masked. Not run on a VM yet.
const progressListSQL = `
select * from (
  select p.pid,
         case when a.backend_type = 'autovacuum worker' then 'autovacuum' else 'VACUUM' end as command,
         p.datname::text,
         case when p.datname = current_database() then p.relid::regclass::text else p.relid::text end,
         p.phase::text, p.heap_blks_scanned::bigint, p.heap_blks_total::bigint, 'blocks'::text,
         round(extract(epoch from now() - a.query_start)::numeric, 1)::float8 as running,
         null::bigint
  from sys_stat_progress_vacuum p left join sys_stat_activity a on a.pid = p.pid
  union all
  select p.pid, p.command::text, p.datname::text,
         case when p.datname = current_database() then p.relid::regclass::text else p.relid::text end,
         p.phase::text,
         case when p.blocks_total > 0 then p.blocks_done else p.tuples_done end::bigint,
         case when p.blocks_total > 0 then p.blocks_total else p.tuples_total end::bigint,
         case when p.blocks_total > 0 then 'blocks' else 'tuples' end::text,
         round(extract(epoch from now() - a.query_start)::numeric, 1)::float8,
         (p.lockers_total - p.lockers_done)::bigint
  from sys_stat_progress_create_index p left join sys_stat_activity a on a.pid = p.pid
  union all
  select p.pid, p.command::text, p.datname::text,
         case when p.datname = current_database() then p.relid::regclass::text else p.relid::text end,
         p.phase::text, p.heap_blks_scanned::bigint, p.heap_blks_total::bigint, 'blocks'::text,
         round(extract(epoch from now() - a.query_start)::numeric, 1)::float8,
         null::bigint
  from sys_stat_progress_cluster p left join sys_stat_activity a on a.pid = p.pid
  union all
  select c.pid, 'CHECKPOINT', null::text, null::text, c.phase::text,
         c.buffers_processed::bigint, c.buffers_scan::bigint, 'buffers'::text,
         round(extract(epoch from now() - c.start_time::timestamptz)::numeric, 1)::float8,
         null::bigint
  from sys_stat_progress_checkpoint c) x
order by running desc nulls last, pid`

func ProgressList(ctx context.Context, x *pgx.Conn) facts.ProgressList {
	st, reason, out := collect(ctx, x, progressListSQL, func(r pgx.CollectableRow) (facts.Operation, error) {
		var o facts.Operation
		err := r.Scan(&o.PID, &o.Command, &o.Datname, &o.Relation, &o.Phase, &o.Done, &o.Total, &o.Unit, &o.RunningS, &o.WaitingLockers)
		return o, err
	})
	return facts.ProgressList{Status: st, Reason: reason, Rows: out}
}
