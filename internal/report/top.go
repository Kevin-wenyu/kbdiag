package report

import (
	"fmt"
	"io"
	"math"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// topView is the top text layout (plan 2026-09-27 appendix B.10): the
// statements in --by order, each with its share of all execution time.
// The numbers are cumulative, and the header says so.
type topView struct {
	t     facts.SQLTop
	by    string
	limit int
}

func (r *Report) SetTop(t facts.SQLTop, by string, limit int) {
	v := &topView{t: t, by: by, limit: limit}
	r.layout = func(r *Report, w io.Writer) error { return v.write(w) }
}

func (v *topView) write(w io.Writer) error {
	if v.t.Status != facts.StatusOK {
		writeNotOKAs(w, "statements", v.t.Status, v.t.Reason)
		return nil
	}
	fmt.Fprintf(w, "\nstatements: %d, by %s  (cumulative since the last reset, whose time is not recorded; read and temp are blocks)\n", len(v.t.Rows), v.by)
	shown := v.t.Rows
	if v.limit > 0 && len(shown) > v.limit {
		shown = shown[:v.limit]
	}
	if len(shown) == 0 {
		return nil
	}
	all := 0.0
	for _, s := range v.t.Rows {
		all += s.TotalExecS
	}
	var rows [][]string
	for _, s := range shown {
		share := "-"
		if all > 0 {
			share = fmt.Sprintf("%.1f%%", s.TotalExecS*100/all)
		}
		q := "?"
		if !s.Masked() {
			q = cell(s.Query)
		}
		rows = append(rows, []string{execTime(s.TotalExecS), share, fmt.Sprint(s.Calls), execTime(s.MeanExecS), fmt.Sprint(s.Rows),
			fmt.Sprint(s.SharedBlksRead), fmt.Sprint(s.TempBlksWritten), name(s.Username), name(s.Datname), q})
	}
	if err := writeTable(w, "  ", []string{"total", "share", "calls", "mean", "rows", "read", "temp", "user", "database", "query"}, rows); err != nil {
		return err
	}
	writeTruncated(w, len(v.t.Rows)-len(shown))
	return nil
}

// execTime shows statement times the way they are read: 0.09 ms, 503 ms,
// 1.75 s, then the two-unit durations past a minute.
func execTime(s float64) string {
	ms := s * 1000
	switch {
	case s >= 60:
		return duration(s)
	case s >= 1:
		return fmt.Sprintf("%.2f s", s)
	case ms >= 10:
		return fmt.Sprintf("%.0f ms", math.Round(ms))
	}
	return fmt.Sprintf("%.2f ms", ms)
}
