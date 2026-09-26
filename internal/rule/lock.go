package rule

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// Locks flags each session that has waited for a lock too long, one finding
// per waiter with its direct blockers (one level; chains are locks --tree).
func Locks(l facts.LockList, th Thresholds) Result {
	judge, unknown := collected(l.Status)
	if !judge {
		return Result{Verdict: verdictOf(nil, unknown)}
	}
	var fs []Finding
	for _, x := range l.Rows {
		if x.Granted || x.PID == nil {
			continue
		}
		if x.WaitS == nil {
			unknown = true // masked or untracked: how long it waited is unseen
			continue
		}
		if *x.WaitS < th.LockWaitWarnS {
			continue
		}
		fs = append(fs, lockWaiting(x))
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}

func lockWaiting(x facts.Lock) Finding {
	target := x.Locktype + " lock"
	if x.Relation != nil {
		target = x.Mode + " on " + *x.Relation
	}
	symptom := fmt.Sprintf("session %d has waited %.0fs for %s", *x.PID, *x.WaitS, target)
	var names []string
	var next []Next
	for _, b := range x.Blockers() {
		if b == facts.PreparedBlocker {
			names = append(names, "an uncommitted two-phase transaction")
			next = append(next, Next{Kind: "verify", Command: "kbdiag txn", Note: "the blocker is an uncommitted two-phase transaction: its gid and how long it has been pending"})
			continue
		}
		names = append(names, strconv.Itoa(int(b)))
		next = append(next, Next{Kind: "verify", Command: fmt.Sprintf("kbdiag session %d", b), Note: "what the blocking session is doing"})
	}
	if len(names) > 0 {
		symptom += ", blocked by " + strings.Join(names, ", ")
	}
	blockers := x.Blockers()
	if blockers == nil {
		blockers = []int32{}
	}
	return Finding{
		ID:      "lock.waiting",
		Level:   LevelWARN,
		Symptom: symptom,
		Evidence: []Evidence{{ProbeID: facts.LockListID, Fields: map[string]any{
			"waiter_pid": *x.PID, "blocker_pids": blockers, "relation": x.Relation, "lock_mode": x.Mode, "wait_s": *x.WaitS,
		}}},
		Next: next,
	}
}
