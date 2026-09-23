package facts

import "time"

// TxnPreparedID is the probe_id of the sys_prepared_xacts probe.
const TxnPreparedID = "txn.prepared"

// PreparedColumns is the column contract of txn.prepared (PRD §5.1).
var PreparedColumns = []string{"gid", "owner", "database", "prepared_at", "age_s", "transaction"}

// Prepared is one prepared (2PC) transaction that is not committed yet.
type Prepared struct {
	GID         string
	Owner       string
	Database    string
	PreparedAt  time.Time
	AgeS        float64
	Transaction uint32
}

func (p Prepared) Row() []any {
	return []any{p.GID, p.Owner, p.Database, p.PreparedAt.Truncate(time.Second).Format(time.RFC3339), p.AgeS, p.Transaction}
}

type TxnPrepared struct {
	Status Status
	Reason string
	Rows   []Prepared
}
