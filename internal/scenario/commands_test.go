package scenario

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

func i32(v int32) *int32 { return &v }
func i64(v int64) *int64 { return &v }

var cst = time.FixedZone("CST", 8*3600)

func prdContext(role, user, location string, h, m, s int) facts.Context {
	return facts.Context{Version: "KingbaseES V008R006C009B0014", Role: role, Location: location, User: user,
		CollectedAt: time.Date(2026, 9, 23, h, m, s, 0, cst)}
}

// assertPRD compares a report's JSON with the PRD §5.1 example of the same name.
func assertPRD(t *testing.T, name string, rep *report.Report) {
	t.Helper()
	var buf bytes.Buffer
	if err := rep.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var got any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if want := prdExample(t, name); !reflect.DeepEqual(got, want) {
		w, _ := json.MarshalIndent(want, "", "  ")
		t.Errorf("JSON differs from PRD §5.1 %s example\n got: %s\nwant: %s", name, buf.String(), w)
	}
}

// lockFacts reproduces the lock injection behind the session and locks examples.
func lockFacts(wait float64) facts.LockList {
	rel := str("public.kbdiag_inj_lock")
	return facts.LockList{Status: facts.StatusOK, Rows: []facts.Lock{
		{PID: i32(236153), Locktype: "relation", Relation: rel, Mode: "AccessExclusiveLock", Granted: true},
		{PID: i32(236153), Locktype: "virtualxid", Mode: "ExclusiveLock", Granted: true},
		{PID: i32(236155), Locktype: "relation", Relation: rel, Mode: "AccessShareLock", WaitS: f64(wait), BlockedBy: []int32{236153}},
		{PID: i32(236155), Locktype: "virtualxid", Mode: "ExclusiveLock", Granted: true},
		{PID: i32(3000), Locktype: "relation", Relation: str("public.other"), Mode: "RowExclusiveLock", Granted: true},
	}}
}

func TestSessionMatchesPRD(t *testing.T) {
	a := facts.SessionActivity{Status: facts.StatusOK, Rows: []facts.Session{
		{PID: 236153, Usename: str("system"), State: str("idle in transaction"), StateAgeS: f64(50)},
		{PID: 236155, Usename: str("system"), Datname: str("test"), ApplicationName: str("kbdiag_inj_lock_waiter"),
			BackendType: str("client backend"), State: str("active"), BackendXmin: xid(5860),
			XactAgeS: f64(42), QueryAgeS: f64(42), StateAgeS: f64(42),
			WaitEventType: str("Lock"), WaitEvent: str("relation"), Query: str("select count(*) from kbdiag_inj_lock;")},
	}}
	l := lockFacts(42)
	// the waiter's own granted virtualxid lock is not in the PRD example
	l.Rows = append(l.Rows[:3], l.Rows[4])
	rep, found := Session(prdContext("primary", "system", "local", 21, 41, 10), a, l, SessionOptions{PID: 236155, Thresholds: rule.Defaults})
	if !found {
		t.Fatal("found = false")
	}
	assertPRD(t, "session", rep)
}

func TestSessionShowsWhoItBlocks(t *testing.T) {
	a := facts.SessionActivity{Status: facts.StatusOK, Rows: []facts.Session{{PID: 236153, State: str("idle in transaction"), StateAgeS: f64(50)}}}
	rep, _ := Session(prdContext("primary", "system", "local", 0, 0, 0), a, lockFacts(42), SessionOptions{PID: 236153, Thresholds: rule.Defaults})
	var pids []any
	for _, r := range rep.Data[facts.LockListID].Rows {
		pids = append(pids, r[0])
	}
	// its own two locks, and the lock of the session it blocks
	if len(pids) != 3 {
		t.Errorf("lock rows = %v", pids)
	}
	if rep.Verdict != rule.VerdictWARN || len(rep.Findings) != 1 || rep.Findings[0].ID != "lock.waiting" {
		t.Errorf("verdict=%s findings=%+v", rep.Verdict, rep.Findings)
	}
}

func TestSessionNotFound(t *testing.T) {
	a := facts.SessionActivity{Status: facts.StatusOK, Rows: []facts.Session{{PID: 1}}}
	rep, found := Session(prdContext("primary", "system", "local", 0, 0, 0), a, lockFacts(42), SessionOptions{PID: 99, Thresholds: rule.Defaults})
	if found || rep.Verdict != rule.VerdictUNKNOWN || len(rep.Data[facts.LockListID].Rows) != 0 {
		t.Errorf("found=%v verdict=%s", found, rep.Verdict)
	}
	// activity not collected: we cannot say it is missing
	a.Status = facts.StatusSkipped
	if _, found := Session(prdContext("primary", "system", "local", 0, 0, 0), a, lockFacts(42), SessionOptions{PID: 99, Thresholds: rule.Defaults}); !found {
		t.Error("a skipped probe must not report the pid as missing")
	}
}

func TestLocksMatchesPRD(t *testing.T) {
	rep := Locks(prdContext("primary", "system", "local", 21, 41, 12), lockFacts(44), LocksOptions{Limit: 50, Thresholds: rule.Defaults})
	assertPRD(t, "locks", rep)
}

func TestLocksLimitDoesNotHideFindings(t *testing.T) {
	l := lockFacts(44)
	rep := Locks(prdContext("primary", "system", "local", 0, 0, 0), l, LocksOptions{Limit: 1, Thresholds: rule.Defaults})
	p := rep.Data[facts.LockListID]
	if len(p.Rows) != 1 || p.Truncated != 1 || len(rep.Findings) != 1 {
		t.Errorf("rows=%d truncated=%d findings=%d", len(p.Rows), p.Truncated, len(rep.Findings))
	}
}

