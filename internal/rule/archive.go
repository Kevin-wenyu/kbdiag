package rule

import (
	"fmt"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/units"
)

// Archive warns when the archiver's last attempt failed: its last failure
// is later than its last success (or it never succeeded). WAL waiting for
// the archive stays on this node and the backups miss it; queries still
// run, so it is WARN. archive_mode=off is a choice, and a standby archives
// only with archive_mode=always. An empty archive_command is not judged.
// archive.ready only shows.
func Archive(s facts.ArchiveStatus, role string) Result {
	judge, unknown := collected(s.Status)
	if !judge || len(s.Rows) == 0 {
		return Result{Verdict: verdictOf(nil, unknown)}
	}
	a := s.Rows[0]
	if a.Mode == "off" || role == "standby" && a.Mode != "always" {
		return Result{Verdict: VerdictOK}
	}
	// with no command the archiver stops trying: an old failure would stay
	// "last" forever (and an empty command is not judged, plan B.4)
	if a.Command == nil || *a.Command == "" {
		return Result{Verdict: VerdictOK}
	}
	failing := a.LastFailedTime != nil && (a.LastArchivedTime == nil || a.LastFailedTime.After(*a.LastArchivedTime))
	if !failing {
		return Result{Verdict: VerdictOK}
	}
	last := "it never succeeded"
	if a.LastArchivedTime != nil {
		last = "the last success was " + agoOf(a.LastArchivedAgeS) + " (" + orDash(a.LastArchivedWAL) + ")"
	}
	f := Finding{
		ID:    "archive.failing",
		Level: LevelWARN,
		Symptom: fmt.Sprintf("archiving is failing: the last attempt failed (%s, %s) and %s; %d failures since the statistics reset. WAL that is not archived stays in sys_wal and is missing from the backups",
			orDash(a.LastFailedWAL), agoOf(a.LastFailedAgeS), last, a.FailedCount),
		Evidence: []Evidence{{ProbeID: facts.ArchiveStatusID, Fields: map[string]any{
			"archive_mode": a.Mode, "failed_count": a.FailedCount, "last_failed_wal": a.LastFailedWAL, "last_failed_time": iso(a.LastFailedTime),
			"archived_count": a.ArchivedCount, "last_archived_wal": a.LastArchivedWAL, "last_archived_time": iso(a.LastArchivedTime),
		}}},
		Next: []Next{{Kind: "verify", Command: "kbdiag space", Note: "how big sys_wal has grown; why archive_command fails is in the server log (log_directory)"}},
	}
	return Result{Verdict: VerdictWARN, Findings: []Finding{f}}
}

func agoOf(s *float64) string {
	if s == nil {
		return "at an unknown time"
	}
	return units.Duration(*s) + " ago"
}

func orDash(s *string) string {
	if s == nil {
		return "-"
	}
	return *s
}

func iso(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}
