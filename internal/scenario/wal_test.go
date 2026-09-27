package scenario

import (
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// walFacts rebuilds wal from the stage 0 captures (wal_node1_*, taken at
// 22:47; archive_status from archive_node1_sys_statusdir). The WAL file is
// what sys_walfile_name gives for the LSN on timeline 3 (not captured).
func walFacts(t *testing.T) (facts.WALPosition, facts.SpaceWAL, facts.SlotList, facts.ArchiveReady) {
	t.Helper()
	m := loadKsql(t, "wal_node1_sys_lsn").rows[0]
	file := "0000000300000000000000A9"
	pos := facts.WALPosition{Status: facts.StatusOK, Rows: []facts.Position{{InRecovery: kBool(m["sys_is_in_recovery"]), LSN: m["sys_current_wal_lsn"], WALFile: &file}}}
	d := loadKsql(t, "wal_node1_sys_dir").rows[0]
	seg, keep, maxWAL := int64(16<<20), int64(512*16<<20), int64(1<<30)
	w := facts.SpaceWAL{Status: facts.StatusOK, Rows: []facts.WAL{{Files: *kI64(t, d["count"]), Bytes: *kI64(t, d["sum"]), MaxWALSizeBytes: &maxWAL, WALKeepBytes: &keep, WALSegmentBytes: &seg}}}
	s := facts.SlotList{Status: facts.StatusOK}
	for _, x := range loadKsql(t, "wal_node1_sys_slots").rows {
		pid := int32(*kI64(t, x["active_pid"]))
		s.Rows = append(s.Rows, facts.Slot{Name: *x["slot_name"], Type: *x["slot_type"], Active: kBool(x["active"]), ActivePID: &pid, RestartLSN: x["restart_lsn"], RetainedWALBytes: i64(0)})
	}
	r := loadKsql(t, "archive_node1_sys_statusdir").rows[0]
	a := facts.ArchiveReady{Status: facts.StatusOK, Rows: []facts.ArchiveQueue{{Ready: *kI64(t, r["ready"]), Done: *kI64(t, r["done"])}}}
	return pos, w, s, a
}

func TestWALText(t *testing.T) {
	p, w, s, a := walFacts(t)
	rep := WAL(v02Context("primary", "system", "local"), p, w, s, a)
	assertGolden(t, "wal_primary", rep)
	if rep.Verdict != rule.VerdictOK || len(rep.Findings) != 0 {
		t.Errorf("verdict=%s findings=%v", rep.Verdict, rep.Findings)
	}
}

// A standby (replay position, no file, an inactive slot of its own) and
// kbdiag_ro (sys_ls_waldir and archive_status refused: stage 0).
func TestWALTextEdges(t *testing.T) {
	_, w, _, a := walFacts(t)
	lsn := "0/A9128C08"
	pos := facts.WALPosition{Status: facts.StatusOK, Rows: []facts.Position{{InRecovery: true, LSN: &lsn}}}
	s := facts.SlotList{Status: facts.StatusOK, Rows: []facts.Slot{{Name: "cascade", Type: "physical", RetainedWALBytes: i64(3 << 30)}}}
	assertGolden(t, "wal_standby", WAL(v02Context("standby", "system", "local"), pos, w, s, a))
	file := "0000000300000000000000A9"
	primary := facts.WALPosition{Status: facts.StatusOK, Rows: []facts.Position{{LSN: &lsn, WALFile: &file}}}
	ro := WAL(v02Context("primary", "kbdiag_ro", "remote"), primary, facts.SpaceWAL{Status: facts.StatusSkipped, Reason: "insufficient_privilege 42501: permission denied for function sys_ls_waldir"},
		facts.SlotList{Status: facts.StatusOK}, facts.ArchiveReady{Status: facts.StatusSkipped, Reason: "insufficient_privilege 42501: permission denied for function sys_ls_archive_statusdir"})
	assertGolden(t, "wal_ro", ro)
	if ro.Verdict != rule.VerdictUNKNOWN {
		t.Errorf("verdict = %s", ro.Verdict)
	}
}
