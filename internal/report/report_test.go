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

// size agrees with pg_size_pretty, including where it switches units.
func TestSize(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0 bytes"}, {10239, "10239 bytes"}, {10240, "10 kB"}, {10239 * 1024, "10239 kB"}, {10240*1024 - 1, "10 MB"},
		{10240 * 1024, "10 MB"}, {340459571, "325 MB"}, {400819359, "382 MB"}, {15614003, "15 MB"},
		{15089946624, "14 GB"}, {213452304384, "199 GB"}, {198362357760, "185 GB"},
		{1.5 * (1 << 20), "1536 kB"}, {10.5 * (1 << 30), "11 GB"}, {1 << 60, "1024 PB"},
	}
	for _, c := range cases {
		if got := size(c.in); got != c.want {
			t.Errorf("size(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDuration(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0s"}, {0.9, "0s"}, {8.0, "8s"}, {59.9, "59s"}, {60, "1m 0s"}, {187, "3m 7s"}, {3600, "1h 0m"},
		{3661, "1h 1m"}, {86399, "23h 59m"}, {86400, "1d 0h"}, {268991, "3d 2h"}, {504535, "5d 20h"},
		{400 * 86400, "400d 0h"}, {-5, "0s"},
	}
	for _, c := range cases {
		if got := duration(c.in); got != c.want {
			t.Errorf("duration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNumber(t *testing.T) {
	i32, f, u := int32(7), 1.5, uint64(9)
	var nilp *float64
	cases := []struct {
		in   any
		want float64
		ok   bool
	}{
		{i32, 7, true}, {&i32, 7, true}, {f, 1.5, true}, {&f, 1.5, true}, {u, 9, true},
		{nil, 0, false}, {nilp, 0, false}, {"7", 0, false},
	}
	for _, c := range cases {
		if got, ok := number(c.in); got != c.want || ok != c.ok {
			t.Errorf("number(%#v) = %v %v", c.in, got, ok)
		}
	}
}

// A reason from the server is escaped like any other cell.
func TestStatusReasonEscaped(t *testing.T) {
	r := New("status", facts.Context{}, rule.Result{Verdict: rule.VerdictUNKNOWN})
	r.AddProbe(facts.InstInfoID, facts.StatusError, "XX000: \x1b[2J", facts.InfoColumns, nil, 0)
	var buf bytes.Buffer
	if err := r.WriteText(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\x1b") || !strings.Contains(buf.String(), `inst.info: error  (XX000: \x1b[2J)`) {
		t.Errorf("text = %q", buf.String())
	}
}
