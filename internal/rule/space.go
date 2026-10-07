package rule

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/units"
)

// Display is the verdict of a command that only shows: OK says every probe
// was collected and nothing was hidden, never that the numbers are fine; a
// probe not collected or a hidden column makes it UNKNOWN.
func Display(hidden bool, sts ...facts.Status) Result {
	unknown := hidden
	for _, st := range sts {
		_, x := collected(st)
		unknown = unknown || x
	}
	return Result{Verdict: verdictOf(nil, unknown)}
}

// Space shows the space account and judges one line (user 2026-09-27): a
// filesystem holding data_directory or the WAL directory with less free
// space than one WAL segment is FAIL. On the WAL filesystem the server can
// no longer create a segment, only reuse old ones, and stops when they run
// out; on the data filesystem tables, SLRU files and temp files are about
// to fail with ERROR (the server keeps running).
// Everything else is shown only: how much is too little otherwise depends
// on the database. A full tablespace disk stops writes to its tables, not
// the server, so it is shown only. The segment size comes from space.wal,
// which kbdiag_ro cannot collect: then the line is not judged (UNKNOWN).
// Over a remote connection the disks are not_applicable, and the line is
// not judged either, so that is UNKNOWN too, not OK. A tablespace that
// could not be statted leaves the data and WAL rows to judge.
func Space(d facts.InstDatabases, t facts.SpaceTablespaces, w facts.SpaceWAL, disk facts.SpaceDisk) Result {
	hidden := len(d.Redacted())+len(t.Redacted()) > 0 || disk.Status == facts.StatusNotApplicable
	r := Display(hidden, d.Status, t.Status, w.Status, disk.Status)
	if len(disk.Rows) == 0 || w.Status != facts.StatusOK || len(w.Rows) == 0 || w.Rows[0].WALSegmentBytes == nil {
		return r
	}
	seg := *w.Rows[0].WALSegmentBytes
	type filesystem struct {
		kinds, paths []string
		d            facts.Disk
	}
	var order []string
	byID := map[string]*filesystem{}
	for _, m := range disk.Rows {
		if m.Kind != "data_directory" && m.Kind != "wal" {
			continue
		}
		id := m.FSID
		if id == "" { // no id: never merged, as in the text
			id = "path:" + m.Path
		}
		f, ok := byID[id]
		if !ok {
			f = &filesystem{d: m.Disk}
			byID[id] = f
			order = append(order, id)
		}
		f.kinds = append(f.kinds, m.Kind)
		f.paths = append(f.paths, m.Path)
	}
	var fs []Finding
	for _, id := range order {
		f := byID[id]
		if seg <= 0 || f.d.AvailBytes >= uint64(seg) {
			continue
		}
		var harm []string
		if slices.Contains(f.kinds, "data_directory") {
			harm = append(harm, "tables, transaction status files and temp files are about to fail to grow")
		}
		if slices.Contains(f.kinds, "wal") {
			harm = append(harm, "a new WAL segment cannot be created, so the server stops once no old segment is left to reuse")
		}
		fs = append(fs, Finding{
			ID:    "space.disk_full",
			Level: LevelFAIL,
			Symptom: fmt.Sprintf("the filesystem holding %s has %s free, less than one WAL segment (%s): %s",
				strings.Join(f.kinds, " and "), units.Bytes(float64(f.d.AvailBytes)), units.Bytes(float64(seg)), strings.Join(harm, "; ")),
			Evidence: []Evidence{{ProbeID: facts.SpaceDiskID, Fields: map[string]any{
				"paths": f.paths, "avail_bytes": f.d.AvailBytes, "total_bytes": f.d.TotalBytes, "wal_segment_bytes": seg,
			}}},
			Next: []Next{
				{Kind: "verify", Command: "kbdiag wal", Note: "what keeps WAL here: slots, wal_keep_segments, archiving"},
				{Kind: "verify", Command: "kbdiag top-objects", Note: "the largest tables and indexes in this database"},
			},
		})
	}
	if len(fs) == 0 {
		return r
	}
	return Result{Verdict: VerdictFAIL, Findings: fs}
}
