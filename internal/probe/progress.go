package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// progressListSQL is progress.list: the three PG progress views of KES
// V8R6 in one set of columns (there is no ANALYZE or base backup view:
// stage 0 capture progress_*_views). done/total follow the phase:
//   - VACUUM counts blocks scanned, then blocks vacuumed in "vacuuming
//     heap"; its index phases have no counter (NULL)
//   - CREATE INDEX counts blocks while scanning the table, tuples once it
//     sorts and loads them (the block counters stay at their last value)
//   - CLUSTER counts heap blocks for a sequential scan, tuples (no total)
//     for an index scan
//
// Running time is from xact_start, as vacuum.progress (a manual VACUUM of
// several tables runs one transaction per table). A masked row has phase,
// relid and counters NULL (PG12); backend_type is masked too, so such a
// VACUUM is not called manual or autovacuum. Not run on a VM yet.
const progressListSQL = `
select * from (
  select p.pid,
         case when a.backend_type = 'autovacuum worker' then 'autovacuum'
              when a.backend_type is null then 'VACUUM or autovacuum'
              else 'VACUUM' end as command,
         p.datname::text,
         case when p.datname = current_database() then p.relid::regclass::text else p.relid::text end,
         p.phase::text,
         case when p.phase = 'vacuuming heap' then p.heap_blks_vacuumed
              when p.phase in ('scanning heap', 'truncating heap', 'initializing') then p.heap_blks_scanned end::bigint,
         case when p.phase in ('vacuuming heap', 'scanning heap', 'truncating heap', 'initializing') then p.heap_blks_total end::bigint,
         'blocks'::text,
         round(extract(epoch from now() - a.xact_start)::numeric, 1)::float8 as running,
         null::bigint
  from sys_stat_progress_vacuum p left join sys_stat_activity a on a.pid = p.pid
  union all
  select p.pid, p.command::text, p.datname::text,
         case when p.datname = current_database() then p.relid::regclass::text else p.relid::text end,
         p.phase::text,
         case when p.tuples_total > 0 then p.tuples_done else p.blocks_done end::bigint,
         case when p.tuples_total > 0 then p.tuples_total else p.blocks_total end::bigint,
         case when p.tuples_total > 0 then 'tuples' else 'blocks' end::text,
         round(extract(epoch from now() - a.xact_start)::numeric, 1)::float8,
         (p.lockers_total - p.lockers_done)::bigint
  from sys_stat_progress_create_index p left join sys_stat_activity a on a.pid = p.pid
  union all
  select p.pid, p.command::text, p.datname::text,
         case when p.datname = current_database() then p.relid::regclass::text else p.relid::text end,
         p.phase::text,
         case when p.heap_blks_total > 0 then p.heap_blks_scanned else p.heap_tuples_scanned end::bigint,
         case when p.heap_blks_total > 0 then p.heap_blks_total end::bigint,
         case when p.heap_blks_total > 0 then 'blocks' else 'tuples' end::text,
         round(extract(epoch from now() - a.xact_start)::numeric, 1)::float8,
         null::bigint
  from sys_stat_progress_cluster p left join sys_stat_activity a on a.pid = p.pid) x
order by running desc nulls last, pid`

// progressCheckpointSQL is progress.checkpoint: KES's own
// sys_stat_progress_checkpoint, in the same columns. Only its column names
// are captured (stage 0), so it is a probe of its own: if a cast fails
// there, the other views still answer. buffers_processed of buffers_scan
// is taken as done of total, not verified.
const progressCheckpointSQL = `
select c.pid, 'CHECKPOINT', null::text, null::text, c.phase::text,
       c.buffers_processed::bigint, c.buffers_scan::bigint, 'buffers'::text,
       round(extract(epoch from now() - c.start_time::timestamptz)::numeric, 1)::float8,
       null::bigint
from sys_stat_progress_checkpoint c`

func scanOperation(r pgx.CollectableRow) (facts.Operation, error) {
	var o facts.Operation
	err := r.Scan(&o.PID, &o.Command, &o.Datname, &o.Relation, &o.Phase, &o.Done, &o.Total, &o.Unit, &o.RunningS, &o.WaitingLockers)
	return o, err
}

func ProgressCheckpoint(ctx context.Context, x *pgx.Conn) facts.ProgressList {
	st, reason, out := collect(ctx, x, progressCheckpointSQL, scanOperation)
	return facts.ProgressList{Status: st, Reason: reason, Rows: out}
}

func ProgressList(ctx context.Context, x *pgx.Conn) facts.ProgressList {
	st, reason, out := collect(ctx, x, progressListSQL, scanOperation)
	return facts.ProgressList{Status: st, Reason: reason, Rows: out}
}
