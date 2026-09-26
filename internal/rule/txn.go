package rule

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// Txn flags long transactions and prepared (2PC) transactions left open.
// Both are WARN: they hold back the vacuum horizon (and a 2PC its locks),
// which hurts later, not the business right now; a lock they cause is
// reported by locks.
func Txn(a facts.SessionActivity, p facts.TxnPrepared, th Thresholds) Result {
	return Merge(longTxn(a, th), prepared(p, th))
}

func longTxn(a facts.SessionActivity, th Thresholds) Result {
	judge, unknown := collected(a.Status)
	if !judge {
		return Result{Verdict: verdictOf(nil, unknown)}
	}
	var fs []Finding
	for _, s := range a.Rows {
		// an untracked row keeps a stale xact_age_s, so it says nothing either
		if s.Masked() || s.Untracked() {
			unknown = true
			continue
		}
		if s.XactAgeS == nil || *s.XactAgeS < th.XactWarnS {
			continue
		}
		state := "-"
		if s.State != nil {
			state = *s.State
		}
		fs = append(fs, Finding{
			ID:      "txn.long",
			Level:   LevelWARN,
			Symptom: fmt.Sprintf("session %d has had a transaction open for %.0fs, now %s", s.PID, *s.XactAgeS, state),
			Evidence: []Evidence{{ProbeID: facts.SessionActivityID, Fields: map[string]any{
				"pid": s.PID, "state": s.State, "xact_age_s": *s.XactAgeS, "backend_xid": s.BackendXID, "backend_xmin": s.BackendXmin,
			}}},
			Next: []Next{{Kind: "verify", Command: fmt.Sprintf("kbdiag session %d", s.PID), Note: "what it is running, which locks it holds, whether it blocks anyone"}},
		})
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}

func prepared(p facts.TxnPrepared, th Thresholds) Result {
	judge, unknown := collected(p.Status)
	if !judge {
		return Result{Verdict: verdictOf(nil, unknown)}
	}
	var fs []Finding
	for _, x := range p.Rows {
		if x.AgeS < th.PreparedWarnS {
			continue
		}
		gid := strings.ReplaceAll(x.GID, "'", "''")
		fix := Next{Kind: "fix", SQL: fmt.Sprintf("ROLLBACK PREPARED '%s'", gid),
			Note: fmt.Sprintf("check with the application whether to commit (COMMIT PREPARED) or roll back; run it connected to database %s, outside a transaction block", x.Database)}
		if strings.IndexFunc(x.GID, unicode.IsControl) >= 0 {
			// the printed text is escaped, so pasting it would not match
			fix = Next{Kind: "verify", Command: "kbdiag txn --json", Note: "the gid has control characters and the text shows it escaped: take the raw gid from the JSON before committing or rolling back"}
		}
		fs = append(fs, Finding{
			ID:      "txn.prepared",
			Level:   LevelWARN,
			Symptom: fmt.Sprintf("two-phase transaction %s prepared %.0fs ago and not finished, holding back the vacuum horizon", x.GID, x.AgeS),
			Evidence: []Evidence{{ProbeID: facts.TxnPreparedID, Fields: map[string]any{
				"gid": x.GID, "owner": x.Owner, "database": x.Database, "age_s": x.AgeS, "transaction": x.Transaction,
			}}},
			Next: []Next{fix},
		})
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}
