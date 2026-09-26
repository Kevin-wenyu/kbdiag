// Package probe collects facts: one probe_id, one SQL (inst.disk, a statfs,
// is the one exception). It makes no judgment; its correctness is only
// provable on a real KES (L3).
package probe

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// collect runs one probe SQL and scans every row; a failure becomes the
// probe status, never an empty ok.
func collect[T any](ctx context.Context, x *pgx.Conn, sql string, scan pgx.RowToFunc[T]) (facts.Status, string, []T) {
	rows, err := x.Query(ctx, sql)
	if err != nil {
		st, reason := classify(err)
		return st, reason, nil
	}
	out, err := pgx.CollectRows(rows, scan)
	if err != nil {
		st, reason := classify(err)
		return st, reason, nil
	}
	return facts.StatusOK, "", out
}

// trackActivities checks our own track_activities: with it off, KES no
// longer records state, wait events and SQL, so probes that read them are
// skipped instead of returning rows that look idle.
func trackActivities(ctx context.Context, x *pgx.Conn) (facts.Status, string) {
	var track string
	if err := x.QueryRow(ctx, "select current_setting('track_activities')").Scan(&track); err != nil {
		return classify(err)
	}
	if track != "on" {
		return facts.StatusSkipped, "track_activities=" + track + "：会话状态和 SQL 未被记录"
	}
	return facts.StatusOK, ""
}

// classify maps a query error to a probe status (PRD §5): timeouts and
// missing privileges are skipped, anything else is an error with its code.
func classify(err error) (facts.Status, string) {
	var pe *pgconn.PgError
	if !errors.As(err, &pe) {
		return facts.StatusError, err.Error()
	}
	reason := fmt.Sprintf("%s: %s", pe.Code, pe.Message)
	switch pe.Code {
	case "57014", "55P03": // statement_timeout, lock_timeout
		return facts.StatusSkipped, "timeout " + reason
	case "42501":
		return facts.StatusSkipped, "insufficient_privilege " + reason
	}
	return facts.StatusError, reason
}
