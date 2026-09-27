package probe

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// topCheckSQL finds the extension's schema in this database (the view is
// then read schema-qualified, never through search_path, where another
// object could stand in for it) and the collection switch; the setting's
// row is missing when the library is not loaded.
const topCheckSQL = `
select (select n.nspname::text from sys_extension e join sys_namespace n on n.oid = e.extnamespace
        where e.extname = 'sys_stat_statements'),
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
from %s s left join sys_database d on d.oid = s.dbid`

// topCollecting says whether the view can be read at all.
func topCollecting(view bool, track *string, db string) (facts.Status, string) {
	switch {
	case track == nil:
		return facts.StatusSkipped, "sys_stat_statements is not loaded: add it to shared_preload_libraries (needs a restart)"
	case !view:
		return facts.StatusSkipped, "sys_stat_statements is not installed in database " + db + ": CREATE EXTENSION sys_stat_statements there, or use -d"
	}
	return facts.StatusOK, ""
}

// topEmpty turns an empty view into a status: it only means "nothing ran"
// when statements are being collected. With track=none for this
// connection, rows collected earlier or for roles and databases that set
// it themselves are still shown.
func topEmpty(track string) (facts.Status, string) {
	if track == "none" {
		return facts.StatusSkipped, "sys_stat_statements.track=none: statements are not being collected; set it to top or all"
	}
	return facts.StatusOK, ""
}

func SQLTop(ctx context.Context, x *pgx.Conn, c facts.Context) facts.SQLTop {
	var schema, track *string
	if err := x.QueryRow(ctx, topCheckSQL).Scan(&schema, &track); err != nil {
		st, reason := classify(err)
		return facts.SQLTop{Status: st, Reason: reason}
	}
	if st, reason := topCollecting(schema != nil, track, c.Database); st != facts.StatusOK {
		return facts.SQLTop{Status: st, Reason: reason}
	}
	view := pgx.Identifier{*schema, "sys_stat_statements"}.Sanitize()
	st, reason, out := collect(ctx, x, fmt.Sprintf(sqlTopSQL, view), func(r pgx.CollectableRow) (facts.Statement, error) {
		var s facts.Statement
		err := r.Scan(&s.QueryID, &s.Username, &s.Datname, &s.Calls, &s.TotalExecS, &s.MeanExecS, &s.MaxExecS, &s.Rows,
			&s.SharedBlksHit, &s.SharedBlksRead, &s.TempBlksWritten, &s.Query)
		return s, err
	})
	if st == facts.StatusOK && len(out) == 0 {
		st, reason = topEmpty(*track)
	}
	return facts.SQLTop{Status: st, Reason: reason, Rows: out, Track: *track}
}
