package probe

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// tableInfoSQL is table.info. The name is resolved by to_regclass($1), bound
// as a parameter: identifier rules apply (unquoted folds to lower case,
// the search_path is searched), and a name that resolves to nothing gives
// no row instead of an error.
// Source: written for kbdiag against the KES V8R6 manual (help.kingbase.com.cn/v8,
// 系统表 sys_class; to_regclass; size functions). Stage 0 capture
// (table_*_class, table_*_sizes, table_*_ambiguous): kbdiag_inj_tbl and
// KBDIAG_INJ_TBL resolve to the same table, public."KBDIAG_INJ_TBL" to
// NULL; kbdiag_ro reads the same. relfrozenxid 0 (a partitioned table, or
// the relation freeze leaves out) gives a NULL age. No size function here:
// they lock the table, and a table held under AccessExclusiveLock is when
// someone looks at it. Not run on a VM yet.
const tableInfoSQL = `
select c.oid, n.nspname::text, c.relname::text, c.relkind::text, c.relpersistence::text, c.reltuples, c.relpages,
       c.reloptions::text[],
       case when c.relfrozenxid::text <> '0' then age(c.relfrozenxid) end,
       case when c.relminmxid::text <> '0' then mxid_age(c.relminmxid) end
from sys_class c join pg_catalog.pg_namespace n on n.oid = c.relnamespace
where c.oid = to_regclass($1::text)`

// tableSizeSQL is table.size: the heap is pg_table_size less TOAST, as in
// object.tables, so the parts add up. The size functions take
// AccessShareLock and may stop at lock_timeout (stage 0 capture
// table_*_sizes).
const tableSizeSQL = `
select pg_total_relation_size(c.oid),
       pg_table_size(c.oid) - coalesce(case when c.reltoastrelid <> 0 then pg_total_relation_size(c.reltoastrelid) end, 0),
       pg_indexes_size(c.oid),
       case when c.reltoastrelid <> 0 then pg_total_relation_size(c.reltoastrelid) end
from sys_class c
where c.oid = $1::oid`

// tableStatsSQL is table.stats: the table's cumulative counters (stage 0
// capture table_*_stat, table_*_iostat; all zeros on a standby).
const tableStatsSQL = `
select s.n_live_tup, s.n_dead_tup, s.n_mod_since_analyze,
       round(extract(epoch from now() - s.last_vacuum)::numeric, 1)::float8,
       round(extract(epoch from now() - s.last_autovacuum)::numeric, 1)::float8,
       round(extract(epoch from now() - s.last_analyze)::numeric, 1)::float8,
       round(extract(epoch from now() - s.last_autoanalyze)::numeric, 1)::float8,
       s.vacuum_count, s.autovacuum_count, s.analyze_count, s.autoanalyze_count,
       s.seq_scan, s.seq_tup_read, s.idx_scan, s.idx_tup_fetch,
       s.n_tup_ins, s.n_tup_upd, s.n_tup_del, s.n_tup_hot_upd,
       io.heap_blks_read, io.heap_blks_hit, io.idx_blks_read, io.idx_blks_hit
from sys_stat_user_tables s left join sys_statio_user_tables io on io.relid = s.relid
where s.relid = $1::oid`

// tableIndexesSQL is table.indexes (stage 0 capture table_*_indexes,
// table_*_idxstat). idx_scan is node-local: NULL on a standby. %s is the
// size: pg_relation_size locks each index, and VACUUM FULL, CLUSTER and
// TRUNCATE hold them exclusively, so when table.size could not be read
// the sizes are left NULL and the list still comes back (review
// 2026-10-07).
const tableIndexesSQL = `
select c.relname::text, pg_get_indexdef(i.indexrelid), %s,
       i.indisunique, i.indisprimary, i.indisvalid,
       case when not sys_is_in_recovery() then s.idx_scan end
from sys_index i
join sys_class c on c.oid = i.indexrelid
left join sys_stat_user_indexes s on s.indexrelid = i.indexrelid
where i.indrelid = $1::oid
order by 1`

