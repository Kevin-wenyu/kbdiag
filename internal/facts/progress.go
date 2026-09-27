package facts

const (
	ProgressListID       = "progress.list"       // VACUUM, CREATE INDEX, CLUSTER
	ProgressCheckpointID = "progress.checkpoint" // KES's own checkpoint view
)

// ProgressColumns is the column contract of both progress probes (PRD §5.2).
var ProgressColumns = []string{"pid", "command", "datname", "relation", "phase", "done", "total", "unit", "running_s", "waiting_lockers"}

// Operation is one long operation from one of the four progress views,
// VACUUM, CREATE INDEX, CLUSTER / VACUUM FULL and KES's CHECKPOINT, put in
// the same columns.
type Operation struct {
	PID            int32
	Command        string // VACUUM, autovacuum, CREATE INDEX ..., CLUSTER, VACUUM FULL, CHECKPOINT
	Datname        *string
	Relation       *string // a name in the current database, the oid elsewhere
	Phase          *string // NULL: masked for this account
	Done           *int64
	Total          *int64 // NULL: the phase has no total
	Unit           string // blocks, tuples or buffers
	RunningS       *float64
	WaitingLockers *int64 // CREATE INDEX CONCURRENTLY: transactions it still waits for
}

func (o Operation) Row() []any {
	return []any{o.PID, o.Command, o.Datname, o.Relation, o.Phase, o.Done, o.Total, o.Unit, o.RunningS, o.WaitingLockers}
}

type ProgressList struct {
	Status Status
	Reason string
	Rows   []Operation
}

// Redacted reports operations whose progress this account may not read.
func (p ProgressList) Redacted(id string) []Redaction {
	n := 0
	for _, x := range p.Rows {
		if x.Phase == nil {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return []Redaction{{ProbeID: id, Field: "phase", Reason: ReasonInsufficientPrivilege, RowsAffected: n}}
}
