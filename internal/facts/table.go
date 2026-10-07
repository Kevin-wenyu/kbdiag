package facts

const (
	TableInfoID    = "table.info"
	TableSizeID    = "table.size"
	TableStatsID   = "table.stats"
	TableIndexesID = "table.indexes"
)

// Column contracts of the table probes (PRD §5.2). freeze.limits and
// vacuum.settings are reused for the judgments.
var (
	TableInfoColumns  = []string{"oid", "schemaname", "relname", "relkind", "relpersistence", "reltuples", "relpages", "reloptions", "xid_age", "mxid_age"}
	TableSizeColumns  = []string{"total_bytes", "table_bytes", "index_bytes", "toast_bytes"}
	TableStatsColumns = []string{"n_live_tup", "n_dead_tup", "n_mod_since_analyze", "last_vacuum_age_s", "last_autovacuum_age_s", "last_analyze_age_s", "last_autoanalyze_age_s",
		"vacuum_count", "autovacuum_count", "analyze_count", "autoanalyze_count", "seq_scan", "seq_tup_read", "idx_scan", "idx_tup_fetch",
		"n_tup_ins", "n_tup_upd", "n_tup_del", "n_tup_hot_upd", "heap_blks_read", "heap_blks_hit", "idx_blks_read", "idx_blks_hit"}
	TableIndexColumns = []string{"indexrelname", "definition", "bytes", "is_unique", "is_primary", "is_valid", "idx_scan"}
)

// TableInfo is the single row of table.info: what sys_class knows of the
// table and its ages. It takes no lock, so it answers even while the table
// is held exclusively; the sizes are table.size.
type TableInfo struct {
	OID            uint32
	Schemaname     string
	Relname        string
	Relkind        string
	Relpersistence string
	Reltuples      float32
	Relpages       int32
	Reloptions     []string
	XIDAge         *int32 // NULL: no frozen xid (relfrozenxid 0, or a partitioned table)
	MXIDAge        *int32
}

func (t TableInfo) Row() []any {
	opts := t.Reloptions
	if opts == nil {
		opts = []string{}
	}
	return []any{t.OID, t.Schemaname, t.Relname, t.Relkind, t.Relpersistence, t.Reltuples, t.Relpages, opts, t.XIDAge, t.MXIDAge}
}

type TableInfos struct {
	Status Status
	Reason string
	Rows   []TableInfo
}

// TableSize is the single row of table.size: the heap (with its free
// space and visibility maps), indexes and TOAST add up to the total.
type TableSize struct {
	TotalBytes int64
	TableBytes int64
	IndexBytes int64
	ToastBytes *int64 // NULL without a TOAST table
}

func (t TableSize) Row() []any { return []any{t.TotalBytes, t.TableBytes, t.IndexBytes, t.ToastBytes} }

type TableSizes struct {
	Status Status
	Reason string
	Rows   []TableSize
}

// TableStat is the table's row of sys_stat_user_tables joined with
// sys_statio_user_tables: counters since the statistics reset.
type TableStat struct {
	NLiveTup, NDeadTup, NModSinceAnalyze                     int64
	LastVacuumAgeS, LastAutovacuumAgeS                       *float64
	LastAnalyzeAgeS, LastAutoanalyzeAgeS                     *float64
	VacuumCount, AutovacuumCount, AnalyzeCount, AutoanalyzeN int64
	SeqScan, SeqTupRead                                      int64
	IdxScan, IdxTupFetch                                     *int64 // NULL without indexes
	NTupIns, NTupUpd, NTupDel, NTupHotUpd                    int64
	HeapBlksRead, HeapBlksHit                                *int64
	IdxBlksRead, IdxBlksHit                                  *int64
}

func (s TableStat) Row() []any {
	return []any{s.NLiveTup, s.NDeadTup, s.NModSinceAnalyze, s.LastVacuumAgeS, s.LastAutovacuumAgeS, s.LastAnalyzeAgeS, s.LastAutoanalyzeAgeS,
		s.VacuumCount, s.AutovacuumCount, s.AnalyzeCount, s.AutoanalyzeN, s.SeqScan, s.SeqTupRead, s.IdxScan, s.IdxTupFetch,
		s.NTupIns, s.NTupUpd, s.NTupDel, s.NTupHotUpd, s.HeapBlksRead, s.HeapBlksHit, s.IdxBlksRead, s.IdxBlksHit}
}

type TableStats struct {
	Status Status
	Reason string
	Rows   []TableStat
}

type TableIndex struct {
	Name       string
	Definition string
	Bytes      *int64 // NULL when the index is locked (table.size not read)
	IsUnique   bool
	IsPrimary  bool
	IsValid    bool
	IdxScan    *int64 // NULL on a standby: statistics are local
}

func (i TableIndex) Row() []any {
	return []any{i.Name, i.Definition, i.Bytes, i.IsUnique, i.IsPrimary, i.IsValid, i.IdxScan}
}

type TableIndexes struct {
	Status Status
	Reason string
	Rows   []TableIndex
}
