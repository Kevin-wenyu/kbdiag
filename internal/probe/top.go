package probe

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// topCheckSQL says whether the view exists in this database and what the
// collection switch is; the setting's row is missing when the library is
// not loaded.
const topCheckSQL = `
select to_regclass('sys_stat_statements') is not null,
       (select setting from sys_settings where name = 'sys_stat_statements.track')`

// sqlTopSQL is sql.top: every statement sys_stat_statements holds, sorted
// by the scenario (--by). Times are converted from milliseconds.
// Source: written for kbdiag against the KES V8R6 manual (help.kingbase.com.cn/v8,
// sys_stat_statements). Stage 0 capture (top_*): version 1.11 with the
// PG13-style names total_exec_time and mean_exec_time; no
// sys_stat_statements_info, so the reset time is unknown; track=none on the
// lab; kbdiag_ro sees another user's statement as <insufficient privilege>
// with a NULL queryid, numbers visible. pg_get_userbyid is the PG kernel's
// (not captured). Not run on a VM yet.
const sqlTopSQL = `
select s.queryid, pg_get_userbyid(s.userid)::text, d.datname::text, s.calls,
       s.total_exec_time / 1000, s.mean_exec_time / 1000, s.max_exec_time / 1000, s.rows,
       s.shared_blks_hit, s.shared_blks_read, s.temp_blks_written, s.query
from sys_stat_statements s left join sys_database d on d.oid = s.dbid`

// topCollecting turns the check into a status: an empty result is only
// "nothing ran" when the statistics are being collected.
func topCollecting(view bool, track *string, db string) (facts.Status, string) {
	switch {
	case track == nil:
		return facts.StatusSkipped, "sys_stat_statements is not loaded: add it to shared_preload_libraries (needs a restart)"
	case !view:
		return facts.StatusSkipped, "sys_stat_statements is not installed in database " + db + ": CREATE EXTENSION sys_stat_statements there, or use -d"
	case *track == "none":
		return facts.StatusSkipped, "sys_stat_statements.track=none: statements are not being collected; set it to top or all"
	}
	return facts.StatusOK, ""
}

func SQLTop(ctx context.Context, x *pgx.Conn, c facts.Context) facts.SQLTop {
	var view bool
	var track *string
	if err := x.QueryRow(ctx, topCheckSQL).Scan(&view, &track); err != nil {
		st, reason := classify(err)
		return facts.SQLTop{Status: st, Reason: reason}
	}
	if st, reason := topCollecting(view, track, c.Database); st != facts.StatusOK {
		return facts.SQLTop{Status: st, Reason: reason}
	}
	st, reason, out := collect(ctx, x, sqlTopSQL, func(r pgx.CollectableRow) (facts.Statement, error) {
		var s facts.Statement
		err := r.Scan(&s.QueryID, &s.Username, &s.Datname, &s.Calls, &s.TotalExecS, &s.MeanExecS, &s.MaxExecS, &s.Rows,
			&s.SharedBlksHit, &s.SharedBlksRead, &s.TempBlksWritten, &s.Query)
		return s, err
	})
	return facts.SQLTop{Status: st, Reason: reason, Rows: out}
}
