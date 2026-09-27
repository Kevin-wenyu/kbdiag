package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// vacuumTablesSQL is vacuum.tables: every user table of the current
// database with what autovacuum decides by (reltuples, reloptions).
// Source: written for kbdiag against the KES V8R6 manual (help.kingbase.com.cn/v8,
// 动态性能视图 sys_stat_user_tables; 系统表 sys_class). Stage 0 capture
// (vacuum_*_tables_dead): kbdiag_ro sees the same rows as system; on the
// standby every counter is 0 (statistics are local), so a standby does not
// run it. reltuples stays float4 (ksql shows 1e+06): the threshold is
// computed in float4, as the server does. With track_counts off the counters
// stop moving, so the probe is skipped rather than showing them as current.
// Not run on a VM yet.
const vacuumTablesSQL = `
select s.schemaname::text, s.relname::text, s.n_live_tup, s.n_dead_tup, c.reltuples, c.reloptions::text[],
       round(extract(epoch from now() - s.last_vacuum)::numeric, 1)::float8,
       round(extract(epoch from now() - s.last_autovacuum)::numeric, 1)::float8,
       s.vacuum_count, s.autovacuum_count
from sys_stat_user_tables s join sys_class c on c.oid = s.relid
order by s.n_dead_tup desc, s.relid`

// vacuumProgressSQL is vacuum.progress: every VACUUM running now, manual or
// autovacuum, in any database. A relation of another database cannot be
// named from here (regclass is per database), so it stays an oid.
// Source: KES V8R6 manual (动态性能视图 sys_stat_progress_vacuum,
// sys_stat_activity); stage 0 captured it empty on both nodes, for system
// and kbdiag_ro alike, so the masked case (phase NULL) is not seen yet.
// Not run on a VM yet.
const vacuumProgressSQL = `
select p.pid,
       p.datname::text,
       case when p.datname = current_database() then p.relid::regclass::text else p.relid::text end,
       p.phase,
       p.heap_blks_total,
       p.heap_blks_scanned,
       case when a.backend_type is not null then a.backend_type = 'autovacuum worker' end,
       round(extract(epoch from now() - a.xact_start)::numeric, 1)::float8
from sys_stat_progress_vacuum p left join sys_stat_activity a on a.pid = p.pid
order by p.pid`

// vacuumSettingsSQL is vacuum.settings (stage 0 capture vacuum_*_settings;
// autovacuum_naptime is read in seconds from sys_settings, current_setting
// would print "1min").
const vacuumSettingsSQL = `
select current_setting('autovacuum'),
       current_setting('track_counts'),
       current_setting('autovacuum_vacuum_threshold')::bigint,
       current_setting('autovacuum_vacuum_scale_factor')::float8,
       (select setting::bigint from sys_settings where name = 'autovacuum_naptime'),
       current_setting('autovacuum_max_workers')::int`

// VacuumTables reads the table statistics; a standby's are all zeros.
func VacuumTables(ctx context.Context, x *pgx.Conn, c facts.Context) facts.VacuumTables {
	if c.Role == "standby" {
		return facts.VacuumTables{Status: facts.StatusNotApplicable, Reason: "standby: table statistics are local to each node and stay 0 here; run on the primary"}
	}
	var track string
	if err := x.QueryRow(ctx, "select current_setting('track_counts')").Scan(&track); err != nil {
		st, reason := classify(err)
		return facts.VacuumTables{Status: st, Reason: reason}
	}
	if track != "on" {
		return facts.VacuumTables{Status: facts.StatusSkipped, Reason: "track_counts=" + track + ": table statistics are not collected"}
	}
	st, reason, out := collect(ctx, x, vacuumTablesSQL, func(r pgx.CollectableRow) (facts.VacuumTable, error) {
		var t facts.VacuumTable
		err := r.Scan(&t.Schemaname, &t.Relname, &t.NLiveTup, &t.NDeadTup, &t.Reltuples, &t.Reloptions,
			&t.LastVacuumAgeS, &t.LastAutovacuumAgeS, &t.VacuumCount, &t.AutovacuumCount)
		return t, err
	})
	return facts.VacuumTables{Status: st, Reason: reason, Rows: out}
}

// VacuumProgress reads the running vacuums; autovacuum does not run on a
// standby, and a manual VACUUM cannot.
func VacuumProgress(ctx context.Context, x *pgx.Conn, c facts.Context) facts.VacuumProgress {
	if c.Role == "standby" {
		return facts.VacuumProgress{Status: facts.StatusNotApplicable, Reason: "standby: autovacuum does not run here; run on the primary"}
	}
	st, reason, out := collect(ctx, x, vacuumProgressSQL, func(r pgx.CollectableRow) (facts.VacuumRun, error) {
		var v facts.VacuumRun
		err := r.Scan(&v.PID, &v.Datname, &v.Relation, &v.Phase, &v.HeapBlksTotal, &v.HeapBlksScanned, &v.IsAutovacuum, &v.XactAgeS)
		return v, err
	})
	return facts.VacuumProgress{Status: st, Reason: reason, Rows: out}
}

func VacuumSettings(ctx context.Context, x *pgx.Conn) facts.VacuumSettings {
	st, reason, out := collect(ctx, x, vacuumSettingsSQL, func(r pgx.CollectableRow) (facts.VacuumSetting, error) {
		var s facts.VacuumSetting
		err := r.Scan(&s.Autovacuum, &s.TrackCounts, &s.Threshold, &s.ScaleFactor, &s.NaptimeS, &s.MaxWorkers)
		return s, err
	})
	return facts.VacuumSettings{Status: st, Reason: reason, Rows: out}
}
