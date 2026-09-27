package facts

import "time"

const (
	ArchiveStatusID = "archive.status"
	ArchiveReadyID  = "archive.ready"
)

// Column contracts of the archive probes (PRD §5.2).
var (
	ArchiveStatusColumns = []string{"archive_mode", "archive_command", "archive_timeout_s", "archived_count", "last_archived_wal", "last_archived_time", "last_archived_age_s",
		"failed_count", "last_failed_wal", "last_failed_time", "last_failed_age_s", "stats_reset"}
	ArchiveReadyColumns = []string{"ready", "done", "oldest_ready_age_s"}
)

// Archiver is the single row of archive.status: the settings and
// sys_stat_archiver, which counts since stats_reset.
type Archiver struct {
	Mode             string
	Command          *string // NULL or '' (Oracle mode reads '' as NULL): nothing to run
	TimeoutS         int64
	ArchivedCount    int64
	LastArchivedWAL  *string
	LastArchivedTime *time.Time
	LastArchivedAgeS *float64
	FailedCount      int64
	LastFailedWAL    *string
	LastFailedTime   *time.Time
	LastFailedAgeS   *float64
	StatsReset       *time.Time
}

func (a Archiver) Row() []any {
	return []any{a.Mode, a.Command, a.TimeoutS, a.ArchivedCount, a.LastArchivedWAL, isoTime(a.LastArchivedTime), a.LastArchivedAgeS,
		a.FailedCount, a.LastFailedWAL, isoTime(a.LastFailedTime), a.LastFailedAgeS, isoTime(a.StatsReset)}
}

// isoTime is the contract's time format (PRD §5): ISO 8601 with the zone.
func isoTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}

type ArchiveStatus struct {
	Status Status
	Reason string
	Rows   []Archiver
}

// ArchiveQueue counts archive_status: .ready files wait for archive_command,
// .done files are archived and wait for a checkpoint to recycle them.
type ArchiveQueue struct {
	Ready           int64
	Done            int64
	OldestReadyAgeS *float64
}

func (q ArchiveQueue) Row() []any { return []any{q.Ready, q.Done, q.OldestReadyAgeS} }

type ArchiveReady struct {
	Status Status
	Reason string
	Rows   []ArchiveQueue
}
