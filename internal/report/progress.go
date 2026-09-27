package report

import (
	"fmt"
	"io"
	"math"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// progressView is the progress text layout (plan 2026-09-27 appendix
// B.12): one line per operation, longest running first.
type progressView struct{ p, cp facts.ProgressList }

func (r *Report) SetProgress(p, cp facts.ProgressList) {
	v := &progressView{p: p, cp: cp}
	r.layout = func(r *Report, w io.Writer) error { return v.write(w) }
}

func (v *progressView) write(w io.Writer) error {
	var ops []facts.Operation
	for _, p := range []struct {
		id string
		l  facts.ProgressList
	}{{"running", v.p}, {"checkpoint", v.cp}} {
		if p.l.Status != facts.StatusOK {
			writeNotOKAs(w, p.id, p.l.Status, p.l.Reason)
			continue
		}
		ops = append(ops, p.l.Rows...)
	}
	if len(ops) == 0 {
		fmt.Fprintln(w, "\nrunning: 0  (VACUUM, CREATE INDEX, CLUSTER and VACUUM FULL, CHECKPOINT; ANALYZE and base backups have no progress view in this version)")
		return nil
	}
	fmt.Fprintf(w, "\nrunning: %d\n", len(ops))
	var rows [][]string
	for _, o := range ops {
		phase := "?"
		if o.Phase != nil {
			phase = escapeControl(*o.Phase)
			if o.WaitingLockers != nil && *o.WaitingLockers > 0 {
				phase += fmt.Sprintf(" (%d transactions left: kbdiag locks)", *o.WaitingLockers)
			}
		}
		prog := "-"
		switch {
		case o.Phase == nil:
			prog = "?"
		case o.Done != nil && o.Total != nil:
			prog = fmt.Sprintf("%d / %d %s", *o.Done, *o.Total, o.Unit)
			if *o.Total > 0 {
				prog += fmt.Sprintf(" (%.0f%%)", math.Floor(float64(*o.Done)*100/float64(*o.Total)))
			}
		case o.Done != nil:
			prog = fmt.Sprintf("%d %s", *o.Done, o.Unit)
		}
		rows = append(rows, []string{fmt.Sprint(o.PID), escapeControl(o.Command), name(o.Datname), fitWidth(cell(o.Relation), 2*maxName), phase, prog, running(o.RunningS)})
	}
	return writeTable(w, "  ", []string{"pid", "command", "database", "relation", "phase", "progress", "running"}, rows)
}
