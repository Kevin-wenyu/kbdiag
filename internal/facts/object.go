package facts

const (
	ObjectTablesID  = "object.tables"
	ObjectIndexesID = "object.indexes"
)

// Column contracts of the top-objects probes (PRD §5.2).
var (
	ObjectTableColumns = []string{"schemaname", "relname", "relkind", "total_bytes", "table_bytes", "index_bytes", "toast_bytes", "reltuples"}
	ObjectIndexColumns = []string{"schemaname", "relname", "table_name", "bytes"}
)

// ObjectTable is one table or materialized view of the current database
// and what it takes on disk.
type ObjectTable struct {
	Schemaname string
	Relname    string
	Relkind    string
	TotalBytes int64 // heap + indexes + TOAST
	TableBytes int64 // the heap with its free space and visibility maps
	IndexBytes int64
	ToastBytes *int64 // NULL without a TOAST table
	Reltuples  int64  // the planner's estimate
}

func (t ObjectTable) Row() []any {
	return []any{t.Schemaname, t.Relname, t.Relkind, t.TotalBytes, t.TableBytes, t.IndexBytes, t.ToastBytes, t.Reltuples}
}

type ObjectTables struct {
	Status Status
	Reason string
	Rows   []ObjectTable
}

type ObjectIndex struct {
	Schemaname string
	Relname    string
	TableName  string
	Bytes      int64
}

func (i ObjectIndex) Row() []any { return []any{i.Schemaname, i.Relname, i.TableName, i.Bytes} }

type ObjectIndexes struct {
	Status Status
	Reason string
	Rows   []ObjectIndex
}
