package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// waitSummarySQL is wait.summary: sys_stat_activity grouped by wait event
// and state, our own backend excluded.
// Source: written for kbdiag against the sys_stat_activity columns in the
// KES V8R6 manual (help.kingbase.com.cn/v8, 动态性能视图 4.1
// sys_stat_activity), checked 2026-09-24 on node1/node2. Masked rows
// (query '<insufficient privilege>') have wait_event*, state NULL and fall
// into one group; masked counts them. background (checked 2026-09-26 on
// node1/node2: FILTER works, backend_type names ksh writer etc.) is for the
// text only.
const waitSummarySQL = `
select wait_event_type,
       wait_event,
       state,
       count(*)::int as sessions,
       array_agg(pid order by pid) as pids,
       count(*) filter (where query = '<insufficient privilege>')::int as masked,
       array_agg(pid order by pid) filter (where backend_type not in ('client backend', 'parallel worker')
                                            and query is distinct from '<insufficient privilege>') as background
from sys_stat_activity
where pid <> sys_backend_pid()
group by wait_event_type, wait_event, state
order by sessions desc, wait_event_type nulls last, wait_event nulls last, state nulls last`

func WaitSummary(ctx context.Context, x *pgx.Conn) facts.WaitSummary {
	if st, reason := trackActivities(ctx, x); st != facts.StatusOK {
		return facts.WaitSummary{Status: st, Reason: reason}
	}
	st, reason, out := collect(ctx, x, waitSummarySQL, func(r pgx.CollectableRow) (facts.Wait, error) {
		var w facts.Wait
		err := r.Scan(&w.WaitEventType, &w.WaitEvent, &w.State, &w.Sessions, &w.PIDs, &w.Masked, &w.Background)
		return w, err
	})
	return facts.WaitSummary{Status: st, Reason: reason, Rows: out}
}
