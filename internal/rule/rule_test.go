package rule

import (
	"fmt"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func str(s string) *string   { return &s }
func f64(f float64) *float64 { return &f }
func xid(x uint32) *uint32   { return &x }
func idle(pid int32, age float64) facts.Session {
	return facts.Session{PID: pid, State: str("idle in transaction"), StateAgeS: f64(age), BackendXID: xid(5855)}
}
func ok(rows ...facts.Session) facts.SessionActivity {
	return facts.SessionActivity{Status: facts.StatusOK, Rows: rows}
}
func masked(pid int32) facts.Session {
	return facts.Session{PID: pid, Usename: str("system"), Query: str("<insufficient privilege>")}
}

var th = Thresholds{IdleInTxnWarnS: 300}

func TestSessions(t *testing.T) {
	cases := []struct {
		name    string
		in      facts.SessionActivity
		verdict Verdict
		pids    []int32 // pids flagged by session.idle_in_txn
	}{
		// normal / negative
		{"no sessions", ok(), VerdictOK, nil},
		{"active long query is not idle in txn", ok(facts.Session{PID: 1, State: str("active"), StateAgeS: f64(9999)}), VerdictOK, nil},
		{"idle is not idle in txn", ok(facts.Session{PID: 1, State: str("idle"), StateAgeS: f64(9999)}), VerdictOK, nil},
		// threshold boundary
		{"just below", ok(idle(1, 299.9)), VerdictOK, nil},
		{"at threshold", ok(idle(1, 300)), VerdictWARN, []int32{1}},
		{"just above", ok(idle(1, 300.1)), VerdictWARN, []int32{1}},
		{"aborted txn counts", ok(facts.Session{PID: 7, State: str("idle in transaction (aborted)"), StateAgeS: f64(301)}), VerdictWARN, []int32{7}},
		{"several flagged in input order", ok(idle(3, 400), idle(1, 10), idle(2, 500)), VerdictWARN, []int32{3, 2}},
		// null / illegal values
		{"null state", ok(facts.Session{PID: 1, StateAgeS: f64(9999)}), VerdictOK, nil},
		{"null state_age_s", ok(facts.Session{PID: 1, State: str("idle in transaction")}), VerdictOK, nil},
		{"negative age (clock skew)", ok(idle(1, -5)), VerdictOK, nil},
		{"other state", ok(facts.Session{PID: 1, State: str("fastpath function call"), StateAgeS: f64(9999)}), VerdictOK, nil},
		// state='disabled': track_activities is off for that session (e.g. ALTER ROLE
		// SET), so its real state is unseen even though ours is on
		{"untracked row, nothing flagged", ok(facts.Session{PID: 1, State: str("disabled")}, idle(2, 10)), VerdictUNKNOWN, nil},
		{"untracked row but a visible WARN", ok(facts.Session{PID: 1, State: str("disabled")}, idle(2, 301)), VerdictWARN, []int32{2}},
		// collection failure never reads as OK
		{"skipped", facts.SessionActivity{Status: facts.StatusSkipped, Reason: "track_activities=off"}, VerdictUNKNOWN, nil},
		{"error", facts.SessionActivity{Status: facts.StatusError, Reason: "42P01"}, VerdictUNKNOWN, nil},
		{"not_applicable does not count", facts.SessionActivity{Status: facts.StatusNotApplicable}, VerdictOK, nil},
		// redacted judged columns
		{"masked rows, nothing flagged", ok(masked(1), idle(2, 10)), VerdictUNKNOWN, nil},
		{"masked rows but a visible WARN", ok(masked(1), idle(2, 301)), VerdictWARN, []int32{2}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Sessions(c.in, th)
			if r.Verdict != c.verdict {
				t.Errorf("verdict = %s, want %s", r.Verdict, c.verdict)
			}
			var got []int32
			for _, f := range r.Findings {
				if f.ID != "session.idle_in_txn" || f.Level != LevelWARN {
					t.Errorf("unexpected finding %s/%s", f.ID, f.Level)
				}
				got = append(got, f.Evidence[0].Fields["pid"].(int32))
			}
			if fmt.Sprint(got) != fmt.Sprint(c.pids) {
				t.Errorf("flagged pids = %v, want %v", got, c.pids)
			}
		})
	}
}

func TestSessionsFindingContent(t *testing.T) {
	r := Sessions(ok(idle(236201, 1830.4)), th)
	f := r.Findings[0]
	if f.Symptom != "session 236201 has been idle in transaction for 1830s" {
		t.Errorf("symptom = %q", f.Symptom)
	}
	ev := f.Evidence[0]
	if ev.ProbeID != facts.SessionActivityID {
		t.Errorf("evidence probe_id = %q", ev.ProbeID)
	}
	for _, k := range []string{"pid", "state", "state_age_s", "backend_xid"} {
		if _, ok := ev.Fields[k]; !ok {
			t.Errorf("evidence missing %s", k)
		}
	}
	if len(f.Next) != 1 || f.Next[0].Kind != "verify" || f.Next[0].Command != "kbdiag session 236201" {
		t.Errorf("next = %+v", f.Next)
	}
}

func TestSessionsManyRows(t *testing.T) {
	rows := make([]facts.Session, 100000)
	for i := range rows {
		rows[i] = idle(int32(i+1), 301)
	}
	if r := Sessions(ok(rows...), th); len(r.Findings) != len(rows) {
		t.Errorf("findings = %d, want %d", len(r.Findings), len(rows))
	}
}

func TestVerdictOf(t *testing.T) {
	cases := []struct {
		levels  []Level
		unknown bool
		want    Verdict
	}{
		{nil, false, VerdictOK},
		{nil, true, VerdictUNKNOWN},
		{[]Level{LevelOK}, true, VerdictUNKNOWN},
		{[]Level{LevelWARN}, true, VerdictWARN},
		{[]Level{LevelWARN, LevelFAIL, LevelOK}, false, VerdictFAIL},
		{[]Level{LevelFAIL, LevelWARN}, true, VerdictFAIL},
	}
	for _, c := range cases {
		var fs []Finding
		for _, l := range c.levels {
			fs = append(fs, Finding{Level: l})
		}
		if got := verdictOf(fs, c.unknown); got != c.want {
			t.Errorf("verdictOf(%v, %v) = %s, want %s", c.levels, c.unknown, got, c.want)
		}
	}
}
