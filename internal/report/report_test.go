package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

func TestExitCode(t *testing.T) {
	want := map[rule.Verdict]int{
		rule.VerdictOK: 0, rule.VerdictWARN: 1, rule.VerdictFAIL: 2, rule.VerdictUNKNOWN: 3, "": 3, "bogus": 3,
	}
	for v, code := range want {
		if got := ExitCode(v); got != code {
			t.Errorf("ExitCode(%q) = %d, want %d", v, got, code)
		}
	}
	if ExitUsage != 64 || ExitUnavailable != 69 {
		t.Errorf("usage/unavailable codes changed: %d/%d", ExitUsage, ExitUnavailable)
	}
}

func rows(n int) [][]any {
	out := make([][]any, n)
	for i := range out {
		out[i] = []any{i}
	}
	return out
}

func TestAddProbeTruncation(t *testing.T) {
	cases := []struct {
		name            string
		st              facts.Status
		n, limit        int
		shown, truncate int
	}{
		{"under limit", facts.StatusOK, 3, 5, 3, 0},
		{"at limit", facts.StatusOK, 5, 5, 5, 0},
		{"over limit", facts.StatusOK, 100000, 50, 50, 99950},
		{"no limit", facts.StatusOK, 7, 0, 7, 0},
		{"negative limit means no limit", facts.StatusOK, 7, -1, 7, 0},
		{"empty", facts.StatusOK, 0, 50, 0, 0},
		{"skipped never carries rows", facts.StatusSkipped, 5, 50, 0, 0},
		{"error never carries rows", facts.StatusError, 5, 50, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := New("x", facts.Context{}, rule.Result{})
			r.AddProbe("p", c.st, "", []string{"n"}, rows(c.n), c.limit)
			p := r.Data["p"]
			if len(p.Rows) != c.shown || p.Truncated != c.truncate {
				t.Errorf("rows=%d truncated=%d, want %d/%d", len(p.Rows), p.Truncated, c.shown, c.truncate)
			}
			if p.Rows == nil {
				t.Error("rows must be [] not null")
			}
		})
	}
}

// Empty collections render as [] and an empty reason as null, never omitted.
func TestJSONEmptyShapes(t *testing.T) {
	r := New("sessions", facts.Context{CollectedAt: time.Unix(0, 0)}, rule.Result{Verdict: rule.VerdictUNKNOWN})
	r.AddProbe(facts.SessionActivityID, facts.StatusSkipped, "track_activities=off", facts.SessionColumns, nil, 50)
	var buf bytes.Buffer
	if err := r.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"findings", "redacted"} {
		if _, ok := m[k].([]any); !ok {
			t.Errorf("%s = %v, want []", k, m[k])
		}
	}
	p := m["data"].(map[string]any)[facts.SessionActivityID].(map[string]any)
	if p["reason"] != "track_activities=off" || p["status"] != "skipped" {
		t.Errorf("probe = %v", p)
	}
	if _, ok := p["rows"].([]any); !ok {
		t.Errorf("rows = %v, want []", p["rows"])
	}
}

func TestCell(t *testing.T) {
	s := "select  *\n from   t"
	long := strings.Repeat("é", 100)
	var nilStr *string
	cases := []struct {
		in   any
		want string
	}{
		{nil, "-"},
		{nilStr, "-"},
		{&s, "select * from t"},
		{int32(7), "7"},
		{long, strings.Repeat("é", maxCell-3) + "..."},
		{"\x1b[2Jboom", `\x1b[2Jboom`},
		{"a\x00b\u009bc", `a\x00b\u009bc`},
		{"中文 ok", "中文 ok"},
	}
	for _, c := range cases {
		if got := cell(c.in); got != c.want {
			t.Errorf("cell(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
