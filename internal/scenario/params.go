package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

func Params(c facts.Context, p facts.ParamsChanged) *report.Report {
	rep := report.New("params", c, rule.Params(p))
	rep.AddProbe(facts.ParamsChangedID, p.Status, p.Reason, facts.ParamColumns, rows(p.Rows), 0)
	rep.AddRedacted(p.Redacted())
	rep.SetParams(p)
	return rep
}