// TableInfo resolves the name; no row means no such relation.
func TableInfo(ctx context.Context, x *pgx.Conn, name string) facts.TableInfos {
	st, reason, out := collectArgs(ctx, x, tableInfoSQL, []any{name}, func(r pgx.CollectableRow) (facts.TableInfo, error) {
		var t facts.TableInfo
		err := r.Scan(&t.OID, &t.Schemaname, &t.Relname, &t.Relkind, &t.Relpersistence, &t.Reltuples, &t.Relpages, &t.Reloptions, &t.XIDAge, &t.MXIDAge)
		return t, err
	})
	return facts.TableInfos{Status: st, Reason: reason, Rows: out}
}

func TableSize(ctx context.Context, x *pgx.Conn, oid uint32) facts.TableSizes {
	st, reason, out := collectArgs(ctx, x, tableSizeSQL, []any{oid}, func(r pgx.CollectableRow) (facts.TableSize, error) {
		var t facts.TableSize
		err := r.Scan(&t.TotalBytes, &t.TableBytes, &t.IndexBytes, &t.ToastBytes)
		return t, err
	})
	return facts.TableSizes{Status: st, Reason: reason, Rows: out}
}

// TableStats reads the counters of the table; a standby's are all zeros,
// and with track_counts off they stop moving (as vacuum.tables).
func TableStats(ctx context.Context, x *pgx.Conn, c facts.Context, oid uint32) facts.TableStats {
	if c.Role == "standby" {
		return facts.TableStats{Status: facts.StatusNotApplicable, Reason: "standby: table statistics are local to each node and stay 0 here; run on the primary"}
	}
	var track string
	if err := x.QueryRow(ctx, "select current_setting('track_counts')").Scan(&track); err != nil {
		st, reason := classify(err)
		return facts.TableStats{Status: st, Reason: reason}
	}
	if track != "on" {
		return facts.TableStats{Status: facts.StatusSkipped, Reason: "track_counts=" + track + ": table statistics are not collected"}
	}
	st, reason, out := collectArgs(ctx, x, tableStatsSQL, []any{oid}, func(r pgx.CollectableRow) (facts.TableStat, error) {
		var s facts.TableStat
		err := r.Scan(&s.NLiveTup, &s.NDeadTup, &s.NModSinceAnalyze, &s.LastVacuumAgeS, &s.LastAutovacuumAgeS, &s.LastAnalyzeAgeS, &s.LastAutoanalyzeAgeS,
			&s.VacuumCount, &s.AutovacuumCount, &s.AnalyzeCount, &s.AutoanalyzeN, &s.SeqScan, &s.SeqTupRead, &s.IdxScan, &s.IdxTupFetch,
			&s.NTupIns, &s.NTupUpd, &s.NTupDel, &s.NTupHotUpd, &s.HeapBlksRead, &s.HeapBlksHit, &s.IdxBlksRead, &s.IdxBlksHit)
		return s, err
	})
	return facts.TableStats{Status: st, Reason: reason, Rows: out}
}

// TableIndexes lists the indexes; sizes says whether to read their sizes.
func TableIndexes(ctx context.Context, x *pgx.Conn, oid uint32, sizes bool) facts.TableIndexes {
	size := "null::bigint"
	if sizes {
		size = "pg_relation_size(i.indexrelid)"
	}
	st, reason, out := collectArgs(ctx, x, fmt.Sprintf(tableIndexesSQL, size), []any{oid}, func(r pgx.CollectableRow) (facts.TableIndex, error) {
		var i facts.TableIndex
		err := r.Scan(&i.Name, &i.Definition, &i.Bytes, &i.IsUnique, &i.IsPrimary, &i.IsValid, &i.IdxScan)
		return i, err
	})
	return facts.TableIndexes{Status: st, Reason: reason, Rows: out}
}
