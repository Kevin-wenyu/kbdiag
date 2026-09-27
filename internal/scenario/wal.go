package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// WAL only shows: slots and archiving are judged by their own commands, so
// the same problem is not reported three times.
func WAL(c facts.Context, p facts.WALPosition, w facts.SpaceWAL, s facts.SlotList, a facts.ArchiveReady) *report.Report {
	rep := report.New("wal", c, rule.Display(false, p.Status, w.Status, s.Status, a.Status))
	rep.AddProbe(facts.WALPositionID, p.Status, p.Reason, facts.WALPositionColumns, rows(p.Rows), 0)
	rep.AddProbe(facts.SpaceWALID, w.Status, w.Reason, facts.WALColumns, rows(w.Rows), 0)
	rep.AddProbe(facts.SlotListID, s.Status, s.Reason, facts.SlotColumns, rows(s.Rows), 0)
	rep.AddProbe(facts.ArchiveReadyID, a.Status, a.Reason, facts.ArchiveReadyColumns, rows(a.Rows), 0)
	rep.SetWAL(p, w, s, a)
	return rep
}
