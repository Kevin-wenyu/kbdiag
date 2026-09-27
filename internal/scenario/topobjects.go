package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

type TopObjectsOptions struct{ Limit int }

// TopObjects only shows; --limit narrows both lists.
func TopObjects(c facts.Context, t facts.ObjectTables, i facts.ObjectIndexes, o TopObjectsOptions) *report.Report {
	rep := report.New("top-objects", c, rule.Display(false, t.Status, i.Status))
	rep.AddProbe(facts.ObjectTablesID, t.Status, t.Reason, facts.ObjectTableColumns, rows(t.Rows), o.Limit)
	rep.AddProbe(facts.ObjectIndexesID, i.Status, i.Reason, facts.ObjectIndexColumns, rows(i.Rows), o.Limit)
	rep.SetTopObjects()
	return rep
}
