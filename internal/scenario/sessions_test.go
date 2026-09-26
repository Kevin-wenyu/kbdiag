package scenario

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

func str(s string) *string   { return &s }
func f64(f float64) *float64 { return &f }
func xid(x uint32) *uint32   { return &x }

// prdFacts reproduces the facts behind the PRD §5.1 sessions example.
func prdFacts() (facts.Context, facts.SessionActivity) {
	c := facts.Context{
		Version: "KingbaseES V008R006C009B0014", Role: "primary", Location: "local", User: "system",
		CollectedAt: time.Date(2026, 9, 23, 21, 40, 5, 0, time.FixedZone("CST", 8*3600)),
	}
	a := facts.SessionActivity{Status: facts.StatusOK, Rows: []facts.Session{
		{PID: 236201, Usename: str("system"), Datname: str("test"), ApplicationName: str("kbdiag_inj_idle_txn"),
			BackendType: str("client backend"), State: str("idle in transaction"), BackendXID: xid(5855), BackendXmin: xid(5855),
			XactAgeS: f64(1830.4), QueryAgeS: f64(1830.4), StateAgeS: f64(1830.4),
			WaitEventType: str("Client"), WaitEvent: str("ClientRead"), Query: str("select txid_current();")},
		{PID: 236188, Usename: str("system"), Datname: str("test"), ApplicationName: str("ksql"), ClientAddr: str("192.168.105.1"),
			BackendType: str("client backend"), State: str("active"), BackendXmin: xid(5855),
			XactAgeS: f64(0.2), QueryAgeS: f64(0.2), StateAgeS: f64(0.2), Query: str("select * from orders where id = 42")},
	}}
	return c, a
}

var defaults = SessionsOptions{Limit: 50, Thresholds: rule.Defaults}

