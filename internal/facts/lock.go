package facts

import "slices"

// LockListID is the probe_id of the sys_locks probe.
const LockListID = "lock.list"

// LockColumns is the column contract of lock.list (PRD §5.1).
var LockColumns = []string{"pid", "locktype", "relation", "mode", "granted", "wait_s", "blocked_by"}

// Lock is one row of sys_locks.
type Lock struct {
	PID       *int32 // NULL for locks held by a prepared (2PC) transaction
	Locktype  string
	Relation  *string // schema.name; the oid when it is in another database
	Mode      string
	Granted   bool
	WaitS     *float64 // waiting rows only; see probe.LockList for what it measures
	BlockedBy []int32  // waiting rows only; 0 stands for a prepared transaction
	Masked    bool     // the holder's sys_stat_activity row is hidden from us
}

func (l Lock) Row() []any {
	blocked := l.BlockedBy
	if blocked == nil {
		blocked = []int32{}
	}
	return []any{l.PID, l.Locktype, l.Relation, l.Mode, l.Granted, l.WaitS, blocked}
}

// Blockers is BlockedBy without repeats, in order. sys_blocking_pids
// reports 0 once per prepared transaction in the way, and a pid more than
// once when parallel workers are involved; each blocker counts once.
func (l Lock) Blockers() []int32 {
	var out []int32
	for _, b := range l.BlockedBy {
		if !slices.Contains(out, b) {
			out = append(out, b)
		}
	}
	return out
}

// PreparedBlocker is the pid sys_blocking_pids reports for a prepared
// transaction (measured on V8R6C9).
const PreparedBlocker = 0

type LockList struct {
	Status Status
	Reason string
	Rows   []Lock
}

// Redacted reports waiting rows whose wait time is hidden: wait_s comes from
// sys_stat_activity, which KES masks for other users' sessions.
func (l LockList) Redacted() []Redaction {
	n := 0
	for _, x := range l.Rows {
		if !x.Granted && x.Masked {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return []Redaction{{ProbeID: LockListID, Field: "wait_s", Reason: ReasonInsufficientPrivilege, RowsAffected: n}}
}
