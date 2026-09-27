package scenario

import (
	"sort"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// TopOrders are the --by values: what "top" means.
var TopOrders = map[string]string{
	"time":  "total execution time",
	"mean":  "mean execution time",
	"calls": "calls",
	"io":    "blocks read from outside shared buffers",
	"temp":  "temporary blocks written",
}

type TopOptions struct {
	Limit int
	By    string
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Top only shows: which statement is too costly has no objective line. The
// rows are sorted by --by before --limit cuts them; ties go by total time.
func Top(c facts.Context, t facts.SQLTop, o TopOptions) *report.Report {
	rs := append([]facts.Statement(nil), t.Rows...)
	key := func(s facts.Statement) float64 {
		switch o.By {
		case "mean":
			return s.MeanExecS
		case "calls":
			return float64(s.Calls)
		case "io":
			return float64(s.SharedBlksRead)
		case "temp":
			return float64(s.TempBlksWritten)
		}
		return s.TotalExecS
	}
	// ties: total time, then calls, queryid (NULL last) and the text, so
	// the order does not depend on the view's hash order
	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		switch {
		case key(a) != key(b):
			return key(a) > key(b)
		case a.TotalExecS != b.TotalExecS:
			return a.TotalExecS > b.TotalExecS
		case a.Calls != b.Calls:
			return a.Calls > b.Calls
		case (a.QueryID == nil) != (b.QueryID == nil):
			return a.QueryID != nil
		case a.QueryID != nil && *a.QueryID != *b.QueryID:
			return *a.QueryID < *b.QueryID
		}
		return orEmpty(a.Query) < orEmpty(b.Query)
	})
	t.Rows = rs
	rep := report.New("top", c, rule.Display(len(t.Redacted()) > 0, t.Status))
	rep.AddProbe(facts.SQLTopID, t.Status, t.Reason, facts.TopColumns, rows(t.Rows), o.Limit)
	rep.AddRedacted(t.Redacted())
	rep.SetTop(t, TopOrders[o.By], o.Limit)
	return rep
}
