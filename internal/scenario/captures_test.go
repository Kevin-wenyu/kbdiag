package scenario

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// A capture is one kbdiag --json run on node1/node2 during stage 0 of the
// polish plan (e2e/testdata/captures, 2026-09-26 19:40-19:45). Tests rebuild
// facts from it, so goldens are driven by what KES really returned.
type capture struct {
	Context struct {
		Version     string `json:"version"`
		Role        string `json:"role"`
		Location    string `json:"location"`
		User        string `json:"user"`
		CollectedAt string `json:"collected_at"`
	} `json:"context"`
	Data map[string]struct {
		Status  facts.Status `json:"status"`
		Reason  *string      `json:"reason"`
		Columns []string     `json:"columns"`
		Rows    [][]any      `json:"rows"`
	} `json:"data"`
}

func loadCapture(t *testing.T, name string) capture {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "e2e", "testdata", "captures", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if i := bytes.Index(b, []byte("\nEXIT_CODE=")); i >= 0 {
		b = b[:i]
	}
	// stderr was captured into the same file: skip to the report.
	if i := bytes.Index(b, []byte("{\n")); i > 0 {
		b = b[i:]
	}
	var c capture
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&c); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return c
}

func (c capture) context(t *testing.T) facts.Context {
	t.Helper()
	at, err := time.Parse(time.RFC3339, c.Context.CollectedAt)
	if err != nil {
		t.Fatal(err)
	}
	return facts.Context{Version: c.Context.Version, Role: c.Context.Role, Location: c.Context.Location, User: c.Context.User, CollectedAt: at}
}

// rows returns the probe's rows as column → value maps, with its status.
func (c capture) rows(t *testing.T, id string) (facts.Status, string, []map[string]any) {
	t.Helper()
	p, ok := c.Data[id]
	if !ok {
		t.Fatalf("capture has no %s", id)
	}
	reason := ""
	if p.Reason != nil {
		reason = *p.Reason
	}
	out := make([]map[string]any, len(p.Rows))
	for i, r := range p.Rows {
		m := map[string]any{}
		for j, col := range p.Columns {
			m[col] = r[j]
		}
		out[i] = m
	}
	return p.Status, reason, out
}

func cStr(v any) *string {
	if v == nil {
		return nil
	}
	s := v.(string)
	return &s
}

func cF64(v any) *float64 {
	if v == nil {
		return nil
	}
	f, err := v.(json.Number).Float64()
	if err != nil {
		panic(err)
	}
	return &f
}

func cI64(v any) *int64 {
	if v == nil {
		return nil
	}
	n, err := v.(json.Number).Int64()
	if err != nil {
		panic(err)
	}
	return &n
}

func cI32(v any) *int32 {
	n := cI64(v)
	if n == nil {
		return nil
	}
	x := int32(*n)
	return &x
}

func cU32(v any) *uint32 {
	n := cI64(v)
	if n == nil {
		return nil
	}
	x := uint32(*n)
	return &x
}

func cI32s(v any) []int32 {
	var out []int32
	for _, x := range v.([]any) {
		out = append(out, *cI32(x))
	}
	return out
}

// sessionActivity rebuilds session.activity from a capture. Captures cap
// nothing: they were taken with --limit 0 or had fewer than 50 rows.
func (c capture) sessionActivity(t *testing.T) facts.SessionActivity {
	t.Helper()
	st, reason, rows := c.rows(t, facts.SessionActivityID)
	a := facts.SessionActivity{Status: st, Reason: reason}
	for _, m := range rows {
		a.Rows = append(a.Rows, facts.Session{
			PID: *cI32(m["pid"]), Usename: cStr(m["usename"]), Datname: cStr(m["datname"]),
			ApplicationName: cStr(m["application_name"]), ClientAddr: cStr(m["client_addr"]), BackendType: cStr(m["backend_type"]),
			State: cStr(m["state"]), BackendXID: cU32(m["backend_xid"]), BackendXmin: cU32(m["backend_xmin"]),
			XactAgeS: cF64(m["xact_age_s"]), QueryAgeS: cF64(m["query_age_s"]), StateAgeS: cF64(m["state_age_s"]),
			WaitEventType: cStr(m["wait_event_type"]), WaitEvent: cStr(m["wait_event"]), Query: cStr(m["query"]),
		})
	}
	return a
}

// lockList rebuilds lock.list from a capture. masked is not in the JSON: a
// waiting row without wait_s is masked (only kbdiag_ro captures have them).
func (c capture) lockList(t *testing.T) facts.LockList {
	t.Helper()
	st, reason, rows := c.rows(t, facts.LockListID)
	l := facts.LockList{Status: st, Reason: reason}
	for _, m := range rows {
		x := facts.Lock{PID: cI32(m["pid"]), Locktype: m["locktype"].(string), Relation: cStr(m["relation"]),
			Mode: m["mode"].(string), Granted: m["granted"].(bool), WaitS: cF64(m["wait_s"]), BlockedBy: cI32s(m["blocked_by"])}
		x.Masked = !x.Granted && x.WaitS == nil
		l.Rows = append(l.Rows, x)
	}
	return l
}

// txnPrepared rebuilds txn.prepared from a capture.
func (c capture) txnPrepared(t *testing.T) facts.TxnPrepared {
	t.Helper()
	st, reason, rows := c.rows(t, facts.TxnPreparedID)
	p := facts.TxnPrepared{Status: st, Reason: reason}
	for _, m := range rows {
		at, err := time.Parse(time.RFC3339, m["prepared_at"].(string))
		if err != nil {
			t.Fatal(err)
		}
		p.Rows = append(p.Rows, facts.Prepared{GID: m["gid"].(string), Owner: m["owner"].(string), Database: m["database"].(string),
			PreparedAt: at, AgeS: *cF64(m["age_s"]), Transaction: *cU32(m["transaction"])})
	}
	return p
}

// Every capture file parses: a broken one would silently drop coverage.
func TestCapturesParse(t *testing.T) {
	names, err := filepath.Glob(filepath.Join("..", "..", "e2e", "testdata", "captures", "*.json"))
	if err != nil || len(names) == 0 {
		t.Fatalf("no captures: %v", err)
	}
	for _, n := range names {
		b, _ := os.ReadFile(n)
		if !bytes.Contains(b, []byte("{\n")) {
			continue // a usage error (exit 64) prints no report
		}
		c := loadCapture(t, strings.TrimSuffix(filepath.Base(n), ".json"))
		if c.Context.CollectedAt == "" && len(c.Data) > 0 {
			t.Errorf("%s: no context", n)
		}
	}
}
