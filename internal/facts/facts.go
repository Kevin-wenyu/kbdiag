// Package facts holds what the probes collected. It is the input of rule and
// report, and carries no judgment of its own.
package facts

import "time"

// Status is the per-probe collection status (PRD §5 data.status).
type Status string

const (
	StatusOK            Status = "ok"
	StatusSkipped       Status = "skipped"
	StatusError         Status = "error"
	StatusNotApplicable Status = "not_applicable"
)

// Context describes the instance the facts were collected from.
type Context struct {
	Version     string
	Role        string // primary | standby
	Location    string // local | remote
	User        string
	CollectedAt time.Time
}

// Redaction is one column that KES did not report for some rows: masked for
// lack of privilege, or not tracked because track_activities was off there.
type Redaction struct {
	ProbeID      string
	Field        string
	Reason       string
	RowsAffected int
}

const (
	ReasonInsufficientPrivilege = "insufficient_privilege"
	ReasonTrackActivitiesOff    = "track_activities_off"
)
