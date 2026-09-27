package facts

import "time"

const (
	CheckpointStatsID    = "checkpoint.stats"
	CheckpointLastID     = "checkpoint.last"
	CheckpointSettingsID = "checkpoint.settings"
)

// Column contracts of the checkpoint probes (PRD §5.2).
var (
	CheckpointStatsColumns = []string{"checkpoints_timed", "checkpoints_req", "checkpoint_write_s", "checkpoint_sync_s", "buffers_checkpoint", "buffers_clean",
		"maxwritten_clean", "buffers_backend", "buffers_backend_fsync", "buffers_alloc", "stats_reset", "stats_reset_age_s"}
	CheckpointLastColumns     = []string{"checkpoint_time", "checkpoint_age_s", "checkpoint_lsn", "redo_lsn", "redo_wal_file"}
	CheckpointSettingsColumns = []string{"checkpoint_timeout_s", "max_wal_size_bytes", "checkpoint_completion_target", "checkpoint_warning_s", "log_checkpoints"}
)

// BGWriter is the single row of sys_stat_bgwriter (PG12 columns: stage 0),
// cumulative since stats_reset. On a standby the checkpoints are
// restartpoints.
type BGWriter struct {
	CheckpointsTimed    int64
	CheckpointsReq      int64
	CheckpointWriteS    float64
	CheckpointSyncS     float64
	BuffersCheckpoint   int64
	BuffersClean        int64
	MaxwrittenClean     int64
	BuffersBackend      int64
	BuffersBackendFsync int64
	BuffersAlloc        int64
	StatsReset          *time.Time
	StatsResetAgeS      *float64
}

func (b BGWriter) Row() []any {
	return []any{b.CheckpointsTimed, b.CheckpointsReq, b.CheckpointWriteS, b.CheckpointSyncS, b.BuffersCheckpoint, b.BuffersClean,
		b.MaxwrittenClean, b.BuffersBackend, b.BuffersBackendFsync, b.BuffersAlloc, isoTime(b.StatsReset), b.StatsResetAgeS}
}

type CheckpointStats struct {
	Status Status
	Reason string
	Rows   []BGWriter
}

// LastCheckpoint is what the control file records of the last checkpoint.
type LastCheckpoint struct {
	Time        *time.Time
	AgeS        *float64
	LSN         *string
	RedoLSN     *string
	RedoWALFile *string
}

func (l LastCheckpoint) Row() []any {
	return []any{isoTime(l.Time), l.AgeS, l.LSN, l.RedoLSN, l.RedoWALFile}
}

type CheckpointLast struct {
	Status Status
	Reason string
	Rows   []LastCheckpoint
}

type CheckpointSetting struct {
	TimeoutS         int64
	MaxWALSizeBytes  int64
	CompletionTarget float64
	WarningS         int64
	LogCheckpoints   string
}

func (s CheckpointSetting) Row() []any {
	return []any{s.TimeoutS, s.MaxWALSizeBytes, s.CompletionTarget, s.WarningS, s.LogCheckpoints}
}

type CheckpointSettings struct {
	Status Status
	Reason string
	Rows   []CheckpointSetting
}
