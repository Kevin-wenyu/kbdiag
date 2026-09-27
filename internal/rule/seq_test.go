package rule

import (
	"math"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func seq(last *int64, min, max, inc int64, cycle bool) facts.Sequence {
	return facts.Sequence{Schemaname: "public", Sequencename: "s", DataType: "integer", StartValue: map[bool]int64{true: min, false: max}[inc > 0],
		MinValue: min, MaxValue: max, IncrementBy: inc, Cycle: cycle, LastValue: last, Readable: true}
}

func TestSeqLeft(t *testing.T) {
	v := func(n int64) *int64 { return &n }
	cases := []struct {
		s    facts.Sequence
		left string
		used float64
		ok   bool
	}{
		{seq(v(1311000), 1, math.MaxInt32, 1, false), "2146172647", 1310999.0 / (math.MaxInt32 - 1), true},
		{seq(v(math.MaxInt32), 1, math.MaxInt32, 1, false), "0", 1, true},
		{seq(v(math.MaxInt32-1), 1, math.MaxInt32, 1, false), "1", 1 - 1.0/(math.MaxInt32-1), true},
		{seq(v(10), 1, 10, 3, false), "0", 1, true},
		{seq(v(7), 1, 10, 3, false), "1", 6.0 / 9, true},
		{seq(v(-100), math.MinInt64, -1, -1, false), "9223372036854775708", 99.0 / (math.MaxInt64 - 1 + 1), true},
		{seq(v(math.MaxInt64), 1, math.MaxInt64, 1, false), "0", 1, true},
		{seq(v(math.MinInt64), math.MinInt64, math.MaxInt64, -5, false), "0", 1, true},
		{seq(nil, 1, 10, 1, false), "", 0, false},
	}
	for _, c := range cases {
		left, used, ok := SeqLeft(c.s)
		got := ""
		if ok {
			got = left.String()
		}
		if ok != c.ok || got != c.left || ok && math.Abs(used-c.used) > 1e-9 {
			t.Errorf("%+v: left %s used %v ok %v, want %s %v %v", c.s, got, used, ok, c.left, c.used, c.ok)
		}
	}
}

func TestSeq(t *testing.T) {
	v := func(n int64) *int64 { return &n }
	list := func(ss ...facts.Sequence) facts.SeqList { return facts.SeqList{Status: facts.StatusOK, Rows: ss} }
	hidden := seq(nil, 1, 10, 1, false)
	hidden.Readable = false
	cases := []struct {
		name    string
		l       facts.SeqList
		verdict Verdict
		n       int
	}{
		{"lab", list(seq(v(1311000), 1, math.MaxInt32, 1, false), seq(nil, 1, 10, 1, false)), VerdictOK, 0},
		{"exhausted", list(seq(v(10), 1, 10, 1, false)), VerdictFAIL, 1},
		{"exhausted but cycles", list(seq(v(10), 1, 10, 1, true)), VerdictOK, 0},
		{"one step left", list(seq(v(9), 1, 10, 1, false)), VerdictOK, 0},
		{"descending, at the minimum", list(seq(v(1), 1, 100, -1, false)), VerdictFAIL, 1},
		{"hidden from this account", list(hidden), VerdictUNKNOWN, 0},
		{"hidden, and one exhausted", list(hidden, seq(v(10), 1, 10, 1, false)), VerdictFAIL, 1},
		{"not collected", facts.SeqList{Status: facts.StatusError}, VerdictUNKNOWN, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Seq(c.l, "primary")
			if r.Verdict != c.verdict || len(r.Findings) != c.n {
				t.Errorf("verdict=%s findings=%+v", r.Verdict, r.Findings)
			}
			for _, f := range r.Findings {
				if f.ID != "seq.exhausted" || f.Level != LevelFAIL {
					t.Errorf("%+v", f)
				}
			}
		})
	}
}

func TestSeqStandbyAndFix(t *testing.T) {
	v := func(n int64) *int64 { return &n }
	full := facts.SeqList{Status: facts.StatusOK, Rows: []facts.Sequence{seq(v(10), 1, 10, 1, false)}}
	if r := Seq(full, "standby"); r.Verdict != VerdictOK || len(r.Findings) != 0 {
		t.Errorf("standby: %s %+v", r.Verdict, r.Findings)
	}
	for _, c := range []struct {
		s   facts.Sequence
		sql string
	}{
		{seq(v(3), 1, 3, 1, false), "ALTER SEQUENCE public.s MAXVALUE <higher>"}, // e2e/inject/seq.sh: integer maxvalue 3
		{seq(v(math.MaxInt32), 1, math.MaxInt32, 1, false), "ALTER SEQUENCE public.s AS bigint"},
		{seq(v(-5), -5, -1, -1, false), "ALTER SEQUENCE public.s MINVALUE <lower>"},
	} {
		if n := seqFix(c.s); n.SQL != c.sql {
			t.Errorf("%+v: %q, want %q", c.s, n.SQL, c.sql)
		}
	}
	// wrapped below its start: nothing used, not negative
	s := facts.Sequence{DataType: "integer", StartValue: 1000, MinValue: 1, MaxValue: 2000, IncrementBy: 1, Cycle: true, LastValue: v(5), Readable: true}
	if _, used, _ := SeqLeft(s); used != 0 {
		t.Errorf("used = %v", used)
	}
}
