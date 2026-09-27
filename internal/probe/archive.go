package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// archiveStatusSQL is archive.status: the archive settings and the
// archiver's counters. Settings go through sys_settings like data_directory
// in inst.info.
// Source: written for kbdiag against the KES V8R6 manual (help.kingbase.com.cn/v8,
// 动态性能视图 sys_stat_archiver; 系统视图 sys_settings). Stage 0 capture
// (archive_*_stat, archive_*_settings): the same for kbdiag_ro; on the lab
// archive_mode is always and archiving fails (17961 failures, last success
// 2026-09-16). archive_timeout is in seconds in sys_settings. Not run on a
// VM yet.
const archiveStatusSQL = `
select (select setting from sys_settings where name = 'archive_mode'),
       (select setting from sys_settings where name = 'archive_command'),
       (select setting::bigint from sys_settings where name = 'archive_timeout'),
       archived_count, last_archived_wal, last_archived_time,
       round(extract(epoch from now() - last_archived_time)::numeric, 1)::float8,
       failed_count, last_failed_wal, last_failed_time,
       round(extract(epoch from now() - last_failed_time)::numeric, 1)::float8,
       stats_reset
from sys_stat_archiver`

// archiveReadySQL is archive.ready: the archive_status directory. kbdiag_ro
// may not call sys_ls_archive_statusdir (stage 0 capture), so it is
// skipped for such accounts; nothing is judged on it. Not run on a VM yet.
const archiveReadySQL = `
select count(*) filter (where name like '%.ready'),
       count(*) filter (where name like '%.done'),
       round(extract(epoch from now() - min(modification) filter (where name like '%.ready'))::numeric, 1)::float8
from sys_ls_archive_statusdir()`

func ArchiveStatus(ctx context.Context, x *pgx.Conn) facts.ArchiveStatus {
	st, reason, out := collect(ctx, x, archiveStatusSQL, func(r pgx.CollectableRow) (facts.Archiver, error) {
		var a facts.Archiver
		err := r.Scan(&a.Mode, &a.Command, &a.TimeoutS, &a.ArchivedCount, &a.LastArchivedWAL, &a.LastArchivedTime, &a.LastArchivedAgeS,
			&a.FailedCount, &a.LastFailedWAL, &a.LastFailedTime, &a.LastFailedAgeS, &a.StatsReset)
		return a, err
	})
	return facts.ArchiveStatus{Status: st, Reason: reason, Rows: out}
}

func ArchiveReady(ctx context.Context, x *pgx.Conn) facts.ArchiveReady {
	st, reason, out := collect(ctx, x, archiveReadySQL, func(r pgx.CollectableRow) (facts.ArchiveQueue, error) {
		var q facts.ArchiveQueue
		err := r.Scan(&q.Ready, &q.Done, &q.OldestReadyAgeS)
		return q, err
	})
	return facts.ArchiveReady{Status: st, Reason: reason, Rows: out}
}