// prdExample extracts the JSON block under "#### 示例：<name>" in docs/PRD.md.
func prdExample(t *testing.T, name string) any {
	t.Helper()
	b, err := os.ReadFile("../../docs/PRD.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	i := strings.Index(doc, "#### 示例："+name+"\n")
	if i < 0 {
		t.Fatalf("PRD has no example %q", name)
	}
	doc = doc[i:]
	start := strings.Index(doc, "```json\n") + len("```json\n")
	end := start + strings.Index(doc[start:], "```")
	var v any
	if err := json.Unmarshal([]byte(doc[start:end]), &v); err != nil {
		t.Fatalf("PRD example %q: %v", name, err)
	}
	return v
}

// The PRD example is the JSON golden: any drift between contract and code fails here.
func TestSessionsMatchesPRD(t *testing.T) {
	c, a := prdFacts()
	var buf bytes.Buffer
	if err := Sessions(c, a, defaults).WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var got any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if want := prdExample(t, "sessions"); !reflect.DeepEqual(got, want) {
		w, _ := json.MarshalIndent(want, "", "  ")
		t.Errorf("JSON differs from PRD §5.1 sessions example\n got: %s\nwant: %s", buf.String(), w)
	}
}

func TestSessionsTextGolden(t *testing.T) {
	c, a := prdFacts()
	assertGolden(t, "sessions", Sessions(c, a, defaults))
}

// sessionsCapture rebuilds a stage-0 capture as sessions facts, at the time
// the appendix A draft was taken.
func sessionsCapture(t *testing.T, name string, at time.Time) (facts.Context, facts.SessionActivity) {
	t.Helper()
	c := loadCapture(t, name)
	ctx := c.context(t)
	if !at.IsZero() {
		ctx.CollectedAt = at
	}
	return ctx, c.sessionActivity(t)
}

func at(h, m, s int) time.Time { return time.Date(2026, 9, 26, h, m, s, 0, cst) }

// The first four goldens are the plan's appendix A drafts, copied by hand.
// Their facts are the stage-0 captures of the same state, taken 15 minutes
// later; the injected rows are set to the values the draft was drawn from.
func TestSessionsText(t *testing.T) {
	lowthr := SessionsOptions{Limit: 50, Thresholds: rule.Thresholds{IdleInTxnWarnS: 1}}
	t.Run("primary injected", func(t *testing.T) {
		c, a := sessionsCapture(t, "sessions_node1_idletxn_longq_lowthr", at(19, 26, 45))
		for i := range a.Rows {
			switch a.Rows[i].PID {
			case 801150:
				a.Rows[i].PID, a.Rows[i].XactAgeS, a.Rows[i].QueryAgeS, a.Rows[i].StateAgeS = 795860, f64(3.4), f64(2.4), f64(1.4)
			case 801314:
				a.Rows[i].PID, a.Rows[i].XactAgeS, a.Rows[i].QueryAgeS, a.Rows[i].StateAgeS = 795975, f64(0.4), f64(0.4), f64(0.4)
			}
		}
		assertGolden(t, "sessions_primary_injected", Sessions(c, a, lowthr))
	})
	t.Run("primary clean", func(t *testing.T) {
		c, a := sessionsCapture(t, "sessions_node1_clean", at(19, 26, 39))
		assertGolden(t, "sessions_primary_clean", Sessions(c, a, defaults))
	})
	t.Run("standby", func(t *testing.T) {
		c, a := sessionsCapture(t, "sessions_node2_clean", at(19, 26, 40))
		assertGolden(t, "sessions_standby", Sessions(c, a, defaults))
	})
	t.Run("kbdiag_ro", func(t *testing.T) {
		c, a := sessionsCapture(t, "sessions_node1_idletxn_longq_ro", at(19, 26, 46))
		rep := Sessions(c, a, defaults)
		if rep.Verdict != rule.VerdictUNKNOWN {
			t.Errorf("verdict = %s", rep.Verdict)
		}
		assertGolden(t, "sessions_ro", rep)
	})

	// Generated from the captures, then read line by line.
	t.Run("--all", func(t *testing.T) {
		c, a := sessionsCapture(t, "sessions_node1_idletxn_longq_lowthr", time.Time{})
		assertGolden(t, "sessions_all", Sessions(c, a, SessionsOptions{All: true, Limit: 50, Thresholds: rule.Thresholds{IdleInTxnWarnS: 1}}))
	})
	t.Run("--all kbdiag_ro", func(t *testing.T) {
		c, a := sessionsCapture(t, "sessions_node1_idletxn_longq_ro", time.Time{})
		assertGolden(t, "sessions_all_ro", Sessions(c, a, SessionsOptions{All: true, Limit: 50, Thresholds: rule.Defaults}))
	})
	t.Run("untracked", func(t *testing.T) {
		c, a := sessionsCapture(t, "sessions_node1_untracked", time.Time{})
		assertGolden(t, "sessions_untracked", Sessions(c, a, defaults))
	})
	t.Run("track_activities off", func(t *testing.T) {
		c, a := sessionsCapture(t, "sessions_node1_trackoff", time.Time{})
		assertGolden(t, "sessions_trackoff", Sessions(c, a, defaults))
	})
	t.Run("connections used up, limited", func(t *testing.T) {
		c, a := sessionsCapture(t, "sessions_node1_conn_all", time.Time{})
		assertGolden(t, "sessions_conn", Sessions(c, a, SessionsOptions{All: true, Limit: 5, Thresholds: rule.Defaults}))
	})
	t.Run("lock injection", func(t *testing.T) {
		c, a := sessionsCapture(t, "sessions_node1_lock", time.Time{})
		assertGolden(t, "sessions_lock", Sessions(c, a, defaults))
	})
}

// The list is what --limit trims; the summary and the findings see every row.
func TestSessionsLimit(t *testing.T) {
	c, _ := prdFacts()
	busy := func(pid int32, xact float64) facts.Session {
		return facts.Session{PID: pid, Usename: str("app"), Datname: str("test"), ApplicationName: str("web"),
			BackendType: str("client backend"), State: str("active"), XactAgeS: f64(xact), QueryAgeS: f64(xact), StateAgeS: f64(xact), Query: str("select 1")}
	}
	a := facts.SessionActivity{Status: facts.StatusOK, Rows: []facts.Session{busy(1, 30), busy(2, 20), busy(3, 10)}}
	cases := []struct {
		limit int
		shown int
		more  bool
	}{{0, 3, false}, {-1, 3, false}, {3, 3, false}, {2, 2, true}, {1, 1, true}}
	for _, x := range cases {
		var buf bytes.Buffer
		rep := Sessions(c, a, SessionsOptions{Limit: x.limit, Thresholds: rule.Defaults})
		if err := rep.WriteText(&buf); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "\n  3      app   test      web          local\n") {
			t.Errorf("limit %d: the summary must count every session:\n%s", x.limit, out)
		}
		if n := strings.Count(out, "select 1"); n != x.shown {
			t.Errorf("limit %d: %d rows listed, want %d", x.limit, n, x.shown)
		}
		if got := strings.Contains(out, fmt.Sprintf("... %d more rows not shown (use --limit 0 to show all)", 3-x.shown)); got != x.more {
			t.Errorf("limit %d: truncation note = %v:\n%s", x.limit, got, out)
		}
		if !strings.Contains(out, "not idle: 3\n") {
			t.Errorf("limit %d: header must count every not-idle session:\n%s", x.limit, out)
		}
	}
}

