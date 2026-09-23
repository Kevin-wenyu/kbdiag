package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// slotListSQL is slot.list.
// Source: written for kbdiag against the sys_replication_slots columns and
// the WAL functions in the KES V8R6 manual (help.kingbase.com.cn/v8, 系统视图
// sys_replication_slots; 备份控制函数 sys_current_wal_lsn,
// sys_last_wal_replay_lsn, sys_wal_lsn_diff), checked 2026-09-24 on
// node1/node2. sys_current_wal_lsn() fails on a standby (recovery is in
// progress), so retained WAL is measured from the replay LSN there.
const slotListSQL = `
select slot_name::text,
       slot_type,
       active,
       active_pid,
       xmin,
       catalog_xmin,
       age(xmin),
       restart_lsn::text,
       sys_wal_lsn_diff(case when sys_is_in_recovery() then sys_last_wal_replay_lsn()
                             else sys_current_wal_lsn() end, restart_lsn)::bigint
from sys_replication_slots
order by slot_name`

func SlotList(ctx context.Context, x *pgx.Conn) facts.SlotList {
	st, reason, out := collect(ctx, x, slotListSQL, func(r pgx.CollectableRow) (facts.Slot, error) {
		var s facts.Slot
		err := r.Scan(&s.Name, &s.Type, &s.Active, &s.ActivePID, &s.Xmin, &s.CatalogXmin, &s.XminAge, &s.RestartLSN, &s.RetainedWALBytes)
		return s, err
	})
	return facts.SlotList{Status: st, Reason: reason, Rows: out}
}
