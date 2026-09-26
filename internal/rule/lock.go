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
	target := x.Locktype + " 锁"
	if x.Relation != nil {
		target = *x.Relation + " 的 " + x.Mode
	}
	symptom := fmt.Sprintf("会话 %d 等 %s 已 %.0f 秒", *x.PID, target, *x.WaitS)
	var names []string
	var next []Next
	for _, b := range x.Blockers() {
		if b == facts.PreparedBlocker {
			names = append(names, "未提交的两阶段事务")
			next = append(next, Next{Kind: "verify", Command: "kbdiag txn", Note: "挡路的是未提交的两阶段事务，看它的 gid 和已经挂了多久"})
			continue
		}
		names = append(names, strconv.Itoa(int(b)))
		next = append(next, Next{Kind: "verify", Command: fmt.Sprintf("kbdiag session %d", b), Note: "看挡路的会话在干什么"})
	}
	if len(names) > 0 {
		symptom += "，被 " + strings.Join(names, "、") + " 挡住"
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
