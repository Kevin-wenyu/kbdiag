package scenario

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// statusFacts is what node1 (primary) and node2 (standby) reported on
// 2026-09-26 18:28 (chronicle/2026-09-26.md, "status 打磨").
func statusFacts(role string) (facts.Context, facts.InstInfo, facts.InstDatabases, facts.InstDownstreams, facts.InstUpstream, facts.InstDisk) {
	c := facts.Context{Version: "KingbaseES V008R006C009B0014", Role: role, Location: "local", User: "system",
		CollectedAt: time.Date(2026, 9, 26, 18, 28, 9, 0, cst)}
	info := facts.Info{Version: "V008R006C009B0014", StartTime: time.Date(2026, 9, 20, 22, 19, 13, 0, cst), UptimeS: 504535,
		Connections: 6, MaxConnections: 100, SuperuserReserved: 3, DataDirectory: str("/home/kingbase/cluster/install/kingbase/data"), Port: 54321}
	d := facts.InstDatabases{Status: facts.StatusOK, Rows: []facts.Database{
		{Datname: "esrep", SizeBytes: i64(15614003)}, {Datname: "kingbase", SizeBytes: i64(15024179)}, {Datname: "mydb", SizeBytes: i64(14877187)},
		{Datname: "security", SizeBytes: i64(14844419)}, {Datname: "test", SizeBytes: i64(340459571)},
	}}
	n := facts.InstDownstreams{Status: facts.StatusOK, Rows: []facts.Downstream{
		{ApplicationName: str("node2"), ClientAddr: str("192.168.105.11"), State: str("streaming"), SyncState: str("quorum")},
	}}
	u := facts.InstUpstream{Status: facts.StatusNotApplicable, Reason: "primary"}
	disk := facts.Disk{TotalBytes: 213452304384, UsedBytes: 15089946624, AvailBytes: 198362357760}
	if role == "standby" {
		c.CollectedAt = c.CollectedAt.Add(time.Second)
		info.StartTime, info.UptimeS, info.Connections = time.Date(2026, 9, 23, 15, 44, 58, 0, cst), 268991, 3
		n.Rows = nil
		u = facts.InstUpstream{Status: facts.StatusOK, Rows: []facts.Upstream{{Status: str("streaming"), SenderHost: str("192.168.105.10"),
			SenderPort: i32(54321), SlotName: str("repmgr_slot_2"), LastMsgAgeS: f64(8.0)}}}
		disk = facts.Disk{TotalBytes: 213452304384, UsedBytes: 12501807104, AvailBytes: 200950497280}
	}
	return c, facts.InstInfo{Status: facts.StatusOK, Rows: []facts.Info{info}}, d, n, u, facts.InstDisk{Status: facts.StatusOK, Rows: []facts.Disk{disk}}
}

func TestStatusMatchesPRD(t *testing.T) {
	assertPRD(t, "status", Status(statusFacts("primary")))
}

func TestStatusStandbyMatchesPRD(t *testing.T) {
	assertPRD(t, "status（备库）", Status(statusFacts("standby")))
}

