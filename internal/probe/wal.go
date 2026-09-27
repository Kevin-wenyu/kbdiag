package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// walPositionSQL is wal.position. sys_current_wal_lsn() and
// sys_walfile_name() fail on a standby (recovery is in progress: stage 0
// capture wal_node2_*), so a standby gives its replay position and no file;
// the CASE keeps them from running there.
// Source: KES V8R6 manual (备份控制函数 sys_current_wal_lsn,
// sys_last_wal_replay_lsn, sys_walfile_name). Not run on a VM yet.
const walPositionSQL = `
select sys_is_in_recovery(),
       case when sys_is_in_recovery() then sys_last_wal_replay_lsn() else sys_current_wal_lsn() end::text,
       case when not sys_is_in_recovery() then sys_walfile_name(sys_current_wal_lsn()) end`

func WALPosition(ctx context.Context, x *pgx.Conn) facts.WALPosition {
	st, reason, out := collect(ctx, x, walPositionSQL, func(r pgx.CollectableRow) (facts.Position, error) {
		var p facts.Position
		err := r.Scan(&p.InRecovery, &p.LSN, &p.WALFile)
		return p, err
	})
	return facts.WALPosition{Status: st, Reason: reason, Rows: out}
}
