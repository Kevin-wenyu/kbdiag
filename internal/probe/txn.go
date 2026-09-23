package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// txnPreparedSQL is txn.prepared.
// Source: written for kbdiag against the sys_prepared_xacts columns in the
// KES V8R6 manual (help.kingbase.com.cn/v8, 系统视图 sys_prepared_xacts),
// checked 2026-09-24 on node1.
const txnPreparedSQL = `
select gid,
       owner::text,
       database::text,
       prepared,
       round(extract(epoch from now() - prepared)::numeric, 1)::float8 as age_s,
       transaction
from sys_prepared_xacts
order by prepared, gid`

// TxnPrepared reads the prepared transactions. A standby does not see the
// primary's (measured 2026-09-23), so there the probe is not applicable.
func TxnPrepared(ctx context.Context, x *pgx.Conn, c facts.Context) facts.TxnPrepared {
	if c.Role == "standby" {
		return facts.TxnPrepared{Status: facts.StatusNotApplicable, Reason: "备库看不到主库的两阶段提交事务，请在主库上运行 kbdiag txn"}
	}
	st, reason, out := collect(ctx, x, txnPreparedSQL, func(r pgx.CollectableRow) (facts.Prepared, error) {
		var p facts.Prepared
		err := r.Scan(&p.GID, &p.Owner, &p.Database, &p.PreparedAt, &p.AgeS, &p.Transaction)
		return p, err
	})
	return facts.TxnPrepared{Status: st, Reason: reason, Rows: out}
}
