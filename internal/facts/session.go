package facts

// SessionActivityID is the probe_id of the sys_stat_activity probe.
const SessionActivityID = "session.activity"

// SessionColumns is the column contract of session.activity (PRD §5.1).
var SessionColumns = []string{
	"pid", "usename", "datname", "application_name", "client_addr", "backend_type",
	"state", "backend_xid", "backend_xmin", "xact_age_s", "query_age_s", "state_age_s",
	"wait_event_type", "wait_event", "query",
}

// Session is one row of session.activity. Everything except PID can be NULL:
// background processes have no user or database, and KES masks other users'
// sessions for accounts without sys_monitor.
type Session struct {
	PID             int32
	Usename         *string
	Datname         *string
	ApplicationName *string
	ClientAddr      *string
	BackendType     *string
	State           *string
	BackendXID      *uint32
	BackendXmin     *uint32
	XactAgeS        *float64
	QueryAgeS       *float64
	StateAgeS       *float64
	WaitEventType   *string
	WaitEvent       *string
	Query           *string
}

// Row returns the session in SessionColumns order.
func (s Session) Row() []any {
	return []any{
		s.PID, s.Usename, s.Datname, s.ApplicationName, s.ClientAddr, s.BackendType,
		s.State, s.BackendXID, s.BackendXmin, s.XactAgeS, s.QueryAgeS, s.StateAgeS,
		s.WaitEventType, s.WaitEvent, s.Query,
	}
}

// insufficientPrivilege is what KES puts in query for sessions the current
// user may not see; the columns in maskedSessionColumns are then NULL, while
// backend_xid and backend_xmin stay visible (measured on V8R6C9 as
// kbdiag_ro, see docs/engineering.md §3).
const insufficientPrivilege = "<insufficient privilege>"

var maskedSessionColumns = []string{
	"client_addr", "backend_type", "state", "xact_age_s",
	"query_age_s", "state_age_s", "wait_event_type", "wait_event", "query",
}

// Masked reports whether KES hid this session's details from us.
func (s Session) Masked() bool {
	return s.Query != nil && *s.Query == insufficientPrivilege
}

// A session with track_activities off (ALTER ROLE ... SET, or its own SET)
// shows state='disabled' even when ours is on; these columns then say nothing
// about it (measured on V8R6C9: state_age_s and wait_event* NULL, query ”,
// while xact_age_s and query_age_s keep stale values).
var untrackedSessionColumns = []string{"state", "state_age_s", "wait_event_type", "wait_event", "query"}

// Untracked reports whether the session does not report its activity.
func (s Session) Untracked() bool {
	return s.State != nil && *s.State == "disabled"
}

// SessionActivity is the result of the session.activity probe.
type SessionActivity struct {
	Status Status
	Reason string
	Rows   []Session
}

// Redacted lists the unreported columns, one entry per column and reason, or
// nil when every row is fully visible.
func (a SessionActivity) Redacted() []Redaction {
	masked, untracked := 0, 0
	for _, s := range a.Rows {
		switch {
		case s.Masked():
			masked++
		case s.Untracked():
			untracked++
		}
	}
	var out []Redaction
	add := func(cols []string, reason string, n int) {
		if n == 0 {
			return
		}
		for _, f := range cols {
			out = append(out, Redaction{ProbeID: SessionActivityID, Field: f, Reason: reason, RowsAffected: n})
		}
	}
	add(maskedSessionColumns, ReasonInsufficientPrivilege, masked)
	add(untrackedSessionColumns, ReasonTrackActivitiesOff, untracked)
	return out
}
