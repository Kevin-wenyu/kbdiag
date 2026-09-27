package rule

import (
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

var labLimits = facts.FreezeLimits{Status: facts.StatusOK, Rows: []facts.FreezeLimit{{FreezeMaxAge: 200_000_000, MultiFreezeMaxAge: 400_000_000, FreezeTableAge: 150_000_000}}}

func dbAges(ages ...[2]int32) facts.FreezeDatabases {
	d := facts.FreezeDatabases{Status: facts.StatusOK}
	for i, a := range ages {
		d.Rows = append(d.Rows, facts.FrozenDatabase{Datname: string(rune('a' + i)), XIDAge: a[0], MXIDAge: a[1], AllowConn: true})
	}
	return d
}

func TestFreeze(t *testing.T) {
	cases := []struct {
		name    string
		d       facts.FreezeDatabases
		l       facts.FreezeLimits
		verdict Verdict
		levels  string
	}{
		{"lab", dbAges([2]int32{5364, 0}), labLimits, VerdictOK, ""},
		{"just below the line", dbAges([2]int32{199_999_999, 0}), labLimits, VerdictOK, ""},
		{"at autovacuum_freeze_max_age", dbAges([2]int32{200_000_000, 0}), labLimits, VerdictWARN, "WARN"},
		{"multixact only", dbAges([2]int32{10, 400_000_000}), labLimits, VerdictWARN, "WARN"},
		{"at the stop limit", dbAges([2]int32{facts.XIDStopAge, 0}), labLimits, VerdictFAIL, "FAIL"},
		{"int32 max (xid 0 would read like this)", dbAges([2]int32{1<<31 - 1, 0}), labLimits, VerdictFAIL, "FAIL"},
		{"one finding per database", dbAges([2]int32{300_000_000, 500_000_000}, [2]int32{5, 5}), labLimits, VerdictWARN, "WARN"},
		{"limits not collected: the stop limit still holds", dbAges([2]int32{facts.XIDStopAge + 1, 0}), facts.FreezeLimits{Status: facts.StatusError}, VerdictFAIL, "FAIL"},
		{"limits not collected, young database", dbAges([2]int32{300_000_000, 0}), facts.FreezeLimits{Status: facts.StatusError}, VerdictUNKNOWN, ""},
		{"databases not collected", facts.FreezeDatabases{Status: facts.StatusSkipped}, labLimits, VerdictUNKNOWN, ""},
		{"negative age (clock of a fresh cluster) is not old", dbAges([2]int32{-5, -5}), labLimits, VerdictOK, ""},
		{"no databases", facts.FreezeDatabases{Status: facts.StatusOK}, labLimits, VerdictOK, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Freeze(c.d, c.l, "primary")
			var levels []string
			for _, f := range r.Findings {
				levels = append(levels, string(f.Level))
				if f.ID != "freeze.database_age" {
					t.Errorf("id = %s", f.ID)
				}
			}
			if r.Verdict != c.verdict || strings.Join(levels, ",") != c.levels {
				t.Errorf("verdict=%s levels=%v", r.Verdict, levels)
			}
		})
	}
}

func TestFreezeNext(t *testing.T) {
	d := dbAges([2]int32{250_000_000, 0})
	d.Rows = append(d.Rows, facts.FrozenDatabase{Datname: "template0", XIDAge: 250_000_000})
	r := Freeze(d, labLimits, "standby")
	if len(r.Findings) != 2 {
		t.Fatalf("findings = %+v", r.Findings)
	}
	var cmds []string
	for _, n := range r.Findings[0].Next {
		cmds = append(cmds, n.Command+n.SQL)
	}
	if strings.Join(cmds, "|") != "kbdiag txn|kbdiag slots|kbdiag -d a freeze|VACUUM (FREEZE, VERBOSE) <table>" {
		t.Errorf("next = %v", cmds)
	}
	if !strings.Contains(r.Findings[0].Next[3].Note, "on the primary") {
		t.Errorf("a standby cannot vacuum: %q", r.Findings[0].Next[3].Note)
	}
	// template0 takes no connections: no -d step
	for _, n := range r.Findings[1].Next {
		if strings.Contains(n.Command, "-d") {
			t.Errorf("template0 next = %+v", n)
		}
	}
	s := r.Findings[0].Symptom
	if !strings.Contains(s, "250000000") || !strings.Contains(s, "autovacuum_freeze_max_age 200000000") {
		t.Errorf("symptom = %q", s)
	}
}

func TestFreezeMultixactAndBoth(t *testing.T) {
	r := Freeze(dbAges([2]int32{10, facts.MXIDStopAge}), labLimits, "primary")
	if r.Verdict != VerdictFAIL || !strings.Contains(r.Findings[0].Symptom, "multixacts old, at or past the stop limit") {
		t.Errorf("%s %+v", r.Verdict, r.Findings)
	}
	r = Freeze(dbAges([2]int32{300_000_000, 500_000_000}), labLimits, "primary")
	if s := r.Findings[0].Symptom; !strings.Contains(s, "300000000 xids") || !strings.Contains(s, "500000000 multixacts") {
		t.Errorf("both ages must be named: %q", s)
	}
}

func TestShellWord(t *testing.T) {
	for in, want := range map[string]string{"test": "test", "my_db-2.x": "my_db-2.x", "my db": "'my db'", "it's": `'it'\''s'`, "": "''", "库": "库"} {
		if got := shellWord(in); got != want {
			t.Errorf("shellWord(%q) = %q, want %q", in, got, want)
		}
	}
}
