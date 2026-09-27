package scenario

import (
	"sort"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

type SeqOptions struct{ Limit int }

// Seq judges every sequence; the rows are ordered by how much of their
// range is used (never used and unreadable last), and --limit cuts them.
func Seq(c facts.Context, l facts.SeqList, o SeqOptions) *report.Report {
	rs := append([]facts.Sequence(nil), l.Rows...)
	used := func(s facts.Sequence) (float64, bool) {
		_, u, ok := rule.SeqLeft(s)
		return u, ok
	}
	sort.SliceStable(rs, func(i, j int) bool {
		a, aok := used(rs[i])
		b, bok := used(rs[j])
		switch {
		case aok != bok:
			return aok
		case a != b:
			return a > b
		}
		return rs[i].Schemaname+"."+rs[i].Sequencename < rs[j].Schemaname+"."+rs[j].Sequencename
	})
	l.Rows = rs
	rep := report.New("seq", c, rule.Seq(l))
	rep.AddProbe(facts.SeqListID, l.Status, l.Reason, facts.SeqColumns, rows(l.Rows), o.Limit)
	rep.AddRedacted(l.Redacted())
	rep.SetSeq(l, o.Limit)
	return rep
}
