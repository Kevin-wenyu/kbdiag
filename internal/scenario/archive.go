package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

func Archive(c facts.Context, s facts.ArchiveStatus, r facts.ArchiveReady) *report.Report {
	rep := report.New("archive", c, rule.Archive(s, c.Role))
	rep.AddProbe(facts.ArchiveStatusID, s.Status, s.Reason, facts.ArchiveStatusColumns, rows(s.Rows), 0)
	rep.AddProbe(facts.ArchiveReadyID, r.Status, r.Reason, facts.ArchiveReadyColumns, rows(r.Rows), 0)
	rep.SetArchive(s)
	return rep
}
