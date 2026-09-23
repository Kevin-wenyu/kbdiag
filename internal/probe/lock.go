package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// lockListSQL is lock.list: every lock but our own.
// Source: written for kbdiag against the sys_locks columns and
// sys_blocking_pids() in the KES V8R6 manual (help.kingbase.com.cn/v8,
// 系统视图 sys_locks; 系统信息函数 sys_blocking_pids), checked 2026-09-24
// on node1/node2. KES notes:
//   - V8R6 sys_locks has no waitstart, so wait_s is the age of the waiter's
//     current statement (state_change): an upper bound of the lock wait
//   - locks of a prepared transaction have pid NULL; sys_blocking_pids
//     reports such a blocker as 0
//   - relation is looked up in this database's catalog; a relation in
//     another database shows as its oid
const lockListSQL = `
select l.pid,
       l.locktype,
       case when l.relation is null then null
            when c.oid is null then l.relation::text
            else n.nspname::text || '.' || c.relname::text end as relation,
       l.mode,
       l.granted,
       case when not l.granted
            then round(extract(epoch from now() - a.state_change)::numeric, 1)::float8 end as wait_s,
       case when l.granted then '{}'::int[] else sys_blocking_pids(l.pid) end as blocked_by,
       coalesce(a.query = '<insufficient privilege>', false) as masked
from sys_locks l
left join sys_stat_activity a on a.pid = l.pid
left join sys_class c on c.oid = l.relation
left join sys_namespace n on n.oid = c.relnamespace
where l.pid is distinct from sys_backend_pid()
order by l.pid nulls last, l.granted desc, l.locktype, l.mode, relation`

func LockList(ctx context.Context, x *pgx.Conn) facts.LockList {
	st, reason, out := collect(ctx, x, lockListSQL, func(r pgx.CollectableRow) (facts.Lock, error) {
		var l facts.Lock
		err := r.Scan(&l.PID, &l.Locktype, &l.Relation, &l.Mode, &l.Granted, &l.WaitS, &l.BlockedBy, &l.Masked)
		return l, err
	})
	return facts.LockList{Status: st, Reason: reason, Rows: out}
}
