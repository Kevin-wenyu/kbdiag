package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

type TxnOptions struct {
	Limit      int
	Thresholds rule.Thresholds
}

// Txn builds the txn report: sessions inside a transaction (an xid, an xmin
// or a transaction start), plus the masked and untracked rows that may be,
// and every prepared transaction. The rules judge every row; --limit only
// narrows the sessions shown.
func Txn(c facts.Context, a facts.SessionActivity, p facts.TxnPrepared, o TxnOptions) *report.Report {
	rep := report.New("txn", c, rule.Txn(a, p, o.Thresholds))
	var shown []facts.Session
	for _, s := range a.Rows {
		if s.Masked() || s.Untracked() || s.XactAgeS != nil || s.BackendXID != nil || s.BackendXmin != nil {
			shown = append(shown, s)
		}
	}
	rep.AddProbe(facts.SessionActivityID, a.Status, a.Reason, facts.SessionColumns, rows(shown), o.Limit)
	rep.AddProbe(facts.TxnPreparedID, p.Status, p.Reason, facts.PreparedColumns, rows(p.Rows), 0)
	rep.AddRedacted(a.Redacted())
	rep.SetTxn(a, p, o.Limit)
	return rep
}
