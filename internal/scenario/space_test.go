package scenario

import (
	"testing"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

func v02Context(role, user, location string) facts.Context {
	return facts.Context{Version: "KingbaseES V008R006C009B0014", Role: role, Location: location, User: user,
		CollectedAt: time.Date(2026, 9, 27, 21, 20, 9, 0, cst)}
}

// spaceFacts rebuilds space from the stage 0 captures of one node and user.
// The disk comes from the shell capture (df -k): data_directory and sys_wal
// are on one filesystem there.
func spaceFacts(t *testing.T, nodeUser string) (facts.InstDatabases, facts.SpaceTablespaces, facts.SpaceWAL) {
	t.Helper()
	d := facts.InstDatabases{Status: facts.StatusOK}
	for _, m := range loadKsql(t, "space_"+nodeUser+"_dbsize").rows {
		if *m["datname"] == "template0" || *m["datname"] == "template1" {
			continue // inst.databases leaves templates out
		}
		d.Rows = append(d.Rows, facts.Database{Datname: *m["datname"], SizeBytes: kI64(t, m["bytes"])})
	}
	ts := facts.SpaceTablespaces{Status: facts.StatusOK}
	tc := loadKsql(t, "space_"+nodeUser+"_tablespace")
	if tc.err != "" {
		// kbdiag_ro: the probe's CASE asks only for the sizes it may read;
		// the capture's plain query failed at sys_global instead.
		sys := loadKsql(t, "space_node1_sys_tablespace")
		for _, m := range sys.rows {
			x := facts.Tablespace{Name: *m["spcname"], Location: m["location"]}
			if x.Name == "sys_default" {
				x.SizeBytes = kI64(t, m["bytes"])
			}
			ts.Rows = append(ts.Rows, x)
		}
	} else {
		for _, m := range tc.rows {
			ts.Rows = append(ts.Rows, facts.Tablespace{Name: *m["spcname"], Location: m["location"], SizeBytes: kI64(t, m["bytes"])})
		}
	}
	w := facts.SpaceWAL{Status: facts.StatusOK}
	wc := loadKsql(t, "space_"+nodeUser+"_waldir")
	if wc.err != "" {
		w = facts.SpaceWAL{Status: facts.StatusSkipped, Reason: "insufficient_privilege 42501: " + wc.err}
	} else {
		m := wc.rows[0]
		seg, keep, maxWAL := int64(16777216), int64(512*16777216), int64(1024<<20)
		w.Rows = []facts.WAL{{Files: *kI64(t, m["files"]), Bytes: *kI64(t, m["bytes"]), MaxWALSizeBytes: &maxWAL, WALKeepBytes: &keep, WALSegmentBytes: &seg}}
	}
	return d, ts, w
}

// node1's filesystem from shell_node1.txt: df -k 208449516 total,
// 14756708 used, 193692808 available.
func node1Disk(dir string) facts.SpaceDisk {
	d := facts.Disk{TotalBytes: 208449516 * 1024, UsedBytes: 14756708 * 1024, AvailBytes: 193692808 * 1024, FSID: "[1 2]"}
	return facts.SpaceDisk{Status: facts.StatusOK, Rows: []facts.Mount{{Kind: "data_directory", Path: dir, Disk: d}, {Kind: "wal", Path: dir + "/sys_wal", Disk: d}}}
}

// The goldens are the stage 2 drafts, written by hand from the captures
// before the code existed.
func TestSpaceText(t *testing.T) {
	dir := "/home/kingbase/cluster/install/kingbase/data"
	t.Run("primary", func(t *testing.T) {
		d, ts, w := spaceFacts(t, "node1_sys")
		rep := Space(v02Context("primary", "system", "local"), d, ts, w, node1Disk(dir))
		assertGolden(t, "space_primary", rep)
	})
	t.Run("kbdiag_ro, remote", func(t *testing.T) {
		d, ts, w := spaceFacts(t, "node1_ro")
		disk := facts.SpaceDisk{Status: facts.StatusNotApplicable, Reason: "remote connection"}
		rep := Space(v02Context("primary", "kbdiag_ro", "remote"), d, ts, w, disk)
		assertGolden(t, "space_ro", rep)
		if rep.Verdict != rule.VerdictUNKNOWN || len(rep.Findings) != 0 {
			t.Errorf("verdict %s, findings %v", rep.Verdict, rep.Findings)
		}
	})
}

// Several filesystems (WAL and a tablespace elsewhere), WAL past what the
// settings keep, a filesystem without an id, sizes in TB, settings not
// visible, an escaped path, and a disk error.
func TestSpaceTextEdges(t *testing.T) {
	c := v02Context("primary", "system", "local")
	tb := uint64(3) << 40
	disk := facts.SpaceDisk{Status: facts.StatusOK, Rows: []facts.Mount{
		{Kind: "data_directory", Path: "/data", Disk: facts.Disk{TotalBytes: tb, UsedBytes: tb - 1<<30, AvailBytes: 1 << 30, FSID: "a"}},
		{Kind: "wal", Path: "/data/sys_wal", Disk: facts.Disk{TotalBytes: 100 << 30, UsedBytes: 99 << 30, AvailBytes: 0, FSID: "b"}},
		{Kind: "tablespace", Path: "/ts/\x1b[2Jone", Disk: facts.Disk{TotalBytes: tb, UsedBytes: tb - 1<<30, AvailBytes: 1 << 30, FSID: "a"}},
		{Kind: "tablespace", Path: "/ts/two", Disk: facts.Disk{}},
	}}
	seg, keep, maxWAL := int64(16<<20), int64(0), int64(1<<30)
	w := facts.SpaceWAL{Status: facts.StatusOK, Rows: []facts.WAL{{Files: 1, Bytes: 5 << 30, MaxWALSizeBytes: &maxWAL, WALKeepBytes: &keep, WALSegmentBytes: &seg}}}
	loc := "/ts/two"
	ts := facts.SpaceTablespaces{Status: facts.StatusOK, Rows: []facts.Tablespace{{Name: "two", Location: &loc, SizeBytes: i64(0)}}}
	d := facts.InstDatabases{Status: facts.StatusOK}
	rep := Space(c, d, ts, w, disk)
	assertGolden(t, "space_edges", rep)
	if rep.Verdict != rule.VerdictOK {
		t.Errorf("verdict = %s", rep.Verdict)
	}

	// settings not visible: no reference, no hint
	w.Rows[0].MaxWALSizeBytes, w.Rows[0].WALKeepBytes, w.Rows[0].WALSegmentBytes = nil, nil, nil
	rep = Space(c, facts.InstDatabases{Status: facts.StatusError, Reason: "XX000: boom"}, facts.SpaceTablespaces{Status: facts.StatusOK},
		w, facts.SpaceDisk{Status: facts.StatusError, Reason: "statfs /data: input/output error"})
	assertGolden(t, "space_errors", rep)
	if rep.Verdict != rule.VerdictUNKNOWN {
		t.Errorf("verdict = %s", rep.Verdict)
	}
}
