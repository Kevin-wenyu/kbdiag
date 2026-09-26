package rule

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func i32(v int32) *int32 { return &v }

func waiter(pid int32, wait float64, blockers ...int32) facts.Lock {
	return facts.Lock{PID: i32(pid), Locktype: "relation", Relation: str("public.t"), Mode: "AccessShareLock", WaitS: f64(wait), BlockedBy: blockers}
}
func holder(pid int32) facts.Lock {
	return facts.Lock{PID: i32(pid), Locktype: "relation", Relation: str("public.t"), Mode: "AccessExclusiveLock", Granted: true}
}
func locks(rows ...facts.Lock) facts.LockList {
	return facts.LockList{Status: facts.StatusOK, Rows: rows}
}

var lth = Thresholds{LockWaitWarnS: 10}

func TestLocks(t *testing.T) {
	cases := []struct {
		name    string
		in      facts.LockList
		verdict Verdict
		pids    []int32 // waiters flagged by lock.waiting
	}{
		// normal / negative
		{"no locks", locks(), VerdictOK, nil},
		{"only granted", locks(holder(1), holder(2)), VerdictOK, nil},
		{"granted row with a stray wait_s is not a waiter", locks(facts.Lock{PID: i32(1), Granted: true, WaitS: f64(999)}), VerdictOK, nil},
		// threshold boundary
		{"just below", locks(holder(1), waiter(2, 9.9, 1)), VerdictOK, nil},
		{"at threshold", locks(holder(1), waiter(2, 10, 1)), VerdictWARN, []int32{2}},
		{"one finding per waiter, input order", locks(holder(1), waiter(3, 50, 1), waiter(2, 20, 1), waiter(4, 1, 1)), VerdictWARN, []int32{3, 2}},
		{"huge wait", locks(waiter(2, 1e9, 1)), VerdictWARN, []int32{2}},
		// null / illegal values
		{"negative wait (clock skew)", locks(waiter(2, -5, 1)), VerdictOK, nil},
		{"waiting row without pid is skipped", locks(facts.Lock{Locktype: "relation", WaitS: f64(99)}), VerdictOK, nil},
		{"blocker already gone", locks(waiter(2, 30)), VerdictWARN, []int32{2}},
		// wait_s unseen: masked or untracked waiter
		{"waiter with null wait_s", locks(facts.Lock{PID: i32(2), Masked: true}), VerdictUNKNOWN, nil},
		{"null wait_s but a visible WARN", locks(facts.Lock{PID: i32(2), Masked: true}, waiter(3, 30, 1)), VerdictWARN, []int32{3}},
		{"masked holder does not matter", locks(facts.Lock{PID: i32(1), Granted: true, Masked: true}), VerdictOK, nil},
		// collection failure never reads as OK
		{"skipped", facts.LockList{Status: facts.StatusSkipped, Reason: "timeout"}, VerdictUNKNOWN, nil},
		{"error", facts.LockList{Status: facts.StatusError}, VerdictUNKNOWN, nil},
		{"not_applicable", facts.LockList{Status: facts.StatusNotApplicable}, VerdictOK, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Locks(c.in, lth)
			if r.Verdict != c.verdict {
				t.Errorf("verdict = %s, want %s", r.Verdict, c.verdict)
			}
			var got []int32
			for _, f := range r.Findings {
				if f.ID != "lock.waiting" || f.Level != LevelWARN {
					t.Errorf("unexpected finding %s/%s", f.ID, f.Level)
				}
				got = append(got, f.Evidence[0].Fields["waiter_pid"].(int32))
			}
			if fmt.Sprint(got) != fmt.Sprint(c.pids) {
				t.Errorf("flagged = %v, want %v", got, c.pids)
			}
		})
	}
}

