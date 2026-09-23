package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// sessionActivitySQL is session.activity.
// Source: written for kbdiag against the sys_stat_activity column list in
// the KES V8R6 manual (help.kingbase.com.cn/v8, 动态性能视图 4.1
// sys_stat_activity), checked 2026-09-23. KES notes:
//   - ages are seconds via extract(epoch ...): interval text is KES-specific
//   - host(client_addr): client_addr::text carries a /32 suffix
//   - own backend excluded; smoke-tested with ksql on node1/node2 2026-09-23
const sessionActivitySQL = `
select pid,
       usename::text,
       datname::text,
       application_name,
       host(client_addr),
       backend_type,
       state,
       backend_xid,
       backend_xmin,
       round(extract(epoch from now() - xact_start)::numeric, 1)::float8 as xact_age_s,
       round(extract(epoch from now() - query_start)::numeric, 1)::float8 as query_age_s,
       round(extract(epoch from now() - state_change)::numeric, 1)::float8 as state_age_s,
       wait_event_type,
       wait_event,
       query
from sys_stat_activity
where pid <> sys_backend_pid()
order by xact_age_s desc nulls last, pid`

// SessionActivity reads sys_stat_activity. With track_activities off, KES
// no longer records state and query, so the probe is skipped instead of
// returning rows that look idle.
func SessionActivity(ctx context.Context, x *pgx.Conn) facts.SessionActivity {
	if st, reason := trackActivities(ctx, x); st != facts.StatusOK {
		return facts.SessionActivity{Status: st, Reason: reason}
	}
	st, reason, out := collect(ctx, x, sessionActivitySQL, func(r pgx.CollectableRow) (facts.Session, error) {
		var s facts.Session
		err := r.Scan(&s.PID, &s.Usename, &s.Datname, &s.ApplicationName, &s.ClientAddr, &s.BackendType,
			&s.State, &s.BackendXID, &s.BackendXmin, &s.XactAgeS, &s.QueryAgeS, &s.StateAgeS,
			&s.WaitEventType, &s.WaitEvent, &s.Query)
		return s, err
	})
	return facts.SessionActivity{Status: st, Reason: reason, Rows: out}
}
