package report

import (
	"fmt"
	"io"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// walView is the wal text layout (plan 2026-09-27 appendix B.14): where
// WAL is, how much sys_wal holds, then everything that keeps it here.
type walView struct {
	p facts.WALPosition
	w facts.SpaceWAL
	s facts.SlotList
	a facts.ArchiveReady
}

func (r *Report) SetWAL(p facts.WALPosition, w facts.SpaceWAL, s facts.SlotList, a facts.ArchiveReady) {
	v := &walView{p: p, w: w, s: s, a: a}
	r.layout = func(r *Report, w io.Writer) error { return v.write(w) }
}

func (v *walView) write(w io.Writer) error {
	if v.p.Status != facts.StatusOK || len(v.p.Rows) == 0 {
		writeNotOKAs(w, "position", v.p.Status, v.p.Reason)
	} else {
		p := v.p.Rows[0]
		lsn := "lsn"
		if p.InRecovery {
			lsn = "replayed" // a standby has no insert position
		}
		kv := [][2]string{{lsn, cell(p.LSN)}}
		if p.WALFile != nil {
			kv = append(kv, [2]string{"file", escapeControl(*p.WALFile)})
		}
		fmt.Fprintln(w, "\nposition")
		writeKV(w, 0, kv)
	}
	if v.w.Status != facts.StatusOK || len(v.w.Rows) == 0 {
		writeNotOKAs(w, "sys_wal", v.w.Status, v.w.Reason)
	} else {
		x := v.w.Rows[0]
		fmt.Fprintf(w, "\nsys_wal: %s, %s\n", plural(int(x.Files), "file", "files"), size(float64(x.Bytes)))
	}
	var kv [][2]string
	if v.w.Status == facts.StatusOK && len(v.w.Rows) > 0 {
		x := v.w.Rows[0]
		if x.MaxWALSizeBytes != nil {
			kv = append(kv, [2]string{"max_wal_size", size(float64(*x.MaxWALSizeBytes))})
		}
		if x.WALKeepBytes != nil {
			keep := size(float64(*x.WALKeepBytes))
			if x.WALSegmentBytes != nil && *x.WALSegmentBytes > 0 {
				keep += fmt.Sprintf("  (%d x %s)", *x.WALKeepBytes / *x.WALSegmentBytes, size(float64(*x.WALSegmentBytes)))
			}
			kv = append(kv, [2]string{"wal_keep_segments", keep})
		}
	}
	if v.s.Status == facts.StatusOK {
		for _, s := range v.s.Rows {
			kept := "-"
			if s.RetainedWALBytes != nil {
				kept = size(max(0, float64(*s.RetainedWALBytes)))
			}
			state := "active"
			if !s.Active {
				state = "inactive: kbdiag slots"
			}
			kv = append(kv, [2]string{"slot " + escapeControl(s.Name), kept + "  (" + state + ")"})
		}
	} else {
		kv = append(kv, [2]string{"slots", "? (" + string(v.s.Status) + ")"})
	}
	if v.a.Status == facts.StatusOK && len(v.a.Rows) > 0 {
		n := v.a.Rows[0].Ready
		line := fmt.Sprintf("%d .ready %s waiting", n, map[bool]string{true: "file", false: "files"}[n == 1])
		if n > 0 {
			line += "  (kbdiag archive)"
		}
		kv = append(kv, [2]string{"archiving", line})
	} else {
		kv = append(kv, [2]string{"archiving", "? (" + string(v.a.Status) + ")"})
	}
	fmt.Fprintln(w, "\nwhat keeps WAL here")
	writeKV(w, 0, kv)
	return nil
}
