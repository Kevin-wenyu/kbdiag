package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// checkpointStatsSQL is checkpoint.stats.
// Source: written for kbdiag against the KES V8R6 manual (help.kingbase.com.cn/v8,
// 动态性能视图 sys_stat_bgwriter). Stage 0 capture (checkpoint_*_bgwriter):
// the PG12 columns (checkpoints_timed, checkpoints_req, buffers_backend_fsync
// and the rest); kbdiag_ro reads the same. Times are milliseconds in the
// view, converted to seconds. Not run on a VM yet.
const checkpointStatsSQL = `
select checkpoints_timed, checkpoints_req, checkpoint_write_time / 1000, checkpoint_sync_time / 1000,
       buffers_checkpoint, buffers_clean, maxwritten_clean, buffers_backend, buffers_backend_fsync, buffers_alloc,
       stats_reset, round(extract(epoch from now() - stats_reset)::numeric, 1)::float8
from sys_stat_bgwriter`

// checkpointLastSQL is checkpoint.last: the control file's last checkpoint
// (stage 0 capture checkpoint_*_control; kbdiag_ro may call it).
const checkpointLastSQL = `
select checkpoint_time, round(extract(epoch from now() - checkpoint_time)::numeric, 1)::float8,
       checkpoint_lsn::text, redo_lsn::text, redo_wal_file
from sys_control_checkpoint()`

// checkpointSettingsSQL is checkpoint.settings (stage 0 capture
// checkpoint_*_settings: checkpoint_timeout and checkpoint_warning in s,
// max_wal_size in MB).
const checkpointSettingsSQL = `
select (select setting::bigint from sys_settings where name = 'checkpoint_timeout'),
       (select setting::bigint from sys_settings where name = 'max_wal_size') * 1048576,
       current_setting('checkpoint_completion_target')::float8,
       (select setting::bigint from sys_settings where name = 'checkpoint_warning'),
       current_setting('log_checkpoints')`

func CheckpointStats(ctx context.Context, x *pgx.Conn) facts.CheckpointStats {
	st, reason, out := collect(ctx, x, checkpointStatsSQL, func(r pgx.CollectableRow) (facts.BGWriter, error) {
		var b facts.BGWriter
		err := r.Scan(&b.CheckpointsTimed, &b.CheckpointsReq, &b.CheckpointWriteS, &b.CheckpointSyncS, &b.BuffersCheckpoint, &b.BuffersClean,
			&b.MaxwrittenClean, &b.BuffersBackend, &b.BuffersBackendFsync, &b.BuffersAlloc, &b.StatsReset, &b.StatsResetAgeS)
		return b, err
	})
	return facts.CheckpointStats{Status: st, Reason: reason, Rows: out}
}

func CheckpointLast(ctx context.Context, x *pgx.Conn) facts.CheckpointLast {
	st, reason, out := collect(ctx, x, checkpointLastSQL, func(r pgx.CollectableRow) (facts.LastCheckpoint, error) {
		var l facts.LastCheckpoint
		err := r.Scan(&l.Time, &l.AgeS, &l.LSN, &l.RedoLSN, &l.RedoWALFile)
		return l, err
	})
	return facts.CheckpointLast{Status: st, Reason: reason, Rows: out}
}

func CheckpointSettings(ctx context.Context, x *pgx.Conn) facts.CheckpointSettings {
	st, reason, out := collect(ctx, x, checkpointSettingsSQL, func(r pgx.CollectableRow) (facts.CheckpointSetting, error) {
		var s facts.CheckpointSetting
		err := r.Scan(&s.TimeoutS, &s.MaxWALSizeBytes, &s.CompletionTarget, &s.WarningS, &s.LogCheckpoints)
		return s, err
	})
	return facts.CheckpointSettings{Status: st, Reason: reason, Rows: out}
}
