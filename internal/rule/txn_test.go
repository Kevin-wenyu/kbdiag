package rule

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func inTxn(pid int32, age float64) facts.Session {
	return facts.Session{PID: pid, State: str("active"), XactAgeS: f64(age), BackendXmin: xid(5855)}
}
func prep(gid string, age float64) facts.Prepared {
	return facts.Prepared{GID: gid, Owner: "system", Database: "test", AgeS: age, Transaction: 5870}
}
func preps(rows ...facts.Prepared) facts.TxnPrepared {
	return facts.TxnPrepared{Status: facts.StatusOK, Rows: rows}
}

var tth = Thresholds{XactWarnS: 300, PreparedWarnS: 900}

func TestTxn(t *testing.T) {
	none := preps()
	na := facts.TxnPrepared{Status: facts.StatusNotApplicable, Reason: "standby"}
	cases := []struct {
		name    string
		a       facts.SessionActivity
		p       facts.TxnPrepared
		verdict Verdict
		flagged []string // id:level:pid-or-gid
	}{
		// normal / negative
		{"nothing open", ok(), none, VerdictOK, nil},
		{"no transaction", ok(facts.Session{PID: 1, State: str("idle")}), none, VerdictOK, nil},
		{"short transaction", ok(inTxn(1, 5)), none, VerdictOK, nil},
		// boundaries
		{"just below warn", ok(inTxn(1, 299.9)), none, VerdictOK, nil},
		{"at warn", ok(inTxn(1, 300)), none, VerdictWARN, []string{"txn.long:WARN:1"}},
		{"30 minutes is still WARN", ok(inTxn(1, 1800)), none, VerdictWARN, []string{"txn.long:WARN:1"}},
		{"a day is still WARN", ok(inTxn(1, 86400)), none, VerdictWARN, []string{"txn.long:WARN:1"}},
		{"idle in txn is a long txn too", ok(facts.Session{PID: 2, State: str("idle in transaction"), XactAgeS: f64(400)}), none, VerdictWARN, []string{"txn.long:WARN:2"}},
		{"prepared just below", ok(), preps(prep("g", 899.9)), VerdictOK, nil},
		{"prepared at warn", ok(), preps(prep("g", 900)), VerdictWARN, []string{"txn.prepared:WARN:g"}},
		{"both kinds", ok(inTxn(1, 301)), preps(prep("g", 1e7)), VerdictWARN, []string{"txn.long:WARN:1", "txn.prepared:WARN:g"}},
		// null / illegal values
		{"null xact age", ok(facts.Session{PID: 1, State: str("active"), BackendXmin: xid(1)}), none, VerdictOK, nil},
		{"negative age", ok(inTxn(1, -3)), preps(prep("g", -3)), VerdictOK, nil},
		// hidden rows: masked, and untracked whose xact age is stale
		{"masked row", ok(masked(1)), none, VerdictUNKNOWN, nil},
		{"untracked row with stale age", ok(facts.Session{PID: 1, State: str("disabled"), XactAgeS: f64(99999)}), none, VerdictUNKNOWN, nil},
		{"masked row but a visible WARN", ok(masked(1), inTxn(2, 2000)), none, VerdictWARN, []string{"txn.long:WARN:2"}},
		// collection status
		{"standby: prepared not applicable", ok(), na, VerdictOK, nil},
		{"standby: long txn still judged", ok(inTxn(1, 301)), na, VerdictWARN, []string{"txn.long:WARN:1"}},
		{"activity skipped", facts.SessionActivity{Status: facts.StatusSkipped}, none, VerdictUNKNOWN, nil},
		{"prepared error", ok(), facts.TxnPrepared{Status: facts.StatusError}, VerdictUNKNOWN, nil},
		{"prepared error but a visible WARN", ok(inTxn(1, 301)), facts.TxnPrepared{Status: facts.StatusError}, VerdictWARN, []string{"txn.long:WARN:1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Txn(c.a, c.p, tth)
			if r.Verdict != c.verdict {
				t.Errorf("verdict = %s, want %s", r.Verdict, c.verdict)
			}
			var got []string
			for _, f := range r.Findings {
				key := f.Evidence[0].Fields["pid"]
				if key == nil {
					key = f.Evidence[0].Fields["gid"]
				}
				got = append(got, fmt.Sprintf("%s:%s:%v", f.ID, f.Level, key))
			}
			if fmt.Sprint(got) != fmt.Sprint(c.flagged) {
				t.Errorf("findings = %v, want %v", got, c.flagged)
			}
		})
	}
}

func TestTxnPreparedFix(t *testing.T) {
	r := Txn(ok(), preps(prep("it's; drop table x", 1000)), tth)
	f := r.Findings[0]
	if f.Next[0].Kind != "fix" || f.Next[0].SQL != "ROLLBACK PREPARED 'it''s; drop table x'" {
		t.Errorf("next = %+v", f.Next)
	}
	if !strings.Contains(f.Next[0].Note, "test") {
		t.Errorf("note must name the database: %q", f.Next[0].Note)
	}
	for _, k := range []string{"gid", "owner", "database", "age_s", "transaction"} {
		if _, ok := f.Evidence[0].Fields[k]; !ok {
			t.Errorf("evidence missing %s", k)
		}
	}
}

func TestTxnLongContent(t *testing.T) {
	f := Txn(ok(inTxn(42, 1830.4)), preps(), tth).Findings[0]
	if f.Symptom != "会话 42 的事务已开了 1830 秒，当前 active" || f.Next[0].Command != "kbdiag session 42" {
		t.Errorf("finding = %+v", f)
	}
	for _, k := range []string{"pid", "state", "xact_age_s", "backend_xid", "backend_xmin"} {
		if _, ok := f.Evidence[0].Fields[k]; !ok {
			t.Errorf("evidence missing %s", k)
		}
	}
}
