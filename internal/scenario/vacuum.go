package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

type VacuumOptions struct{ Limit int }

// Vacuum judges every table; --limit only narrows the tables shown.
func Vacuum(c facts.Context, t facts.VacuumTables, p facts.VacuumProgress, s facts.VacuumSettings, o VacuumOptions) *report.Report {
	rep := report.New("vacuum", c, rule.Vacuum(t, s, c.Role, c.Database))
	rep.AddProbe(facts.VacuumSettingsID, s.Status, s.Reason, facts.VacuumSettingColumns, rows(s.Rows), 0)
	rep.AddProbe(facts.VacuumProgressID, p.Status, p.Reason, facts.VacuumProgressColumns, rows(p.Rows), 0)
	rep.AddProbe(facts.VacuumTablesID, t.Status, t.Reason, facts.VacuumTableColumns, rows(t.Rows), o.Limit)
	rep.AddRedacted(p.Redacted())
	var set facts.VacuumSetting
	if s.Status == facts.StatusOK && len(s.Rows) > 0 {
		set = s.Rows[0]
	}
	rep.SetVacuum(t, set, o.Limit)
	return rep
}
