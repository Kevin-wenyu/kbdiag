package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// objectTablesSQL is object.tables: every table, partitioned table and
// materialized view of the current database with its size, largest first.
// TOAST tables are counted in their parent, not listed.
// Source: written for kbdiag against the KES V8R6 manual (help.kingbase.com.cn/v8,
// 数据库对象管理函数 pg_total_relation_size, pg_relation_size,
// pg_indexes_size; size functions only have the pg_ prefix in KES). Stage 0
// capture (topobj_*_rels): kbdiag_ro reads the same sizes as system; the
// standby's match the primary's; reltuples is float4 (ksql shows 1e+06)
// and is cast. The heap is pg_table_size less TOAST, so it includes the
// free space and visibility maps and the three parts add up to the total.
// A relation dropped while the query runs reads NULL (try_relation_open)
// and is left out. The size functions take AccessShareLock on each relation, so
// a relation held under AccessExclusiveLock (VACUUM FULL, TRUNCATE, most
// ALTER TABLE) stops the probe at lock_timeout: it is then skipped with
// that reason, never shown partly. Not run on a VM yet.
const objectTablesSQL = `
select * from (
  select n.nspname::text, c.relname::text, c.relkind::text,
         pg_total_relation_size(c.oid) as total,
         pg_table_size(c.oid) - coalesce(case when c.reltoastrelid <> 0 then pg_total_relation_size(c.reltoastrelid) end, 0),
         pg_indexes_size(c.oid),
         case when c.reltoastrelid <> 0 then pg_total_relation_size(c.reltoastrelid) end,
         c.reltuples::bigint
  from sys_class c join pg_catalog.pg_namespace n on n.oid = c.relnamespace
  where c.relkind in ('r', 'p', 'm')) x
where total is not null
order by 4 desc, 1, 2`

// objectIndexesSQL is object.indexes: every index of the current database
// but TOAST's (counted in their table's TOAST size), largest first (stage 0
// capture topobj_*_idx). Same lock caveat as object.tables.
const objectIndexesSQL = `
select * from (
  select n.nspname::text, c.relname::text, t.relname::text, pg_relation_size(c.oid) as bytes
  from sys_index i
  join sys_class c on c.oid = i.indexrelid
  join sys_class t on t.oid = i.indrelid
  join pg_catalog.pg_namespace n on n.oid = c.relnamespace
  where t.relkind <> 't') x
where bytes is not null
order by 4 desc, 1, 2`

func ObjectTables(ctx context.Context, x *pgx.Conn) facts.ObjectTables {
	st, reason, out := collect(ctx, x, objectTablesSQL, func(r pgx.CollectableRow) (facts.ObjectTable, error) {
		var t facts.ObjectTable
		err := r.Scan(&t.Schemaname, &t.Relname, &t.Relkind, &t.TotalBytes, &t.TableBytes, &t.IndexBytes, &t.ToastBytes, &t.Reltuples)
		return t, err
	})
	return facts.ObjectTables{Status: st, Reason: reason, Rows: out}
}

func ObjectIndexes(ctx context.Context, x *pgx.Conn) facts.ObjectIndexes {
	st, reason, out := collect(ctx, x, objectIndexesSQL, func(r pgx.CollectableRow) (facts.ObjectIndex, error) {
		var i facts.ObjectIndex
		err := r.Scan(&i.Schemaname, &i.Relname, &i.TableName, &i.Bytes)
		return i, err
	})
	return facts.ObjectIndexes{Status: st, Reason: reason, Rows: out}
}
