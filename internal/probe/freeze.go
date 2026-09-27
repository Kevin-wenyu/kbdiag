package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// freezeDatabasesSQL is freeze.databases.
// Source: written for kbdiag against the KES V8R6 manual (help.kingbase.com.cn/v8,
// 系统表 sys_database; age, mxid_age). Stage 0 capture (freeze_*_db): kbdiag_ro
// sees every row; a standby reports the same ages as the primary (the
// catalog is replicated, age() counts from the replayed next xid).
// Not run on a VM yet.
const freezeDatabasesSQL = `
select datname::text, datfrozenxid, age(datfrozenxid), datminmxid, mxid_age(datminmxid), datallowconn
from sys_database
order by age(datfrozenxid) desc, datname`

// freezeTablesSQL is freeze.tables: every relation of the current database
// that carries a frozen xid. relfrozenxid 0 (InvalidTransactionId) is left
// out: the stage 0 capture (freeze_*_rel) has _kingbase_loginfo and its
// TOAST table with relfrozenxid 0, and age(0) reads 2147483647, which would
// look past every limit. The xid is compared as text: xid has no <> with
// integers in a PG12 kernel. Names are qualified so the reader can VACUUM
// them. The size is relpages × block_size, not pg_total_relation_size: that
// one takes AccessShareLock on every relation, and a single VACUUM FULL or
// TRUNCATE (common while fighting wraparound) would fail the whole probe on
// lock_timeout. Not run on a VM yet.
const freezeTablesSQL = `
select n.nspname::text || '.' || c.relname::text, c.relkind::text,
       c.relfrozenxid, age(c.relfrozenxid), c.relminmxid, mxid_age(c.relminmxid),
       c.relpages::bigint * current_setting('block_size')::bigint
from sys_class c join sys_namespace n on n.oid = c.relnamespace
where c.relkind in ('r', 'm', 't') and c.relfrozenxid::text <> '0'
order by age(c.relfrozenxid) desc, 1`

// freezeLimitsSQL is freeze.limits (stage 0 capture freeze_*_settings).
const freezeLimitsSQL = `
select current_setting('autovacuum_freeze_max_age')::bigint,
       current_setting('autovacuum_multixact_freeze_max_age')::bigint,
       current_setting('vacuum_freeze_table_age')::bigint`

func FreezeDatabases(ctx context.Context, x *pgx.Conn) facts.FreezeDatabases {
	st, reason, out := collect(ctx, x, freezeDatabasesSQL, func(r pgx.CollectableRow) (facts.FrozenDatabase, error) {
		var d facts.FrozenDatabase
		err := r.Scan(&d.Datname, &d.DatFrozenXID, &d.XIDAge, &d.DatMinMXID, &d.MXIDAge, &d.AllowConn)
		return d, err
	})
	return facts.FreezeDatabases{Status: st, Reason: reason, Rows: out}
}

func FreezeTables(ctx context.Context, x *pgx.Conn) facts.FreezeTables {
	st, reason, out := collect(ctx, x, freezeTablesSQL, func(r pgx.CollectableRow) (facts.FrozenTable, error) {
		var t facts.FrozenTable
		err := r.Scan(&t.Relation, &t.Relkind, &t.RelFrozenXID, &t.XIDAge, &t.RelMinMXID, &t.MXIDAge, &t.HeapBytesEst)
		return t, err
	})
	return facts.FreezeTables{Status: st, Reason: reason, Rows: out}
}

func FreezeLimits(ctx context.Context, x *pgx.Conn) facts.FreezeLimits {
	st, reason, out := collect(ctx, x, freezeLimitsSQL, func(r pgx.CollectableRow) (facts.FreezeLimit, error) {
		var l facts.FreezeLimit
		err := r.Scan(&l.FreezeMaxAge, &l.MultiFreezeMaxAge, &l.FreezeTableAge)
		return l, err
	})
	return facts.FreezeLimits{Status: st, Reason: reason, Rows: out}
}
