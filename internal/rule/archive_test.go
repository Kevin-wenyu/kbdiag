package rule

import (
	"strings"
	"testing"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func archiver(mode string, okAt, failAt *time.Time, failed int64) facts.ArchiveStatus {
	cmd := "cp %p /arch/%f"
	a := facts.Archiver{Mode: mode, Command: &cmd, ArchivedCount: 35, FailedCount: failed, LastArchivedTime: okAt, LastFailedTime: failAt}
	if okAt != nil {
		a.LastArchivedWAL = sp("0000000300000000000000A2")
	}
	if failAt != nil {
		a.LastFailedWAL = sp("0000000300000000000000A3")
	}
	return facts.ArchiveStatus{Status: facts.StatusOK, Rows: []facts.Archiver{a}}
}

func sp(s string) *string { return &s }

func TestArchive(t *testing.T) {
	t1 := time.Date(2026, 9, 16, 13, 49, 51, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	cases := []struct {
		name    string
		s       facts.ArchiveStatus
		role    string
		verdict Verdict
		finding bool
	}{
		{"lab: last failure after last success", archiver("always", &t1, &t2, 17961), "primary", VerdictWARN, true},
		{"recovered: success after the failure", archiver("on", &t2, &t1, 3), "primary", VerdictOK, false},
		{"never archived, failing", archiver("on", nil, &t1, 5), "primary", VerdictWARN, true},
		{"never tried", archiver("on", nil, nil, 0), "primary", VerdictOK, false},
		{"same instant is not later", archiver("on", &t1, &t1, 1), "primary", VerdictOK, false},
		{"archiving off: a choice", archiver("off", &t1, &t2, 9), "primary", VerdictOK, false},
		{"standby with on does not archive", archiver("on", &t1, &t2, 9), "standby", VerdictOK, false},
		{"standby with always does", archiver("always", &t1, &t2, 9), "standby", VerdictWARN, true},
		{"empty command: the archiver stopped trying", func() facts.ArchiveStatus {
			s := archiver("on", &t1, &t2, 9)
			s.Rows[0].Command = sp("")
			return s
		}(), "primary", VerdictOK, false},
		{"NULL command (Oracle mode reads '' as NULL)", func() facts.ArchiveStatus {
			s := archiver("on", &t1, &t2, 9)
			s.Rows[0].Command = nil
			return s
		}(), "primary", VerdictOK, false},
		{"not collected", facts.ArchiveStatus{Status: facts.StatusError}, "primary", VerdictUNKNOWN, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Archive(c.s, c.role)
			if r.Verdict != c.verdict || (len(r.Findings) == 1) != c.finding {
				t.Fatalf("verdict=%s findings=%+v", r.Verdict, r.Findings)
			}
			if c.finding {
				f := r.Findings[0]
				if f.ID != "archive.failing" || f.Level != LevelWARN || !strings.Contains(f.Symptom, "0000000300000000000000A3") {
					t.Errorf("%+v", f)
				}
			}
		})
	}
}
