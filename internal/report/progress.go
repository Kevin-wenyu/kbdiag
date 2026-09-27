package report

import (
	"fmt"
	"io"
	"math"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// progressView is the progress text layout (plan 2026-09-27 appendix
// B.12): one line per operation, longest running first.
type progressView struct{ p facts.ProgressList }

func (r *Report) SetProgress(p facts.ProgressList) {
	v := &progressView{p: p}
	r.layout = func(r *Report, w io.Writer) error { return v.write(w) }
}

func (v *progressView) write(w io.Writer) error {
	if v.p.Status != facts.StatusOK {
		writeNotOKAs(w, "running", v.p.Status, v.p.Reason)
		return nil
	}
	if len(v.p.Rows) == 0 {
		fmt.Fprintln(w, "\nrunning: 0  (VACUUM, CREATE INDEX, CLUSTER and VACUUM FULL, CHECKPOINT; ANALYZE and base backups have no progress view in this version)")
		return nil
	}
	fmt.Fprintf(w, "\nrunning: %d\n", len(v.p.Rows))
	var rows [][]string
	for _, o := range v.p.Rows {
		phase := "?"
		if o.Phase != nil {
			phase = escapeControl(*o.Phase)
			if o.WaitingLockers != nil && *o.WaitingLockers > 0 {
				phase += fmt.Sprintf(" (%d transactions left: kbdiag locks)", *o.WaitingLockers)
			}
		}
		prog := "?"
		if o.Done != nil && o.Total != nil {
			prog = fmt.Sprintf("%d / %d %s", *o.Done, *o.Total, o.Unit)
			if *o.Total > 0 {
				prog += fmt.Sprintf(" (%.0f%%)", math.Floor(float64(*o.Done)*100/float64(*o.Total)))
			}
		}
		rows = append(rows, []string{fmt.Sprint(o.PID), escapeControl(o.Command), name(o.Datname), fitWidth(cell(o.Relation), 2*maxName), phase, prog, running(o.RunningS)})
	}
	return writeTable(w, "  ", []string{"pid", "command", "database", "relation", "phase", "progress", "running"}, rows)
}
