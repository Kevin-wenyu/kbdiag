package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// Progress only shows: how long is too long has no objective line.
func Progress(c facts.Context, p facts.ProgressList) *report.Report {
	rep := report.New("progress", c, rule.Display(len(p.Redacted()) > 0, p.Status))
	rep.AddProbe(facts.ProgressListID, p.Status, p.Reason, facts.ProgressColumns, rows(p.Rows), 0)
	rep.AddRedacted(p.Redacted())
	rep.SetProgress(p)
	return rep
}
