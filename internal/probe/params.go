package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// paramsChangedSQL is params.changed: every parameter someone set. Left out:
// default; override (fixed at build or initdb: block_size, data_checksums);
// client and session (this connection's own: kbdiag sends application_name,
// lock_timeout and the like at startup, stage 0 shows ksql's
// application_name as client).
// Source: written for kbdiag against the KES V8R6 manual (help.kingbase.com.cn/v8,
// 系统视图 sys_settings). Stage 0 capture (params_*_nondefault): 509 rows
// for system, 482 for kbdiag_ro, which also gets sourcefile and sourceline
// as NULL. Not run on a VM yet.
const paramsChangedSQL = `
select name::text, setting, unit, source, sourcefile, sourceline, boot_val, reset_val, context, pending_restart
from sys_settings
where source not in ('default', 'override', 'client', 'session')
order by name`

func ParamsChanged(ctx context.Context, x *pgx.Conn) facts.ParamsChanged {
	st, reason, out := collect(ctx, x, paramsChangedSQL, func(r pgx.CollectableRow) (facts.Param, error) {
		var p facts.Param
		err := r.Scan(&p.Name, &p.Setting, &p.Unit, &p.Source, &p.Sourcefile, &p.Sourceline, &p.BootVal, &p.ResetVal, &p.Context, &p.PendingRestart)
		return p, err
	})
	return facts.ParamsChanged{Status: st, Reason: reason, Rows: out}
}
