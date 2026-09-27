package facts

// WALPositionID is the probe_id of this node's WAL position.
const WALPositionID = "wal.position"

// WALPositionColumns is the column contract of wal.position (PRD §5.2).
var WALPositionColumns = []string{"in_recovery", "lsn", "wal_file"}

// Position is where this node's WAL is: the current insert position on a
// primary, the replay position on a standby (where sys_walfile_name fails:
// stage 0 capture wal_node2_*, so wal_file is NULL there).
type Position struct {
	InRecovery bool
	LSN        *string
	WALFile    *string
}

func (p Position) Row() []any { return []any{p.InRecovery, p.LSN, p.WALFile} }

type WALPosition struct {
	Status Status
	Reason string
	Rows   []Position
}