// Edge cases a real instance produces less often than it could.
func TestSessionsTextEdges(t *testing.T) {
	c, _ := prdFacts()
	long := strings.Repeat("select * from orders where customer_id in (select id from customers) ", 3)
	a := facts.SessionActivity{Status: facts.StatusOK, Rows: []facts.Session{
		// multi-byte and wide characters must stay aligned
		{PID: 10, Usename: str("用户"), Datname: str("库"), ApplicationName: str("报表服务"), ClientAddr: str("10.0.0.1"),
			BackendType: str("client backend"), State: str("active"), XactAgeS: f64(7384), QueryAgeS: f64(65), StateAgeS: f64(65),
			WaitEventType: str("IO"), WaitEvent: str("DataFileRead"), Query: str(long)},
		// every nullable column NULL; a negative age from a clock step
		{PID: 11, BackendType: str("client backend"), State: str("idle in transaction (aborted)"), XactAgeS: f64(-2), Query: str("")},
		// control characters in SQL and application name
		{PID: 12, Usename: str("app"), ApplicationName: str("a\x1b[2Jb"), BackendType: str("client backend"), State: str("active"),
			XactAgeS: f64(0), QueryAgeS: f64(0), StateAgeS: f64(0), Query: str("select\n\t1;\x1b[31m")},
		{PID: 13, Usename: str("app"), BackendType: str("client backend"), State: str("idle")},
		// a long, wide application name in both tables
		{PID: 14, Usename: str("app"), ApplicationName: str(strings.Repeat("长应用名", 20)), BackendType: str("client backend"),
			State: str("active"), XactAgeS: f64(59.6), QueryAgeS: f64(0.5), StateAgeS: f64(0.5), Query: str("select 2")},
		// both redaction reasons in one report: two redacted lines
		{PID: 15, Usename: str("other"), Query: str("<insufficient privilege>")},
		{PID: 16, Usename: str("app"), BackendType: str("client backend"), State: str("disabled"), XactAgeS: f64(9), QueryAgeS: f64(9), Query: str("")},
		// a walsender over the socket comes from "local", as a client does
		{PID: 17, Usename: str("rep"), ApplicationName: str("pg_basebackup"), BackendType: str("walsender"), State: str("active")},
	}}
	rep := Sessions(c, a, SessionsOptions{All: true, Limit: 50, Thresholds: rule.Defaults})
	assertGolden(t, "sessions_edges_all", rep)
	assertGolden(t, "sessions_edges", Sessions(c, a, defaults))

	t.Run("no sessions at all", func(t *testing.T) {
		assertGolden(t, "sessions_empty", Sessions(c, facts.SessionActivity{Status: facts.StatusOK}, defaults))
	})
}

// Findings are judged on every row, not only the rows the limit leaves shown.
func TestSessionsLimitDoesNotHideFindings(t *testing.T) {
	c, a := prdFacts()
	a.Rows[0], a.Rows[1] = a.Rows[1], a.Rows[0]
	rep := Sessions(c, a, SessionsOptions{Limit: 1, Thresholds: rule.Defaults})
	p := rep.Data[facts.SessionActivityID]
	if len(p.Rows) != 1 || p.Truncated != 1 {
		t.Errorf("rows=%d truncated=%d", len(p.Rows), p.Truncated)
	}
	if len(rep.Findings) != 1 || rep.Verdict != rule.VerdictWARN {
		t.Errorf("verdict=%s findings=%d", rep.Verdict, len(rep.Findings))
	}
}

func TestSessionsSkipped(t *testing.T) {
	c, _ := prdFacts()
	a := facts.SessionActivity{Status: facts.StatusSkipped, Reason: "track_activities=off"}
	rep := Sessions(c, a, SessionsOptions{All: true, Limit: 50, Thresholds: rule.Defaults})
	p := rep.Data[facts.SessionActivityID]
	if rep.Verdict != rule.VerdictUNKNOWN || p.Status != facts.StatusSkipped || *p.Reason != "track_activities=off" || len(p.Rows) != 0 {
		t.Errorf("verdict=%s probe=%+v", rep.Verdict, p)
	}
}