func TestLocksRedacted(t *testing.T) {
	l := facts.LockList{Status: facts.StatusOK, Rows: []facts.Lock{{PID: i32(1), Locktype: "relation", Masked: true}}}
	rep := Locks(prdContext("primary", "kbdiag_ro", "remote", 0, 0, 0), l, LocksOptions{Limit: 50, Thresholds: rule.Defaults})
	if rep.Verdict != rule.VerdictUNKNOWN || len(rep.Redacted) != 1 || rep.Redacted[0].Field != "wait_s" || rep.Redacted[0].RowsAffected != 1 {
		t.Errorf("verdict=%s redacted=%+v", rep.Verdict, rep.Redacted)
	}
}

func TestTxnMatchesPRD(t *testing.T) {
	a := facts.SessionActivity{Status: facts.StatusOK, Rows: []facts.Session{
		{PID: 5000, BackendType: str("walreceiver")},
		{PID: 5001, BackendType: str("startup")},
	}}
	p := facts.TxnPrepared{Status: facts.StatusNotApplicable, Reason: "备库看不到主库的两阶段提交事务，请在主库上运行 kbdiag txn"}
	rep := Txn(prdContext("standby", "system", "local", 21, 45, 0), a, p, TxnOptions{Limit: 50, Thresholds: rule.Defaults})
	assertPRD(t, "txn", rep)
}

func TestTxnShowsOnlyTransactions(t *testing.T) {
	a := facts.SessionActivity{Status: facts.StatusOK, Rows: []facts.Session{
		{PID: 1, XactAgeS: f64(3)},
		{PID: 2, BackendXID: xid(10)},
		{PID: 3, BackendXmin: xid(9)},
		{PID: 4, State: str("idle")},
		{PID: 5, Query: str("<insufficient privilege>")},
		{PID: 6, State: str("disabled")},
	}}
	rep := Txn(prdContext("primary", "system", "local", 0, 0, 0), a, facts.TxnPrepared{Status: facts.StatusOK}, TxnOptions{Limit: 50, Thresholds: rule.Defaults})
	var pids []any
	for _, r := range rep.Data[facts.SessionActivityID].Rows {
		pids = append(pids, r[0])
	}
	if !reflect.DeepEqual(pids, []any{int32(1), int32(2), int32(3), int32(5), int32(6)}) {
		t.Errorf("rows = %v", pids)
	}
	if rep.Verdict != rule.VerdictUNKNOWN || len(rep.Redacted) == 0 {
		t.Errorf("verdict=%s redacted=%d", rep.Verdict, len(rep.Redacted))
	}
}

func TestWaitsMatchesPRD(t *testing.T) {
	w := facts.WaitSummary{Status: facts.StatusOK, Rows: []facts.Wait{
		{Sessions: 7, PIDs: []int32{3120, 3121, 3125, 236153, 236155, 236188, 236201}, Masked: 7},
		{State: str("active"), Sessions: 1, PIDs: []int32{237010}},
	}}
	assertPRD(t, "waits", Waits(prdContext("primary", "kbdiag_ro", "remote", 21, 47, 30), w))
}

func TestStatusMatchesPRD(t *testing.T) {
	i := facts.InstInfo{Status: facts.StatusOK, Rows: []facts.Info{{
		Version: "KingbaseES V008R006C009B0014 on x86_64-pc-linux-gnu", StartTime: time.Date(2026, 9, 20, 9, 12, 44, 0, cst),
		UptimeS: 304036, Connections: 12, MaxConnections: 100, SuperuserReserved: 3, DataDirectory: str("/data/kingbase/data"),
	}}}
	d := facts.InstDatabases{Status: facts.StatusOK, Rows: []facts.Database{
		{Datname: "kingbase", SizeBytes: i64(13918723)}, {Datname: "security", SizeBytes: i64(12321059)}, {Datname: "test", SizeBytes: i64(14647811)},
	}}
	n := facts.InstDownstreams{Status: facts.StatusOK, Rows: []int64{1}}
	assertPRD(t, "status", Status(prdContext("primary", "system", "local", 21, 50, 0), i, d, n, rule.Defaults))
}

func TestStatusHiddenSize(t *testing.T) {
	d := facts.InstDatabases{Status: facts.StatusOK, Rows: []facts.Database{{Datname: "secret"}}}
	rep := Status(prdContext("primary", "kbdiag_ro", "remote", 0, 0, 0), facts.InstInfo{Status: facts.StatusOK}, d, facts.InstDownstreams{Status: facts.StatusOK}, rule.Defaults)
	if rep.Verdict != rule.VerdictOK || len(rep.Redacted) != 1 || rep.Redacted[0].Field != "size_bytes" {
		t.Errorf("verdict=%s redacted=%+v", rep.Verdict, rep.Redacted)
	}
}

func TestSlotsMatchesPRD(t *testing.T) {
	l := facts.SlotList{Status: facts.StatusOK, Rows: []facts.Slot{{
		Name: "repmgr_slot_2", Type: "physical", Xmin: xid(5859), XminAge: i32(14),
		RestartLSN: str("0/A4013160"), RetainedWALBytes: i64(50331648),
	}}}
	assertPRD(t, "slots", Slots(prdContext("primary", "system", "local", 21, 52, 40), l))
}
