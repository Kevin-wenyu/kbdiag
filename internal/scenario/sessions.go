// Package scenario declares, per command, which facts it reads and which
// rules it runs, and assembles the Report. It never touches the database.
package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

type SessionsOptions struct {
	All        bool // list every session, not only client sessions that are not idle
	Limit      int
	Thresholds rule.Thresholds
}

// Sessions builds the sessions report. The rules and the text summary see
// every row; --limit trims the text list and the JSON rows, and --all only
// changes the text list: the JSON always carries every session.
func Sessions(c facts.Context, a facts.SessionActivity, o SessionsOptions) *report.Report {
	rep := report.New("sessions", c, rule.Sessions(a, o.Thresholds))
	rows := make([][]any, len(a.Rows))
	for i, s := range a.Rows {
		rows[i] = s.Row()
	}
	rep.AddProbe(facts.SessionActivityID, a.Status, a.Reason, facts.SessionColumns, rows, o.Limit)
	rep.AddRedacted(a.Redacted())
	rep.SetSessions(a.Rows, o.All, o.Limit)
	return rep
}
