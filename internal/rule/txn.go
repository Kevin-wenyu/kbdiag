package rule

import (
	"fmt"
	"strings"

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
			Symptom: fmt.Sprintf("会话 %d 的事务已开了 %.0f 秒，当前 %s", s.PID, *s.XactAgeS, state),
			Evidence: []Evidence{{ProbeID: facts.SessionActivityID, Fields: map[string]any{
				"pid": s.PID, "state": s.State, "xact_age_s": *s.XactAgeS, "backend_xid": s.BackendXID, "backend_xmin": s.BackendXmin,
			}}},
			Next: []Next{{Kind: "verify", Command: fmt.Sprintf("kbdiag session %d", s.PID), Note: "看它在跑什么、持有哪些锁、有没有挡住别人"}},
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
		fs = append(fs, Finding{
			ID:      "txn.prepared",
			Level:   LevelWARN,
			Symptom: fmt.Sprintf("两阶段事务 %s 已 prepare %.0f 秒未结束，压着视界", x.GID, x.AgeS),
			Evidence: []Evidence{{ProbeID: facts.TxnPreparedID, Fields: map[string]any{
				"gid": x.GID, "owner": x.Owner, "database": x.Database, "age_s": x.AgeS, "transaction": x.Transaction,
			}}},
			Next: []Next{{Kind: "fix", SQL: fmt.Sprintf("ROLLBACK PREPARED '%s'", gid),
				Note: fmt.Sprintf("先和应用确认它该提交还是回滚（提交用 COMMIT PREPARED）；要连到库 %s 执行，且不能放在事务块里", x.Database)}},
		})
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}