func assertGolden(t *testing.T, name string, rep *report.Report) {
	t.Helper()
	var buf bytes.Buffer
	if err := rep.WriteText(&buf); err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", name+".txt.golden")
	if *update {
		if err := os.WriteFile(golden, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != string(want) {
		t.Errorf("text output differs from %s (run go test -update after review)\n got:\n%s\nwant:\n%s", golden, buf.String(), want)
	}
}

// The first three goldens are the drafts the user confirmed on 2026-09-26.
func TestStatusText(t *testing.T) {
	t.Run("primary", func(t *testing.T) { assertGolden(t, "status_primary", Status(statusFacts("primary"))) })
	t.Run("standby", func(t *testing.T) { assertGolden(t, "status_standby", Status(statusFacts("standby"))) })
	t.Run("standby without walreceiver, remote", func(t *testing.T) {
		c, i, d, n, _, _ := statusFacts("standby")
		c.Location = "remote"
		rep := Status(c, i, d, n, facts.InstUpstream{Status: facts.StatusOK}, facts.InstDisk{Status: facts.StatusNotApplicable, Reason: "remote connection"})
		if rep.Verdict != rule.VerdictWARN {
			t.Errorf("verdict = %s", rep.Verdict)
		}
		assertGolden(t, "status_standby_no_walreceiver", rep)
	})
	// kbdiag_ro without sys_monitor, every usable slot taken: masked cells,
	// a hidden size, nothing collected for the disk.
	t.Run("masked and full", func(t *testing.T) {
		c, i, d, _, _, _ := statusFacts("primary")
		c.User = "kbdiag_ro"
		i.Rows[0].Connections, i.Rows[0].DataDirectory = 99, nil
		d.Rows[0].SizeBytes = nil
		n := facts.InstDownstreams{Status: facts.StatusOK, Rows: []facts.Downstream{{ApplicationName: str("node2")}}}
		rep := Status(c, i, d, n, facts.InstUpstream{Status: facts.StatusNotApplicable, Reason: "primary"},
			facts.InstDisk{Status: facts.StatusSkipped, Reason: "insufficient_privilege: 看不到 data_directory"})
		if rep.Verdict != rule.VerdictFAIL {
			t.Errorf("verdict = %s", rep.Verdict)
		}
		assertGolden(t, "status_masked_full", rep)
	})
	t.Run("standby, status hidden, info error", func(t *testing.T) {
		c, _, d, n, _, _ := statusFacts("standby")
		u := facts.InstUpstream{Status: facts.StatusOK, Rows: []facts.Upstream{{}}}
		rep := Status(c, facts.InstInfo{Status: facts.StatusError, Reason: "42P01: relation does not exist"}, d, n, u,
			facts.InstDisk{Status: facts.StatusSkipped, Reason: "inst.info 没采到，不知道 data_directory"})
		if rep.Verdict != rule.VerdictUNKNOWN {
			t.Errorf("verdict = %s", rep.Verdict)
		}
		assertGolden(t, "status_unknown", rep)
	})
}

func TestStatusRedacted(t *testing.T) {
	c, i, _, _, u, disk := statusFacts("primary")
	d := facts.InstDatabases{Status: facts.StatusOK, Rows: []facts.Database{{Datname: "secret"}}}
	n := facts.InstDownstreams{Status: facts.StatusOK, Rows: []facts.Downstream{{ApplicationName: str("node2")}}}
	rep := Status(c, i, d, n, u, disk)
	var fields []string
	for _, x := range rep.Redacted {
		fields = append(fields, x.ProbeID+"."+x.Field)
	}
	want := []string{"inst.databases.size_bytes", "inst.downstreams.state", "inst.downstreams.sync_state"}
	if rep.Verdict != rule.VerdictOK || !reflect.DeepEqual(fields, want) {
		t.Errorf("verdict=%s redacted=%v, want OK %v", rep.Verdict, fields, want)
	}
	c.Role = "standby"
	rep = Status(c, i, facts.InstDatabases{Status: facts.StatusOK}, facts.InstDownstreams{Status: facts.StatusOK},
		facts.InstUpstream{Status: facts.StatusOK, Rows: []facts.Upstream{{}}}, disk)
	if rep.Verdict != rule.VerdictUNKNOWN || len(rep.Redacted) != 1 || rep.Redacted[0].ProbeID+"."+rep.Redacted[0].Field != "inst.upstream.status" {
		t.Errorf("standby: verdict=%s redacted=%+v", rep.Verdict, rep.Redacted)
	}
}

// inst.disk is shown, never judged: however it ends, the verdict stays OK.
func TestStatusDiskNotJudged(t *testing.T) {
	c, i, d, n, u, _ := statusFacts("primary")
	for _, st := range []facts.Status{facts.StatusError, facts.StatusSkipped, facts.StatusNotApplicable} {
		if rep := Status(c, i, d, n, u, facts.InstDisk{Status: st, Reason: "x"}); rep.Verdict != rule.VerdictOK {
			t.Errorf("disk %s: verdict = %s", st, rep.Verdict)
		}
	}
}

func TestSlotsMatchesPRD(t *testing.T) {
	l := facts.SlotList{Status: facts.StatusOK, Rows: []facts.Slot{{
		Name: "repmgr_slot_2", Type: "physical", Xmin: xid(5859), XminAge: i32(14),
		RestartLSN: str("0/A4013160"), RetainedWALBytes: i64(50331648),
	}}}
	assertPRD(t, "slots", Slots(prdContext("primary", "system", "local", 21, 52, 40), l))
}

func locksCapture(t *testing.T, name string) (facts.Context, facts.LockList) {
	t.Helper()
	c := loadCapture(t, name)
	return c.context(t), c.lockList(t)
}

// The first five goldens were drawn by hand from the stage-0 captures
// before the code existed. The captures hold lock.list as shown, not every
// sys_locks row: the 2PC's own lock rows (pid NULL) are missing, so
// locks_prepared's "holds -" is provisional until the VM run.
func TestLocksText(t *testing.T) {
	lowthr := LocksOptions{Limit: 50, Thresholds: rule.Thresholds{LockWaitWarnS: 1}}
	for _, x := range []struct{ golden, capture string }{
		{"locks_primary", "locks_node1_lock_lowthr"},
		{"locks_primary_clean", "locks_node1_clean"},
		{"locks_standby", "locks_node2_lock_lowthr"},
		{"locks_ro", "locks_node1_lock_ro"},
		{"locks_prepared", "locks_node1_prepared_waiter"},
	} {
		t.Run(x.golden, func(t *testing.T) {
			c, l := locksCapture(t, x.capture)
			assertGolden(t, x.golden, Locks(c, l, lowthr))
		})
	}
	t.Run("default threshold", func(t *testing.T) {
		c, l := locksCapture(t, "locks_node1_lock")
		rep := Locks(c, l, LocksOptions{Limit: 50, Thresholds: rule.Defaults})
		if rep.Verdict != rule.VerdictOK || len(rep.Findings) != 0 {
			t.Errorf("a 3.5s wait at the 10s default: verdict=%s findings=%d", rep.Verdict, len(rep.Findings))
		}
		assertGolden(t, "locks_below_threshold", rep)
	})
	t.Run("probe error", func(t *testing.T) {
		c, _ := locksCapture(t, "locks_node1_clean")
		assertGolden(t, "locks_skipped", Locks(c, facts.LockList{Status: facts.StatusError, Reason: "57014: canceling statement due to statement timeout"}, lowthr))
	})
}

// A pile-up: one blocker, a waiter queued behind it that also blocks, a
// prepared transaction, a masked waiter, and --limit trimming the list.
func TestLocksPileUp(t *testing.T) {
	c, _ := locksCapture(t, "locks_node1_clean")
	rel, other := str("public.orders"), str("public.长表名")
	l := facts.LockList{Status: facts.StatusOK, Rows: []facts.Lock{
		{PID: i32(100), Locktype: "relation", Relation: rel, Mode: "AccessExclusiveLock", Granted: true},
		{PID: i32(100), Locktype: "virtualxid", Mode: "ExclusiveLock", Granted: true},
		{PID: i32(101), Locktype: "relation", Relation: rel, Mode: "RowExclusiveLock", WaitS: f64(120.4), BlockedBy: []int32{100}},
		{PID: i32(102), Locktype: "relation", Relation: rel, Mode: "AccessShareLock", WaitS: f64(30), BlockedBy: []int32{100, 101, 100}}, // parallel worker
		{PID: i32(103), Locktype: "relation", Relation: rel, Mode: "AccessShareLock", Masked: true, BlockedBy: []int32{100, 101}},
		{Locktype: "relation", Relation: other, Mode: "ShareLock", Granted: true},
		{PID: i32(104), Locktype: "relation", Relation: other, Mode: "ExclusiveLock", WaitS: f64(-1), BlockedBy: []int32{0, 0}}, // two 2PCs
		{PID: i32(105), Locktype: "transactionid", Mode: "ShareLock", WaitS: f64(5), BlockedBy: nil},
	}}
	rep := Locks(c, l, LocksOptions{Limit: 3, Thresholds: rule.Defaults})
	if rep.Verdict != rule.VerdictWARN || len(rep.Findings) != 2 {
		t.Errorf("verdict=%s findings=%d, want WARN with 101 and 102", rep.Verdict, len(rep.Findings))
	}
	assertGolden(t, "locks_pileup", rep)
	assertGolden(t, "locks_pileup_all", Locks(c, l, LocksOptions{Limit: 0, Thresholds: rule.Defaults}))
}

// The first seven goldens were drawn by hand from the stage-0 captures
// before the code existed.
func TestSessionText(t *testing.T) {
	low := rule.Thresholds{LockWaitWarnS: 1, IdleInTxnWarnS: 1}
	for _, x := range []struct {
		golden, capture string
		pid             int32
		th              rule.Thresholds
	}{
		{"session_waiter", "session_node1_lock_waiter_lowthr", 803890, low},
		{"session_holder", "session_node1_lock_holder", 803881, rule.Defaults},
		{"session_idletxn", "session_node1_idletxn_lowthr", 801150, low},
		{"session_ro", "session_node1_lock_waiter_ro", 803890, rule.Defaults},
		{"session_standby", "session_node2_lock_waiter", 422855, rule.Defaults},
		{"session_notfound", "session_node1_nosuchpid", 999999, rule.Defaults},
		{"session_prepared_waiter", "session_node1_prepared_waiter", 807225, rule.Defaults},
		{"session_untracked", "session_node1_untracked", 809073, rule.Defaults},
	} {
		t.Run(x.golden, func(t *testing.T) {
			c := loadCapture(t, x.capture)
			rep, _ := Session(c.context(t), c.sessionActivity(t), c.lockList(t), SessionOptions{PID: x.pid, Thresholds: x.th})
			assertGolden(t, x.golden, rep)
		})
	}
}

// A finding about this very session must not send the reader to
// "kbdiag session <this pid>": they are already looking at it.
func TestSessionNextIsNotItself(t *testing.T) {
	for _, x := range []struct {
		capture string
		pid     int32
	}{{"session_node1_lock_holder", 803881}, {"session_node1_idletxn_lowthr", 801150}} {
		c := loadCapture(t, x.capture)
		rep, _ := Session(c.context(t), c.sessionActivity(t), c.lockList(t),
			SessionOptions{PID: x.pid, Thresholds: rule.Thresholds{LockWaitWarnS: 1, IdleInTxnWarnS: 1}})
		if len(rep.Findings) == 0 {
			t.Fatalf("%s: no findings", x.capture)
		}
		for _, f := range rep.Findings {
			for _, n := range f.Next {
				if n.Command == fmt.Sprintf("kbdiag session %d", x.pid) {
					t.Errorf("%s: finding %s points at itself", x.capture, f.ID)
				}
			}
		}
	}
}

// Edge cases: multi-line SQL with tabs and control characters, a long wide
// application name, NULL everything, a skipped activity probe.
func TestSessionTextEdges(t *testing.T) {
	c, _ := locksCapture(t, "locks_node1_clean")
	s := facts.Session{PID: 7, Usename: str("用户"), ApplicationName: str(strings.Repeat("报表", 30)), BackendType: str("client backend"),
		State: str("active"), XactAgeS: f64(90061), QueryAgeS: f64(0), StateAgeS: f64(0), WaitEventType: str("IO"), WaitEvent: str("DataFileRead"),
		Query: str("select *\r\n\tfrom t\n where x = '\x1b[2J'\n")}
	a := facts.SessionActivity{Status: facts.StatusOK, Rows: []facts.Session{s}}
	l := facts.LockList{Status: facts.StatusOK, Rows: []facts.Lock{
		{PID: i32(7), Locktype: "relation", Relation: str("public.t"), Mode: "AccessShareLock", Granted: true},
		{PID: i32(7), Locktype: "transactionid", Mode: "ExclusiveLock", Granted: true},
		{PID: i32(8), Locktype: "transactionid", Mode: "ShareLock", WaitS: f64(3), BlockedBy: []int32{7}},
		{PID: i32(9), Locktype: "relation", Relation: str("public.t"), Mode: "AccessExclusiveLock", Masked: true, BlockedBy: []int32{7}},
	}}
	rep, _ := Session(c, a, l, SessionOptions{PID: 7, Thresholds: rule.Defaults})
	assertGolden(t, "session_edges", rep)

	rep, found := Session(c, facts.SessionActivity{Status: facts.StatusSkipped, Reason: "track_activities=off"}, l, SessionOptions{PID: 7, Thresholds: rule.Defaults})
	if !found {
		t.Error("a skipped probe must not report the pid as missing")
	}
	assertGolden(t, "session_skipped", rep)

	// Row locks, subtransactions, a waiter repeated by parallel workers, and a
	// contested virtualxid (CREATE INDEX CONCURRENTLY waits on those).
	rows := facts.LockList{Status: facts.StatusOK, Rows: []facts.Lock{
		{PID: i32(7), Locktype: "tuple", Relation: str("public.t"), Mode: "ExclusiveLock", Granted: true},
		{PID: i32(7), Locktype: "tuple", Relation: str("public.t"), Mode: "ExclusiveLock", Granted: true},
		{PID: i32(7), Locktype: "transactionid", Mode: "ExclusiveLock", Granted: true},
		{PID: i32(7), Locktype: "transactionid", Mode: "ExclusiveLock", Granted: true},
		{PID: i32(7), Locktype: "virtualxid", Mode: "ExclusiveLock", Granted: true},
		{PID: i32(8), Locktype: "tuple", Relation: str("public.t"), Mode: "ExclusiveLock", WaitS: f64(4), BlockedBy: []int32{7, 7}},
		{PID: i32(10), Locktype: "transactionid", Mode: "ShareLock", WaitS: f64(2), BlockedBy: []int32{7}},
		{PID: i32(11), Locktype: "virtualxid", Mode: "ShareLock", WaitS: f64(1), BlockedBy: []int32{7}},
	}}
	rep, _ = Session(c, a, rows, SessionOptions{PID: 7, Thresholds: rule.Defaults})
	assertGolden(t, "session_rowlocks", rep)

	bare := facts.SessionActivity{Status: facts.StatusOK, Rows: []facts.Session{{PID: 7}}}
	rep, _ = Session(c, bare, facts.LockList{Status: facts.StatusOK}, SessionOptions{PID: 7, Thresholds: rule.Defaults})
	assertGolden(t, "session_bare", rep)
}
