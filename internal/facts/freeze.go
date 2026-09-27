package facts

const (
	FreezeDatabasesID = "freeze.databases"
	FreezeTablesID    = "freeze.tables"
	FreezeLimitsID    = "freeze.limits"
)

// Column contracts of the freeze probes (PRD §5.2).
var (
	FreezeDatabaseColumns = []string{"datname", "datfrozenxid", "xid_age", "datminmxid", "mxid_age", "datallowconn"}
	FreezeTableColumns    = []string{"relation", "relkind", "relfrozenxid", "xid_age", "relminmxid", "mxid_age", "heap_bytes_est"}
	FreezeLimitColumns    = []string{"autovacuum_freeze_max_age", "autovacuum_multixact_freeze_max_age", "vacuum_freeze_table_age"}
)

// XIDStopAge is the age at which a PG12 kernel refuses to assign new
// transaction IDs: SetTransactionIdLimit puts xidStopLimit 1,000,000 before
// the wrap limit, oldest datfrozenxid + 2^31 - 1 (PG14 moved it to 3,000,000).
// MXIDStopAge is the same for multixacts: multiStopLimit is 100 before
// their wrap limit. KES V8R6 runs a PG12 kernel; that it keeps these limits
// is not verified on KES yet.
const (
	XIDStopAge  = 1<<31 - 1 - 1_000_000
	MXIDStopAge = 1<<31 - 1 - 100
)

type FrozenDatabase struct {
	Datname      string
	DatFrozenXID uint32
	XIDAge       int32
	DatMinMXID   uint32
	MXIDAge      int32
	AllowConn    bool
}

func (d FrozenDatabase) Row() []any {
	return []any{d.Datname, d.DatFrozenXID, d.XIDAge, d.DatMinMXID, d.MXIDAge, d.AllowConn}
}

type FreezeDatabases struct {
	Status Status
	Reason string
	Rows   []FrozenDatabase
}

// FrozenTable is one relation of the current database with a frozen xid:
// a table, materialized view or TOAST table.
type FrozenTable struct {
	Relation     string
	Relkind      string
	RelFrozenXID uint32
	XIDAge       int32
	RelMinMXID   uint32
	MXIDAge      int32
	HeapBytesEst int64 // relpages × block_size: what VACUUM FREEZE has to read
}

func (t FrozenTable) Row() []any {
	return []any{t.Relation, t.Relkind, t.RelFrozenXID, t.XIDAge, t.RelMinMXID, t.MXIDAge, t.HeapBytesEst}
}

type FreezeTables struct {
	Status Status
	Reason string
	Rows   []FrozenTable
}

type FreezeLimit struct {
	FreezeMaxAge      int64
	MultiFreezeMaxAge int64
	FreezeTableAge    int64
}

func (l FreezeLimit) Row() []any { return []any{l.FreezeMaxAge, l.MultiFreezeMaxAge, l.FreezeTableAge} }

type FreezeLimits struct {
	Status Status
	Reason string
	Rows   []FreezeLimit
}
