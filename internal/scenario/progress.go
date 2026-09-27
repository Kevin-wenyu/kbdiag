package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// Progress only shows: how long is too long has no objective line.
func Progress(c facts.Context, p, cp facts.ProgressList) *report.Report {
	hidden := len(p.Redacted(facts.ProgressListID))+len(cp.Redacted(facts.ProgressCheckpointID)) > 0
	rep := report.New("progress", c, rule.Display(hidden, p.Status, cp.Status))
	rep.AddProbe(facts.ProgressListID, p.Status, p.Reason, facts.ProgressColumns, rows(p.Rows), 0)
	rep.AddProbe(facts.ProgressCheckpointID, cp.Status, cp.Reason, facts.ProgressColumns, rows(cp.Rows), 0)
	rep.AddRedacted(p.Redacted(facts.ProgressListID))
	rep.AddRedacted(cp.Redacted(facts.ProgressCheckpointID))
	rep.SetProgress(p, cp)
	return rep
}
