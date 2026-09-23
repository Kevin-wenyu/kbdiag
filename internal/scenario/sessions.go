// Package scenario declares, per command, which facts it reads and which
// rules it runs, and assembles the Report. It never touches the database.
package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

type SessionsOptions struct {
	ActiveOnly bool
	Limit      int
	Thresholds rule.Thresholds
}

// Sessions builds the sessions report. The rules judge every row; --active
// and --limit only narrow what is shown. --active keeps masked and untracked
// rows: their state is hidden, so they may be active.
func Sessions(c facts.Context, a facts.SessionActivity, o SessionsOptions) *report.Report {
	rep := report.New("sessions", c, rule.Sessions(a, o.Thresholds))
	shown := a.Rows
	if o.ActiveOnly {
		shown = nil
		for _, s := range a.Rows {
			if s.Masked() || s.Untracked() || (s.State != nil && *s.State == "active") {
				shown = append(shown, s)
			}
		}
	}
	rows := make([][]any, len(shown))
	for i, s := range shown {
		rows[i] = s.Row()
	}
	rep.AddProbe(facts.SessionActivityID, a.Status, a.Reason, facts.SessionColumns, rows, o.Limit)
	rep.AddRedacted(a.Redacted())
	return rep
}