func TestLockWaitingContent(t *testing.T) {
	cases := []struct {
		name    string
		in      facts.Lock
		symptom string
		next    []string // commands
	}{
		{"one blocker", facts.Lock{PID: i32(236155), Locktype: "relation", Relation: str("public.kbdiag_inj_lock"), Mode: "AccessShareLock", WaitS: f64(44), BlockedBy: []int32{236153}},
			"session 236155 has waited 44s for AccessShareLock on public.kbdiag_inj_lock, blocked by 236153", []string{"kbdiag session 236153"}},
		{"two blockers", waiter(5, 12.6, 1, 2), "session 5 has waited 13s for AccessShareLock on public.t, blocked by 1, 2", []string{"kbdiag session 1", "kbdiag session 2"}},
		{"prepared transaction", waiter(5, 11, 0), "session 5 has waited 11s for AccessShareLock on public.t, blocked by an uncommitted two-phase transaction", []string{"kbdiag txn"}},
		{"no relation", facts.Lock{PID: i32(5), Locktype: "transactionid", Mode: "ShareLock", WaitS: f64(20), BlockedBy: []int32{7}},
			"session 5 has waited 20s for transactionid lock, blocked by 7", []string{"kbdiag session 7"}},
		{"blocker gone", waiter(5, 20), "session 5 has waited 20s for AccessShareLock on public.t", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := Locks(locks(c.in), lth).Findings[0]
			if f.Symptom != c.symptom {
				t.Errorf("symptom = %q, want %q", f.Symptom, c.symptom)
			}
			var cmds []string
			for _, n := range f.Next {
				cmds = append(cmds, n.Command)
			}
			if fmt.Sprint(cmds) != fmt.Sprint(c.next) {
				t.Errorf("next = %v, want %v", cmds, c.next)
			}
			ev := f.Evidence[0]
			if ev.ProbeID != facts.LockListID {
				t.Errorf("probe_id = %s", ev.ProbeID)
			}
			for _, k := range []string{"waiter_pid", "blocker_pids", "relation", "lock_mode", "wait_s"} {
				if _, ok := ev.Fields[k]; !ok {
					t.Errorf("evidence missing %s", k)
				}
			}
			if ev.Fields["blocker_pids"] == nil {
				t.Error("blocker_pids must be an array, not null")
			}
		})
	}
}

func TestMerge(t *testing.T) {
	warn := Result{Verdict: VerdictWARN, Findings: []Finding{{ID: "a", Level: LevelWARN}}}
	fail := Result{Verdict: VerdictFAIL, Findings: []Finding{{ID: "b", Level: LevelFAIL}}}
	unknown := Result{Verdict: VerdictUNKNOWN}
	okr := Result{Verdict: VerdictOK}
	cases := []struct {
		in   []Result
		want Verdict
		n    int
	}{
		{nil, VerdictOK, 0},
		{[]Result{okr, okr}, VerdictOK, 0},
		{[]Result{okr, unknown}, VerdictUNKNOWN, 0},
		{[]Result{unknown, warn}, VerdictWARN, 1},
		{[]Result{warn, fail, unknown}, VerdictFAIL, 2},
	}
	for i, c := range cases {
		r := Merge(c.in...)
		if r.Verdict != c.want || len(r.Findings) != c.n {
			t.Errorf("case %d: verdict=%s findings=%d, want %s/%d", i, r.Verdict, len(r.Findings), c.want, c.n)
		}
	}
}

// Two prepared transactions in the way give blocked_by {0,0}: one blocker,
// named once, one next step.
func TestLocksDuplicateBlockers(t *testing.T) {
	rel := str("public.t")
	for _, by := range [][]int32{{0, 0}, {100, 100}} {
		l := facts.LockList{Status: facts.StatusOK, Rows: []facts.Lock{
			{PID: i32(1), Locktype: "relation", Relation: rel, Mode: "AccessShareLock", WaitS: f64(20), BlockedBy: by},
		}}
		r := Locks(l, Defaults)
		if len(r.Findings) != 1 || len(r.Findings[0].Next) != 1 || strings.Count(r.Findings[0].Symptom, "、") != 0 {
			t.Errorf("blocked_by %v: findings %+v", by, r.Findings)
		}
	}
}
