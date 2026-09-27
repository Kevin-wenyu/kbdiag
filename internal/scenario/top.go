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
	sort.SliceStable(rs, func(i, j int) bool {
		if a, b := key(rs[i]), key(rs[j]); a != b {
			return a > b
		}
		return rs[i].TotalExecS > rs[j].TotalExecS
	})
	t.Rows = rs
	rep := report.New("top", c, rule.Display(len(t.Redacted()) > 0, t.Status))
	rep.AddProbe(facts.SQLTopID, t.Status, t.Reason, facts.TopColumns, rows(t.Rows), o.Limit)
	rep.AddRedacted(t.Redacted())
	rep.SetTop(t, TopOrders[o.By], o.Limit)
	return rep
}
