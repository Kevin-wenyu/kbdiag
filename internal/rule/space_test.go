package rule

import (
	"reflect"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func TestSpaceDiskFull(t *testing.T) {
	const mb = 1 << 20
	seg := int64(16 * mb)
	d := facts.InstDatabases{Status: facts.StatusOK}
	ts := facts.SpaceTablespaces{Status: facts.StatusOK}
	wal := facts.SpaceWAL{Status: facts.StatusOK, Rows: []facts.WAL{{WALSegmentBytes: &seg}}}
	disk := func(avail uint64, fsid string) facts.Disk {
		return facts.Disk{TotalBytes: 200 << 30, UsedBytes: 200<<30 - avail, AvailBytes: avail, FSID: fsid}
	}
	mounts := func(ms ...facts.Mount) facts.SpaceDisk { return facts.SpaceDisk{Status: facts.StatusOK, Rows: ms} }
	cases := []struct {
		name  string
		w     facts.SpaceWAL
		disk  facts.SpaceDisk
		want  Verdict
		paths [][]string // one finding per filesystem
	}{
		{"one filesystem, 12 MB free", wal, mounts(
			facts.Mount{Kind: "data_directory", Path: "/data", Disk: disk(12*mb, "1")},
			facts.Mount{Kind: "wal", Path: "/data/sys_wal", Disk: disk(12*mb, "1")}),
			VerdictFAIL, [][]string{{"/data", "/data/sys_wal"}}},
		{"exactly one segment left", wal, mounts(
			facts.Mount{Kind: "data_directory", Path: "/data", Disk: disk(16*mb, "1")}),
			VerdictOK, nil},
		{"only the WAL disk is full", wal, mounts(
			facts.Mount{Kind: "data_directory", Path: "/data", Disk: disk(50<<30, "1")},
			facts.Mount{Kind: "wal", Path: "/wal/sys_wal", Disk: disk(0, "2")}),
			VerdictFAIL, [][]string{{"/wal/sys_wal"}}},
		// a full tablespace disk stops writes to its tables, not the server:
		// shown, not judged
		{"tablespace disk full", wal, mounts(
			facts.Mount{Kind: "data_directory", Path: "/data", Disk: disk(50<<30, "1")},
			facts.Mount{Kind: "tablespace", Path: "/ts1", Disk: disk(0, "3")}),
			VerdictOK, nil},
		// no filesystem id: never merged
		{"no fsid", wal, mounts(
			facts.Mount{Kind: "data_directory", Path: "/data", Disk: disk(1*mb, "")},
			facts.Mount{Kind: "wal", Path: "/data/sys_wal", Disk: disk(1*mb, "")}),
			VerdictFAIL, [][]string{{"/data"}, {"/data/sys_wal"}}},
		// kbdiag_ro: sys_ls_waldir is denied, so the segment size is unknown
		{"segment size not collected", facts.SpaceWAL{Status: facts.StatusSkipped}, mounts(
			facts.Mount{Kind: "data_directory", Path: "/data", Disk: disk(0, "1")}),
			VerdictUNKNOWN, nil},
		// the only judged line could not be checked: not OK
		{"remote", wal, facts.SpaceDisk{Status: facts.StatusNotApplicable, Reason: "remote connection"}, VerdictUNKNOWN, nil},
		// a tablespace failed statfs: the data filesystem is still judged
		{"tablespace failed, data full", wal, facts.SpaceDisk{Status: facts.StatusError, Reason: "statfs /ts/one: no such file or directory",
			Rows: []facts.Mount{{Kind: "data_directory", Path: "/data", Disk: disk(1*mb, "1")}}}, VerdictFAIL, [][]string{{"/data"}}},
		{"tablespace failed, data fine", wal, facts.SpaceDisk{Status: facts.StatusError, Reason: "statfs /ts/one: no such file or directory",
			Rows: []facts.Mount{{Kind: "data_directory", Path: "/data", Disk: disk(50<<30, "1")}}}, VerdictUNKNOWN, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Space(d, ts, c.w, c.disk)
			if r.Verdict != c.want {
				t.Errorf("verdict = %s, want %s", r.Verdict, c.want)
			}
			if len(r.Findings) != len(c.paths) {
				t.Fatalf("findings = %+v, want %d", r.Findings, len(c.paths))
			}
			for i, f := range r.Findings {
				if f.ID != "space.disk_full" || f.Level != LevelFAIL || len(f.Evidence) != 1 || f.Evidence[0].ProbeID != facts.SpaceDiskID {
					t.Fatalf("finding = %+v", f)
				}
				ev := f.Evidence[0].Fields
				if !reflect.DeepEqual(ev["paths"], c.paths[i]) || ev["wal_segment_bytes"] != seg {
					t.Errorf("evidence = %v", ev)
				}
				if len(f.Next) != 2 || f.Next[0].Command != "kbdiag wal" || f.Next[1].Command != "kbdiag top-objects" {
					t.Errorf("next = %+v", f.Next)
				}
			}
		})
	}
	r := Space(d, ts, wal, mounts(facts.Mount{Kind: "data_directory", Path: "/data", Disk: disk(12*mb, "1")}, facts.Mount{Kind: "wal", Path: "/data/sys_wal", Disk: disk(12*mb, "1")}))
	want := "the filesystem holding data_directory and wal has 12 MB free, less than one WAL segment (16 MB): tables, transaction status files and temp files are about to fail to grow; a new WAL segment cannot be created, so the server stops once no old segment is left to reuse"
	if r.Findings[0].Symptom != want {
		t.Errorf("symptom = %q", r.Findings[0].Symptom)
	}
	// sys_wal on another disk: the data filesystem alone does not stop the server
	r = Space(d, ts, wal, mounts(facts.Mount{Kind: "data_directory", Path: "/data", Disk: disk(12*mb, "1")}, facts.Mount{Kind: "wal", Path: "/wal", Disk: disk(50<<30, "2")}))
	want = "the filesystem holding data_directory has 12 MB free, less than one WAL segment (16 MB): tables, transaction status files and temp files are about to fail to grow"
	if len(r.Findings) != 1 || r.Findings[0].Symptom != want {
		t.Errorf("findings = %+v", r.Findings)
	}
}
