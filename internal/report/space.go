package report

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// spaceView is the space text layout (plan 2026-09-27 appendix B.1): the
// disks first (is one filling up), then WAL (the usual culprit), databases
// and tablespaces. Directories on one filesystem share a line.
type spaceView struct{ disk facts.SpaceDisk }

func (r *Report) SetSpace(d facts.SpaceDisk) {
	v := &spaceView{disk: d}
	r.layout = func(r *Report, w io.Writer) error { return v.write(r, w) }
}

func (v *spaceView) write(r *Report, w io.Writer) error {
	if err := v.writeDisk(w); err != nil {
		return err
	}
	for _, id := range []string{facts.SpaceWALID, facts.InstDatabasesID, facts.SpaceTablespacesID} {
		p := r.Data[id]
		if p.Status != facts.StatusOK {
			writeNotOK(w, strings.TrimPrefix(strings.TrimPrefix(id, "space."), "inst."), p)
			continue
		}
		var err error
		switch id {
		case facts.SpaceWALID:
			writeWAL(w, p)
		case facts.InstDatabasesID:
			err = writeDatabases(w, p, "databases")
		case facts.SpaceTablespacesID:
			err = writeTablespaces(w, p)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *spaceView) writeDisk(w io.Writer) error {
	if v.disk.Status != facts.StatusOK {
		writeNotOKAs(w, "disk", v.disk.Status, v.disk.Reason)
		return nil
	}
	type fs struct {
		holds []string
		d     facts.Disk
	}
	var order []string
	byID := map[string]*fs{}
	for _, m := range v.disk.Rows {
		id := m.FSID
		if id == "" { // no id: never merge
			id = "path:" + m.Path
		}
		f, ok := byID[id]
		if !ok {
			f = &fs{d: m.Disk}
			byID[id] = f
			order = append(order, id)
		}
		what := m.Kind
		if m.Kind == "tablespace" {
			what = "tablespace " + escapeControl(m.Path)
		}
		f.holds = append(f.holds, what)
	}
	fmt.Fprintf(w, "\ndisk: %s\n", plural(len(order), "filesystem", "filesystems"))
	var rows [][]string
	for _, id := range order {
		f := byID[id]
		use := "-"
		if f.d.UsedBytes+f.d.AvailBytes > 0 { // as df: of what non-root users can fill
			use = fmt.Sprintf("%.0f%%", math.Round(float64(f.d.UsedBytes)*100/float64(f.d.UsedBytes+f.d.AvailBytes)))
		}
		rows = append(rows, []string{strings.Join(f.holds, ", "), size(float64(f.d.UsedBytes)), size(float64(f.d.TotalBytes)), size(float64(f.d.AvailBytes)), use})
	}
	return writeTable(w, "  ", []string{"holds", "used", "size", "free", "use"}, rows)
}

// writeWAL gives the settings WAL is measured against, and points at who
// else keeps it once it is past their sum: max_wal_size is soft, so only
// well beyond it is a hint worth giving.
func writeWAL(w io.Writer, p Probe) {
	for _, row := range p.rows() {
		files, _ := number(row["files"])
		bytes, _ := number(row["bytes"])
		fmt.Fprintf(w, "\nwal: %s, %s\n", plural(int(files), "file", "files"), size(bytes))
		var kv [][2]string
		limit := 0.0
		if v, ok := number(row["max_wal_size_bytes"]); ok {
			kv = append(kv, [2]string{"max_wal_size", size(v)})
			limit = v
		}
		if v, ok := number(row["wal_keep_bytes"]); ok {
			keep := size(v)
			if seg, ok := number(row["wal_segment_bytes"]); ok && seg > 0 {
				keep += fmt.Sprintf("  (%.0f x %s)", v/seg, size(seg))
			}
			kv = append(kv, [2]string{"wal_keep_segments", keep})
			limit += v
		}
		writeKV(w, 0, kv)
		if len(kv) > 0 && bytes > limit {
			fmt.Fprintln(w, "  more than max_wal_size plus wal_keep_segments: see kbdiag slots (replication slots), kbdiag archive (failed archiving)")
		}
	}
}

func writeTablespaces(w io.Writer, p Probe) error {
	fmt.Fprintf(w, "\ntablespaces: %d\n", len(p.Rows))
	if len(p.Rows) == 0 {
		return nil
	}
	var rows [][]string
	for _, row := range p.rows() {
		loc := "(in data_directory)"
		if !isNil(row["location"]) && cell(row["location"]) != "" {
			loc = escapeControl(*row["location"].(*string)) // a path: never cut
		}
		sz := "?"
		if v, ok := number(row["size_bytes"]); ok {
			sz = size(v)
		}
		rows = append(rows, []string{name(row["spcname"]), loc, sz})
	}
	return writeTable(w, "  ", []string{"name", "location", "size"}, rows)
}
