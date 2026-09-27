package report

import (
	"fmt"
	"io"
	"math"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// checkpointView is the checkpoint text layout (plan 2026-09-27 appendix
// B.13): the last checkpoint, what the counters say since their reset,
// then the settings that shape them.
type checkpointView struct {
	s   facts.CheckpointStats
	l   facts.CheckpointLast
	set facts.CheckpointSettings
}

func (r *Report) SetCheckpoint(s facts.CheckpointStats, l facts.CheckpointLast, set facts.CheckpointSettings) {
	v := &checkpointView{s: s, l: l, set: set}
	r.layout = func(r *Report, w io.Writer) error { return v.write(r, w) }
}

func (v *checkpointView) write(r *Report, w io.Writer) error {
	what := "checkpoint"
	if r.Context.Role == "standby" {
		what = "restartpoint" // a standby's checkpoints
	}
	if v.l.Status != facts.StatusOK || len(v.l.Rows) == 0 {
		writeNotOKAs(w, "last "+what, v.l.Status, v.l.Reason)
	} else {
		l := v.l.Rows[0]
		at := "-"
		if l.Time != nil {
			at = l.Time.Format(time.DateTime)
			if l.AgeS != nil {
				at += "  (" + duration(*l.AgeS) + " ago)"
			}
		}
		redo := cell(l.RedoLSN)
		if l.RedoWALFile != nil {
			redo += "  (" + escapeControl(*l.RedoWALFile) + ")"
		}
		fmt.Fprintf(w, "\nlast %s\n", what)
		writeKV(w, 0, [][2]string{{"time", at}, {"redo", redo}})
	}
	if v.s.Status != facts.StatusOK || len(v.s.Rows) == 0 {
		writeNotOKAs(w, "statistics", v.s.Status, v.s.Reason)
	} else {
		b := v.s.Rows[0]
		since := "since the statistics reset"
		if b.StatsReset != nil {
			since += " " + b.StatsReset.Format(time.DateTime)
			if b.StatsResetAgeS != nil {
				since += " (" + duration(*b.StatsResetAgeS) + " ago)"
			}
		}
		if what == "restartpoint" {
			since += "; on a standby these are restartpoints"
		}
		total := b.CheckpointsTimed + b.CheckpointsReq
		written := b.BuffersCheckpoint + b.BuffersClean + b.BuffersBackend
		fmt.Fprintf(w, "\n%s\n", since)
		writeKV(w, 0, [][2]string{
			{"checkpoints", fmt.Sprintf("%d: %d timed, %d requested (%s forced by WAL volume or by hand)", total, b.CheckpointsTimed, b.CheckpointsReq, pct(b.CheckpointsReq, total))},
			{"write, sync time", execTime(b.CheckpointWriteS) + ", " + execTime(b.CheckpointSyncS)},
			{"buffers written", fmt.Sprintf("%d: checkpointer %d (%s), bgwriter %d (%s), backends %d (%s)", written,
				b.BuffersCheckpoint, pct(b.BuffersCheckpoint, written), b.BuffersClean, pct(b.BuffersClean, written), b.BuffersBackend, pct(b.BuffersBackend, written))},
			{"backend fsyncs", fmt.Sprint(b.BuffersBackendFsync)},
			{"bgwriter stopped", fmt.Sprintf("%d times at bgwriter_lru_maxpages", b.MaxwrittenClean)},
		})
	}
	if v.set.Status != facts.StatusOK || len(v.set.Rows) == 0 {
		writeNotOKAs(w, "settings", v.set.Status, v.set.Reason)
		return nil
	}
	s := v.set.Rows[0]
	fmt.Fprintln(w, "\nsettings")
	writeKV(w, 0, [][2]string{
		{"checkpoint_timeout", duration(float64(s.TimeoutS))},
		{"max_wal_size", size(float64(s.MaxWALSizeBytes))},
		{"checkpoint_completion_target", fmt.Sprint(s.CompletionTarget)},
		{"checkpoint_warning", duration(float64(s.WarningS))},
		{"log_checkpoints", escapeControl(s.LogCheckpoints)},
	})
	return nil
}

// pct is n of total, rounded; "-" when there is no total.
func pct(n, total int64) string {
	if total <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", math.Round(float64(n)*100/float64(total)))
}
