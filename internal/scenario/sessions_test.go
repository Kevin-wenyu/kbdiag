package scenario

import (
	"bytes"
	"encoding/json"
	"flag"
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

func TestSessionsActiveOnly(t *testing.T) {
	c, a := prdFacts()
	masked := facts.Session{PID: 9, Usename: str("system"), Query: str("<insufficient privilege>")}
	untracked := facts.Session{PID: 8, Usename: str("system"), State: str("disabled"), Query: str("")}
	a.Rows = append(a.Rows, masked, untracked)
	rep := Sessions(c, a, SessionsOptions{ActiveOnly: true, Limit: 50, Thresholds: rule.Defaults})
	rows := rep.Data[facts.SessionActivityID].Rows
	var pids []int32
	for _, r := range rows {
		pids = append(pids, r[0].(int32))
	}
	if !reflect.DeepEqual(pids, []int32{236188, 9, 8}) {
		t.Errorf("--active rows = %v, want active + masked + untracked", pids)
	}
	// --active only filters what is shown: the hidden idle-in-txn row is still judged.
	if rep.Verdict != rule.VerdictWARN || len(rep.Findings) != 1 || rep.Findings[0].Evidence[0].Fields["pid"] != int32(236201) {
		t.Errorf("verdict=%s findings=%+v", rep.Verdict, rep.Findings)
	}
	if len(rep.Redacted) == 0 {
		t.Error("masked row must be listed in redacted")
	}
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
	rep := Sessions(c, a, SessionsOptions{ActiveOnly: true, Limit: 50, Thresholds: rule.Defaults})
	p := rep.Data[facts.SessionActivityID]
	if rep.Verdict != rule.VerdictUNKNOWN || p.Status != facts.StatusSkipped || *p.Reason != "track_activities=off" || len(p.Rows) != 0 {
		t.Errorf("verdict=%s probe=%+v", rep.Verdict, p)
	}
}
