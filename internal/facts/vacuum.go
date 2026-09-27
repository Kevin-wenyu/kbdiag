package facts

const (
	VacuumTablesID   = "vacuum.tables"
	VacuumProgressID = "vacuum.progress"
	VacuumSettingsID = "vacuum.settings"
)

// Column contracts of the vacuum probes (PRD §5.2).
var (
	VacuumTableColumns    = []string{"schemaname", "relname", "n_live_tup", "n_dead_tup", "reltuples", "reloptions", "last_vacuum_age_s", "last_autovacuum_age_s", "vacuum_count", "autovacuum_count"}
	VacuumProgressColumns = []string{"pid", "datname", "relation", "phase", "heap_blks_total", "heap_blks_scanned", "is_autovacuum", "xact_age_s"}
	VacuumSettingColumns  = []string{"autovacuum", "track_counts", "autovacuum_vacuum_threshold", "autovacuum_vacuum_scale_factor", "autovacuum_naptime_s", "autovacuum_max_workers"}
)

// VacuumTable is one row of sys_stat_user_tables in the current database,
// with the reltuples and reloptions autovacuum decides by.
type VacuumTable struct {
	Schemaname         string
	Relname            string
	NLiveTup           int64
	NDeadTup           int64
	Reltuples          float32  // float4 in sys_class, as the server computes with it
	Reloptions         []string // raw "name=value" entries
	LastVacuumAgeS     *float64 // NULL: never
	LastAutovacuumAgeS *float64
	VacuumCount        int64
	AutovacuumCount    int64
}

func (t VacuumTable) Row() []any {
	opts := t.Reloptions
	if opts == nil {
		opts = []string{}
	}
	return []any{t.Schemaname, t.Relname, t.NLiveTup, t.NDeadTup, t.Reltuples, opts, t.LastVacuumAgeS, t.LastAutovacuumAgeS, t.VacuumCount, t.AutovacuumCount}
}

type VacuumTables struct {
	Status Status
	Reason string
	Rows   []VacuumTable
}

// VacuumRun is a VACUUM running now, manual or autovacuum.
type VacuumRun struct {
	PID             int32
	Datname         *string
	Relation        *string // a name in the current database, the oid elsewhere
	Phase           *string // NULL: masked for this account
	HeapBlksTotal   *int64
	HeapBlksScanned *int64
	IsAutovacuum    *bool // NULL: backend_type masked
	XactAgeS        *float64
}

func (v VacuumRun) Row() []any {
	return []any{v.PID, v.Datname, v.Relation, v.Phase, v.HeapBlksTotal, v.HeapBlksScanned, v.IsAutovacuum, v.XactAgeS}
}

type VacuumProgress struct {
	Status Status
	Reason string
	Rows   []VacuumRun
}

// Redacted reports running vacuums whose progress this account may not read.
func (p VacuumProgress) Redacted() []Redaction {
	n := 0
	for _, x := range p.Rows {
		if x.Phase == nil {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return []Redaction{{ProbeID: VacuumProgressID, Field: "phase", Reason: ReasonInsufficientPrivilege, RowsAffected: n}}
}

type VacuumSetting struct {
	Autovacuum  string
	TrackCounts string
	Threshold   int64
	ScaleFactor float64
	NaptimeS    int64
	MaxWorkers  int32
}

func (s VacuumSetting) Row() []any {
	return []any{s.Autovacuum, s.TrackCounts, s.Threshold, s.ScaleFactor, s.NaptimeS, s.MaxWorkers}
}

type VacuumSettings struct {
	Status Status
	Reason string
	Rows   []VacuumSetting
}
