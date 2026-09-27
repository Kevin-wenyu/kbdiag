package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

type FreezeOptions struct{ Limit int }

// Freeze judges every database; --limit only narrows the tables shown.
func Freeze(c facts.Context, d facts.FreezeDatabases, t facts.FreezeTables, l facts.FreezeLimits, o FreezeOptions) *report.Report {
	rep := report.New("freeze", c, rule.Freeze(d, l, c.Role))
	rep.AddProbe(facts.FreezeLimitsID, l.Status, l.Reason, facts.FreezeLimitColumns, rows(l.Rows), 0)
	rep.AddProbe(facts.FreezeDatabasesID, d.Status, d.Reason, facts.FreezeDatabaseColumns, rows(d.Rows), 0)
	rep.AddProbe(facts.FreezeTablesID, t.Status, t.Reason, facts.FreezeTableColumns, rows(t.Rows), o.Limit)
	rep.SetFreeze()
	return rep
}
