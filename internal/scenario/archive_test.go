package scenario

import (
	"testing"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

func kTime(t *testing.T, s *string) *time.Time {
	t.Helper()
	if s == nil {
		return nil
	}
	x, err := time.Parse("2006-01-02 15:04:05.999999-07", *s)
	if err != nil {
		t.Fatal(err)
	}
	x = x.In(cst)
	return &x
}

// archiveFacts rebuilds archive from the stage 0 captures: settings and
// sys_stat_archiver (with the capture's now() for the ages), and the
// archive_status counts.
func archiveFacts(t *testing.T, nodeUser string) (facts.ArchiveStatus, facts.ArchiveReady, time.Time) {
	t.Helper()
	set := map[string]*string{}
	for _, m := range loadKsql(t, "archive_"+nodeUser+"_settings").rows {
		set[*m["name"]] = m["setting"]
	}
	m := loadKsql(t, "archive_"+nodeUser+"_stat").rows[0]
	now := *kTime(t, m["now"])
	age := func(x *time.Time) *float64 {
		if x == nil {
			return nil
		}
		s := now.Sub(*x).Seconds()
		return &s
	}
	a := facts.Archiver{Mode: *set["archive_mode"], Command: set["archive_command"], TimeoutS: *kI64(t, set["archive_timeout"]),
		ArchivedCount: *kI64(t, m["archived_count"]), LastArchivedWAL: m["last_archived_wal"], LastArchivedTime: kTime(t, m["last_archived_time"]),
		FailedCount: *kI64(t, m["failed_count"]), LastFailedWAL: m["last_failed_wal"], LastFailedTime: kTime(t, m["last_failed_time"]), StatsReset: kTime(t, m["stats_reset"])}
	a.LastArchivedAgeS, a.LastFailedAgeS = age(a.LastArchivedTime), age(a.LastFailedTime)
	r := facts.ArchiveReady{Status: facts.StatusOK}
	c := loadKsql(t, "archive_"+nodeUser+"_statusdir")
	if c.err != "" {
		r = facts.ArchiveReady{Status: facts.StatusSkipped, Reason: "insufficient_privilege 42501: " + c.err}
	} else {
		r.Rows = []facts.ArchiveQueue{{Ready: *kI64(t, c.rows[0]["ready"]), Done: *kI64(t, c.rows[0]["done"])}}
	}
	return facts.ArchiveStatus{Status: facts.StatusOK, Rows: []facts.Archiver{a}}, r, now.Truncate(time.Second)
}

func TestArchiveText(t *testing.T) {
	t.Run("primary: the lab's archiving fails", func(t *testing.T) {
		s, r, now := archiveFacts(t, "node1_sys")
		c := v02Context("primary", "system", "local")
		c.CollectedAt = now
		rep := Archive(c, s, r)
		assertGolden(t, "archive_primary", rep)
		if rep.Verdict != rule.VerdictWARN {
			t.Errorf("verdict = %s", rep.Verdict)
		}
	})
	t.Run("standby, kbdiag_ro", func(t *testing.T) {
		s, _, now := archiveFacts(t, "node2_sys")
		_, r, _ := archiveFacts(t, "node2_ro")
		c := v02Context("standby", "kbdiag_ro", "remote")
		c.CollectedAt = now
		rep := Archive(c, s, r)
		assertGolden(t, "archive_standby_ro", rep)
		if rep.Verdict != rule.VerdictOK {
			t.Errorf("verdict = %s: archive.ready only shows", rep.Verdict)
		}
	})
}

// Never archived and failing with a hostile command and WAL name, an old
// .ready file; archiving off; a standby with archive_mode=on and an empty
// command; the archiver not collected.
func TestArchiveTextEdges(t *testing.T) {
	c := v02Context("primary", "system", "local")
	fail := time.Date(2026, 9, 27, 21, 0, 0, 0, cst)
	bad := "rm -rf \x1b[2J"
	st := facts.ArchiveStatus{Status: facts.StatusOK, Rows: []facts.Archiver{{Mode: "on", Command: &bad, TimeoutS: 60, FailedCount: 3,
		LastFailedWAL: str("00000001‮0001"), LastFailedTime: &fail, LastFailedAgeS: f64(1209)}}}
	ready := facts.ArchiveReady{Status: facts.StatusOK, Rows: []facts.ArchiveQueue{{Ready: 250, Done: 0, OldestReadyAgeS: f64(90000)}}}
	assertGolden(t, "archive_never", Archive(c, st, ready))

	st.Rows[0].Mode, st.Rows[0].Command = "off", nil
	rep := Archive(c, st, ready)
	assertGolden(t, "archive_off", rep)
	if rep.Verdict != rule.VerdictOK {
		t.Errorf("off: %s", rep.Verdict)
	}
	st.Rows[0].Mode, st.Rows[0].Command = "on", str("")
	sb := v02Context("standby", "system", "local")
	rep = Archive(sb, st, ready)
	assertGolden(t, "archive_standby_on", rep)
	if rep.Verdict != rule.VerdictOK {
		t.Errorf("standby with on: %s", rep.Verdict)
	}
	rep = Archive(c, facts.ArchiveStatus{Status: facts.StatusError, Reason: "XX000: boom"}, facts.ArchiveReady{Status: facts.StatusError, Reason: "XX000: boom"})
	if rep.Verdict != rule.VerdictUNKNOWN {
		t.Errorf("not collected: %s", rep.Verdict)
	}
}
