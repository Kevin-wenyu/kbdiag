package scenario

import (
	"slices"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

type SessionOptions struct {
	PID        int32
	Thresholds rule.Thresholds
}

// Session builds the report on one session: its activity row, the locks it
// holds or waits for, and the waits of sessions it blocks. found is false
// when the activity was read and has no such pid; the verdict is then
// UNKNOWN, since there is nothing to judge.
func Session(c facts.Context, a facts.SessionActivity, l facts.LockList, o SessionOptions) (rep *report.Report, found bool) {
	act := a
	act.Rows = nil
	for _, s := range a.Rows {
		if s.PID == o.PID {
			act.Rows = append(act.Rows, s)
		}
	}
	locks := l
	locks.Rows = nil
	for _, x := range l.Rows {
		if (x.PID != nil && *x.PID == o.PID) || slices.Contains(x.BlockedBy, o.PID) {
			locks.Rows = append(locks.Rows, x)
		}
	}
	found = a.Status != facts.StatusOK || len(act.Rows) > 0
	r := rule.Merge(rule.Sessions(act, o.Thresholds), rule.Locks(locks, o.Thresholds))
	if !found {
		r = rule.Merge(r, rule.Result{Verdict: rule.VerdictUNKNOWN})
	}
	rep = report.New("session", c, r)
	rep.AddProbe(facts.SessionActivityID, act.Status, act.Reason, facts.SessionColumns, rows(act.Rows), 0)
	rep.AddProbe(facts.LockListID, locks.Status, locks.Reason, facts.LockColumns, rows(locks.Rows), 0)
	rep.AddRedacted(act.Redacted())
	rep.AddRedacted(locks.Redacted())
	return rep, found
}

type LocksOptions struct {
	Limit      int
	Thresholds rule.Thresholds
}

// Locks builds the locks report: every waiting lock, plus the granted locks
// its direct blockers hold on the same object. The rule judges every row;
// --limit only narrows what is shown.
func Locks(c facts.Context, l facts.LockList, o LocksOptions) *report.Report {
	rep := report.New("locks", c, rule.Locks(l, o.Thresholds))
	type object struct {
		pid      int32
		locktype string
		relation string
	}
	contested := map[object]bool{}
	for _, x := range l.Rows {
		if x.Granted {
			continue
		}
		for _, b := range x.BlockedBy {
			contested[object{b, x.Locktype, deref(x.Relation)}] = true
		}
	}
	var shown []facts.Lock
	for _, x := range l.Rows {
		if !x.Granted || (x.PID != nil && contested[object{*x.PID, x.Locktype, deref(x.Relation)}]) {
			shown = append(shown, x)
		}
	}
	rep.AddProbe(facts.LockListID, l.Status, l.Reason, facts.LockColumns, rows(shown), o.Limit)
	rep.AddRedacted(l.Redacted())
	rep.SetLocks(l.Rows, o.Limit)
	return rep
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// rows renders facts rows in their probe's column order.
func rows[T interface{ Row() []any }](xs []T) [][]any {
	out := make([][]any, len(xs))
	for i, x := range xs {
		out[i] = x.Row()
	}
	return out
}
