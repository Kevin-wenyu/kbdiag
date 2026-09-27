package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// TableKinds are the relkinds table accepts: tables, partitioned tables,
// materialized views, TOAST tables.
var TableKinds = map[string]bool{"r": true, "p": true, "m": true, "t": true}

// TableAfter is what the probes after table.info report when it did not
// name a table: not collected when table.info itself failed, not
// applicable when there is no such table.
func TableAfter(i facts.TableInfos) (facts.Status, string) {
	if i.Status != facts.StatusOK {
		return facts.StatusSkipped, "table.info was not collected"
	}
	return facts.StatusNotApplicable, "no such table"
}

// Table builds the table report. found is false when the name resolved to
// no relation, or to one that is not a table: the verdict is then UNKNOWN,
// as for a session pid that is gone.
func Table(c facts.Context, i facts.TableInfos, z facts.TableSizes, s facts.TableStats, x facts.TableIndexes, l facts.FreezeLimits, v facts.VacuumSettings) (*report.Report, bool) {
	found := i.Status != facts.StatusOK || (len(i.Rows) == 1 && TableKinds[i.Rows[0].Relkind])
	r := rule.Result{Verdict: rule.VerdictUNKNOWN}
	if found {
		r = rule.Table(i, s, l, v, c.Role, c.Database)
	}
	rep := report.New("table", c, r)
	rep.AddProbe(facts.TableInfoID, i.Status, i.Reason, facts.TableInfoColumns, rows(i.Rows), 0)
	rep.AddProbe(facts.TableSizeID, z.Status, z.Reason, facts.TableSizeColumns, rows(z.Rows), 0)
	rep.AddProbe(facts.TableStatsID, s.Status, s.Reason, facts.TableStatsColumns, rows(s.Rows), 0)
	rep.AddProbe(facts.TableIndexesID, x.Status, x.Reason, facts.TableIndexColumns, rows(x.Rows), 0)
	rep.AddProbe(facts.FreezeLimitsID, l.Status, l.Reason, facts.FreezeLimitColumns, rows(l.Rows), 0)
	rep.AddProbe(facts.VacuumSettingsID, v.Status, v.Reason, facts.VacuumSettingColumns, rows(v.Rows), 0)
	rep.SetTable(i, z, s, x, l, v, found)
	return rep, found
}
