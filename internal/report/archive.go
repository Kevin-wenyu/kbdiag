package report

import (
	"fmt"
	"io"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// archiveView is the archive text layout (plan 2026-09-27 appendix B.4):
// the settings, the archiver's two counters with their last WAL, then what
// waits in archive_status.
type archiveView struct{ s facts.ArchiveStatus }

func (r *Report) SetArchive(s facts.ArchiveStatus) {
	v := &archiveView{s: s}
	r.layout = func(r *Report, w io.Writer) error { return v.write(r, w) }
}

func (v *archiveView) write(r *Report, w io.Writer) error {
	if v.s.Status != facts.StatusOK || len(v.s.Rows) == 0 {
		writeNotOKAs(w, "archiver", v.s.Status, v.s.Reason)
	} else {
		a := v.s.Rows[0]
		cmd := "(empty)"
		if a.Command != nil && *a.Command != "" {
			cmd = escapeControl(*a.Command)
		}
		timeout := fmt.Sprintf("%ds", a.TimeoutS)
		if a.TimeoutS == 0 {
			timeout = "0 (off)"
		}
		mode := escapeControl(a.Mode)
		switch {
		case a.Mode == "off":
			mode += "  (archiving is off)"
		case r.Context.Role == "standby" && a.Mode != "always":
			mode += "  (a standby archives only with always)"
		}
		fmt.Fprintln(w, "\nsettings")
		kv := [][2]string{{"archive_mode", mode}, {"archive_timeout", timeout}}
		if a.Mode != "off" {
			kv = [][2]string{kv[0], {"archive_command", cmd}, kv[1]}
		}
		writeKV(w, 0, kv)
		since := ""
		if a.StatsReset != nil {
			since = "  (counting since " + a.StatsReset.Format(time.DateTime) + ")"
		}
		fmt.Fprintf(w, "\narchiver%s\n", since)
		rows := [][]string{archiverRow("archived", a.ArchivedCount, a.LastArchivedWAL, a.LastArchivedAgeS), archiverRow("failed", a.FailedCount, a.LastFailedWAL, a.LastFailedAgeS)}
		if err := writeTable(w, "  ", nil, rows); err != nil {
			return err
		}
	}
	p := r.Data[facts.ArchiveReadyID]
	if p.Status != facts.StatusOK {
		writeNotOKAs(w, "waiting", p.Status, reasonOf(p))
		return nil
	}
	for _, row := range p.rows() {
		fmt.Fprintf(w, "\nwaiting: %s .ready, %s .done", cell(row["ready"]), cell(row["done"]))
		if s, ok := number(row["oldest_ready_age_s"]); ok {
			fmt.Fprintf(w, "  (oldest .ready %s)", duration(s))
		}
		fmt.Fprintln(w)
	}
	return nil
}

func archiverRow(what string, n int64, wal *string, age *float64) []string {
	row := []string{what, fmt.Sprint(n)}
	if wal != nil {
		row = append(row, "last "+escapeControl(*wal), ago(age))
	}
	return row
}
