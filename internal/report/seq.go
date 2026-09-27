package report

import (
	"fmt"
	"io"
	"math"
	"math/big"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// seqView is the seq text layout (plan 2026-09-27 appendix B.15): the
// sequences that used the most of their range first.
type seqView struct {
	l     facts.SeqList
	limit int
}

func (r *Report) SetSeq(l facts.SeqList, limit int) {
	v := &seqView{l: l, limit: limit}
	r.layout = func(r *Report, w io.Writer) error { return v.write(r, w) }
}

func (v *seqView) write(r *Report, w io.Writer) error {
	title := "sequences in " + escapeControl(r.Context.Database)
	if r.Context.Database == "" {
		title = "sequences"
	}
	if v.l.Status != facts.StatusOK {
		writeNotOKAs(w, title, v.l.Status, v.l.Reason)
		return nil
	}
	fmt.Fprintf(w, "\n%s: %d, most used first\n", title, len(v.l.Rows))
	shown := v.l.Rows
	if v.limit > 0 && len(shown) > v.limit {
		shown = shown[:v.limit]
	}
	if len(shown) == 0 {
		return nil
	}
	var rows [][]string
	for _, s := range shown {
		last, used, left := "-", "-", "never used"
		switch l, u, ok := rule.SeqLeft(s); {
		case !s.Readable:
			last, used, left = "?", "?", "?"
		case ok:
			last = fmt.Sprint(*s.LastValue)
			used = fmt.Sprintf("%.2f%%", math.Floor(u*10000)/100)
			left = count(l)
			if s.Cycle {
				left += " (cycles)"
			}
		}
		limit := s.MaxValue
		if s.IncrementBy < 0 {
			limit = s.MinValue
		}
		rows = append(rows, []string{fitWidth(escapeControl(s.Schemaname+"."+s.Sequencename), 2*maxName), escapeControl(s.DataType), last, fmt.Sprint(limit), used, left})
	}
	if err := writeTable(w, "  ", []string{"sequence", "type", "last", "limit", "used", "left"}, rows); err != nil {
		return err
	}
	writeTruncated(w, len(v.l.Rows)-len(shown))
	return nil
}

// count is exact below a million, three significant digits above.
func count(n *big.Int) string {
	if n.IsInt64() && n.Int64() < 1_000_000 {
		return n.String()
	}
	f, _ := new(big.Float).SetInt(n).Float64()
	return fmt.Sprintf("%.3g", f)
}
