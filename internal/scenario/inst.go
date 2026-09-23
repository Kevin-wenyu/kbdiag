package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

func Waits(c facts.Context, w facts.WaitSummary) *report.Report {
	rep := report.New("waits", c, rule.Waits(w))
	rep.AddProbe(facts.WaitSummaryID, w.Status, w.Reason, facts.WaitColumns, rows(w.Rows), 0)
	rep.AddRedacted(w.Redacted())
	return rep
}

func Status(c facts.Context, i facts.InstInfo, d facts.InstDatabases, n facts.InstDownstreams, th rule.Thresholds) *report.Report {
	rep := report.New("status", c, rule.Status(i, d, n, th))
	rep.AddProbe(facts.InstInfoID, i.Status, i.Reason, facts.InfoColumns, rows(i.Rows), 0)
	rep.AddProbe(facts.InstDatabasesID, d.Status, d.Reason, facts.DatabaseColumns, rows(d.Rows), 0)
	down := make([][]any, len(n.Rows))
	for j, x := range n.Rows {
		down[j] = []any{x}
	}
	rep.AddProbe(facts.InstDownstreamsID, n.Status, n.Reason, facts.DownstreamsColumns, down, 0)
	rep.AddRedacted(d.Redacted())
	return rep
}

func Slots(c facts.Context, l facts.SlotList) *report.Report {
	rep := report.New("slots", c, rule.Slots(l))
	rep.AddProbe(facts.SlotListID, l.Status, l.Reason, facts.SlotColumns, rows(l.Rows), 0)
	return rep
}
