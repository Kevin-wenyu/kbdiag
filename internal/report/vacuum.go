package report

import (
	"fmt"
	"io"
	"strconv"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// vacuumView is the vacuum text layout (plan 2026-09-27 appendix B.3): the
// switches, what is vacuuming now, then the tables with the most dead
// tuples and where autovacuum's line is for each.
type vacuumView struct {
	tables facts.VacuumTables
	set    facts.VacuumSetting
	limit  int
}

func (r *Report) SetVacuum(t facts.VacuumTables, s facts.VacuumSetting, limit int) {
	v := &vacuumView{tables: t, set: s, limit: limit}
	r.layout = func(r *Report, w io.Writer) error { return v.write(r, w) }
}

func (v *vacuumView) write(r *Report, w io.Writer) error {
	setOK := r.Data[facts.VacuumSettingsID].Status == facts.StatusOK
	if p := r.Data[facts.VacuumSettingsID]; !setOK {
		writeNotOKAs(w, "settings", p.Status, reasonOf(p))
	} else {
		s := v.set
		fmt.Fprintln(w, "\nsettings")
		writeKV(w, 0, [][2]string{
			{"autovacuum", fmt.Sprintf("%s  (%d workers, naptime %s)", escapeControl(s.Autovacuum), s.MaxWorkers, duration(float64(s.NaptimeS)))},
			{"track_counts", escapeControl(s.TrackCounts)},
			{"threshold", fmt.Sprintf("%d + %g x reltuples", s.Threshold, s.ScaleFactor)},
		})
	}
	if err := writeVacuumRuns(w, r.Data[facts.VacuumProgressID]); err != nil {
		return err
	}
	title := "tables in " + escapeControl(r.Context.Database)
	if r.Context.Database == "" {
		title = "tables"
	}
	if v.tables.Status != facts.StatusOK {
		writeNotOKAs(w, title, v.tables.Status, v.tables.Reason)
		return nil
	}
	fmt.Fprintf(w, "\n%s: %d, most dead tuples first\n", title, len(v.tables.Rows))
	shown := v.tables.Rows
	if v.limit > 0 && len(shown) > v.limit {
		shown = shown[:v.limit]
	}
	if len(shown) == 0 {
		return nil
	}
	var rows [][]string
	for _, x := range shown {
		threshold, due, auto := "?", "?", "?"
		if setOK {
			th, on := rule.VacuumThreshold(x, v.set)
			threshold, due, auto = strconv.FormatFloat(float64(th), 'f', -1, 32), "-", "on"
			if float32(x.NDeadTup) > th {
				due = "yes"
			}
			if !on || v.set.Autovacuum != "on" || v.set.TrackCounts != "on" {
				auto = "off"
			}
		}
		rows = append(rows, []string{fitWidth(escapeControl(x.Schemaname+"."+x.Relname), 2*maxName), fmt.Sprint(x.NDeadTup), fmt.Sprint(x.NLiveTup),
			threshold, due, auto, ago(x.LastAutovacuumAgeS), ago(x.LastVacuumAgeS)})
	}
	if err := writeTable(w, "  ", []string{"table", "dead", "live", "threshold", "due", "autovacuum", "last autovacuum", "last vacuum"}, rows); err != nil {
		return err
	}
	writeTruncated(w, len(v.tables.Rows)-len(shown))
	return nil
}

func writeVacuumRuns(w io.Writer, p Probe) error {
	if p.Status != facts.StatusOK {
		writeNotOKAs(w, "running", p.Status, reasonOf(p))
		return nil
	}
	fmt.Fprintf(w, "\nrunning: %d\n", len(p.Rows))
	if len(p.Rows) == 0 {
		return nil
	}
	var rows [][]string
	for _, row := range p.rows() {
		kind := "?"
		if b, ok := row["is_autovacuum"].(*bool); ok && b != nil {
			kind = map[bool]string{true: "autovacuum", false: "manual"}[*b]
		}
		scanned := "?"
		if n, ok := number(row["heap_blks_scanned"]); ok {
			scanned = fmt.Sprintf("%.0f", n)
			if tot, ok := number(row["heap_blks_total"]); ok && tot > 0 {
				scanned += fmt.Sprintf(" / %.0f blocks (%.0f%%)", tot, n*100/tot)
			}
		}
		phase := "?"
		if !isNil(row["phase"]) {
			phase = cell(row["phase"])
		}
		rows = append(rows, []string{cell(row["pid"]), kind, name(row["datname"]), fitWidth(cell(row["relation"]), 2*maxName), phase, scanned, running(row["xact_age_s"])})
	}
	return writeTable(w, "  ", []string{"pid", "kind", "database", "relation", "phase", "scanned", "running"}, rows)
}
