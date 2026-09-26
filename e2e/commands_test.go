//go:build vm

package e2e

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	lockColumns     = []string{"pid", "locktype", "relation", "mode", "granted", "wait_s", "blocked_by"}
	preparedColumns = []string{"gid", "owner", "database", "prepared_at", "age_s", "transaction"}
	waitColumns     = []string{"wait_event_type", "wait_event", "state", "sessions", "pids"}
	slotColumns     = []string{"slot_name", "slot_type", "active", "active_pid", "xmin", "catalog_xmin", "xmin_age", "restart_lsn", "retained_wal_bytes"}
)

// rowsOf returns every row as a column → value map.
func (p probeData) rowsOf() []map[string]any {
	var out []map[string]any
	for _, r := range p.Rows {
		m := map[string]any{}
		for i, c := range p.Columns {
			m[c] = r[i]
		}
		out = append(out, m)
	}
	return out
}

// findings returns the findings with id whose evidence field key equals v.
func findings(r report, id, key string, v any) (levels []string) {
	for _, f := range r.Findings {
		if f.ID == id && len(f.Evidence) > 0 && reflect.DeepEqual(f.Evidence[0].Fields[key], v) {
			levels = append(levels, f.Level)
		}
	}
	return levels
}

// okProbe fails the test unless the probe was collected with the contract columns.
func okProbe(t *testing.T, r report, id string, cols []string) probeData {
	t.Helper()
	p, ok := r.Data[id]
	if !ok {
		t.Fatalf("no %s in data: %v", id, r.Data)
	}
	if p.Status != "ok" || p.Reason != nil || !reflect.DeepEqual(p.Columns, cols) {
		t.Fatalf("%s status=%s reason=%v columns=%v", id, p.Status, p.Reason, p.Columns)
	}
	return p
}

// textLine returns the first line of text output whose first field is key.
func textLine(out, key string) string {
	for _, l := range strings.Split(out, "\n") {
		if f := strings.Fields(l); len(f) > 0 && f[0] == key {
			return l
		}
	}
	return ""
}

// textRow reports whether some line starts with key and contains every want.
func textRow(out, key string, want ...string) bool {
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) == 0 || f[0] != key {
			continue
		}
		ok := true
		for _, w := range want {
			ok = ok && strings.Contains(l, w)
		}
		if ok {
			return true
		}
	}
	return false
}

// vmOut runs a command on the node as kingbase and returns its stdout.
func vmOut(t *testing.T, args ...string) string {
	t.Helper()
	out, err := vm(args...).Output()
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return string(out)
}

// lockWait is how long ksql says pid has been in its current statement.
func lockWait(t *testing.T, pid float64) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(ksql(t, fmt.Sprintf("select extract(epoch from now()-state_change)::float8 from sys_stat_activity where pid = %d", int(pid))), 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// lockInjection is the lock.sh state as ksql sees it.
type lockInjection struct {
	holder, waiter float64
	relation       any // "public.kbdiag_inj_lock" on a primary; nil for the standby's advisory lock
	locktype, mode string
}

func injectLock(t *testing.T) lockInjection {
	t.Helper()
	inject(t, "lock")
	l := lockInjection{holder: injected(t, "lock_holder").pid, waiter: injected(t, "lock_waiter").pid,
		relation: "public.kbdiag_inj_lock", locktype: "relation", mode: "AccessShareLock"}
	if role == "standby" {
		l.relation, l.locktype, l.mode = nil, "advisory", "ExclusiveLock"
	}
	time.Sleep(1500 * time.Millisecond) // a wait of at least 1s, so an always-0 wait_s fails
	return l
}

// L3 + L4: the waiter's row matches what ksql sees, and it is flagged once past
// the threshold with the holder as its blocker.
func TestLocks(t *testing.T) {
	l := injectLock(t)
	before := lockWait(t, l.waiter)
	r, code := kbdiag(t, nil, "locks", "--limit", "0", "--lock-wait-warn", "1")
	after := lockWait(t, l.waiter)
	p := okProbe(t, r, "lock.list", lockColumns)
	if r.Command != "locks" || len(r.Redacted) != 0 {
		t.Errorf("command=%s redacted=%+v", r.Command, r.Redacted)
	}
	var w, h map[string]any
	for _, row := range p.rowsOf() {
		if row["locktype"] != l.locktype || row["relation"] != l.relation {
			continue
		}
		switch row["pid"] {
		case l.waiter:
			w = row
		case l.holder:
			h = row
		}
	}
	if w == nil || h == nil {
		t.Fatalf("waiter or holder row missing: %v", p.Rows)
	}
	if w["granted"] != false || w["mode"] != l.mode || !reflect.DeepEqual(w["blocked_by"], []any{l.holder}) {
		t.Errorf("waiter row = %v", w)
	}
	if s, ok := w["wait_s"].(float64); !ok || before < 1 || s < before-0.1 || s > after+0.1 {
		t.Errorf("wait_s = %v, ksql says %.2f before and %.2f after", w["wait_s"], before, after)
	}
	if h["granted"] != true || h["wait_s"] != nil || !reflect.DeepEqual(h["blocked_by"], []any{}) {
		t.Errorf("holder row = %v", h)
	}
	for _, row := range p.rowsOf() {
		if row["granted"] == true && row["pid"] != l.holder {
			t.Errorf("granted row of a non-blocker shown: %v", row)
		}
	}
	if got := findings(r, "lock.waiting", "waiter_pid", l.waiter); !reflect.DeepEqual(got, []string{"WARN"}) {
		t.Fatalf("lock.waiting for waiter %v: %v", l.waiter, got)
	}
	if got := findings(r, "lock.waiting", "blocker_pids", []any{l.holder}); len(got) != 1 {
		t.Errorf("evidence blocker_pids does not name holder %v", l.holder)
	}
	if got := findings(r, "lock.waiting", "waiter_pid", l.holder); len(got) != 0 {
		t.Errorf("holder flagged as a waiter")
	}
	if r.Verdict != "WARN" || code != 1 {
		t.Errorf("verdict=%s exit=%d, want WARN/1", r.Verdict, code)
	}

	// The text leads with the holder as a blocker, then the waiter.
	t.Run("text", func(t *testing.T) {
		out, _ := kbdiagText(t, nil, "locks")
		holder, waiter := fmt.Sprint(l.holder), fmt.Sprint(l.waiter)
		object := "advisory"
		if l.relation != nil {
			object = fmt.Sprint(l.relation)
		}
		if f := strings.Fields(textLine(out, holder)); len(f) < 3 || f[1] != "1" || !strings.Contains(textLine(out, holder), object) ||
			!textRow(out, waiter, object, l.mode) ||
			!strings.Contains(out, "\nblockers: ") || !strings.Contains(out, "\nwaiting: ") {
			t.Errorf("text lacks blocker %s or waiter %s on %s:\n%s", holder, waiter, object, out)
		}
		if !strings.HasSuffix(strings.TrimSpace(textLine(out, waiter)), holder) {
			t.Errorf("waiter line does not end with its blocker %s:\n%s", holder, out)
		}
	})

	t.Run("below the threshold", func(t *testing.T) {
		r, code := kbdiag(t, nil, "locks", "--lock-wait-warn", "3600")
		if okProbe(t, r, "lock.list", lockColumns).row("pid", l.waiter) == nil {
			t.Fatal("waiter not shown: the negative would mean nothing")
		}
		if got := findings(r, "lock.waiting", "waiter_pid", l.waiter); len(got) != 0 || r.Verdict != "OK" || code != 0 {
			t.Errorf("findings=%v verdict=%s exit=%d, want none/OK/0", got, r.Verdict, code)
		}
	})
}

// L4: a waiter blocked by a prepared transaction has blocker 0, and the next
// step points at txn instead of a session.
func TestLocksBlockedByPrepared(t *testing.T) {
	if role == "standby" {
		t.Skip("prepared transactions are made on the primary")
	}
	inject(t, "prepared")
	inject(t, "prepared_waiter")
	waiter := injected(t, "prepared_waiter").pid
	time.Sleep(1500 * time.Millisecond)
	r, _ := kbdiag(t, nil, "locks", "--lock-wait-warn", "1")
	if got := findings(r, "lock.waiting", "waiter_pid", waiter); !reflect.DeepEqual(got, []string{"WARN"}) {
		t.Fatalf("lock.waiting for %v: %v", waiter, got)
	}
	if got := findings(r, "lock.waiting", "blocker_pids", []any{0.0}); len(got) != 1 {
		t.Errorf("blocker_pids is not [0]: %+v", r.Findings)
	}
	// The 2PC's own lock has no pid in sys_locks (seen on node1, 2026-09-26);
	// lock.list keeps it because it is what the waiter wants.
	held := false
	for _, row := range okProbe(t, r, "lock.list", lockColumns).rowsOf() {
		if row["pid"] == nil && row["granted"] == true && row["relation"] == "public.kbdiag_inj_2pc" {
			held = true
		}
	}
	if !held {
		t.Errorf("lock.list lacks the prepared transaction's lock on kbdiag_inj_2pc")
	}
	out, _ := kbdiagText(t, nil, "locks")
	if !textRow(out, "2PC", "1", "public.kbdiag_inj_2pc") || !strings.HasSuffix(strings.TrimSpace(textLine(out, fmt.Sprint(waiter))), "2PC") {
		t.Errorf("text does not name the prepared transaction as 2PC with the lock it holds:\n%s", out)
	}
}

// L4: session <pid> narrows both probes to one session.
func TestSession(t *testing.T) {
	l := injectLock(t)

	t.Run("waiter", func(t *testing.T) {
		r, code := kbdiag(t, nil, "session", strconv.Itoa(int(l.waiter)), "--lock-wait-warn", "1")
		a := okProbe(t, r, sessionProbe, sessionColumns)
		if len(a.Rows) != 1 || a.Rows[0][0] != l.waiter {
			t.Fatalf("session.activity rows = %v", a.Rows)
		}
		if row := a.row("pid", l.waiter); row["wait_event_type"] != "Lock" {
			t.Errorf("wait_event_type = %v", row["wait_event_type"])
		}
		for _, row := range okProbe(t, r, "lock.list", lockColumns).rowsOf() {
			if row["pid"] != l.waiter {
				t.Errorf("lock row of another session: %v", row)
			}
		}
		if got := findings(r, "lock.waiting", "waiter_pid", l.waiter); len(got) != 1 || r.Verdict != "WARN" || code != 1 {
			t.Errorf("findings=%v verdict=%s exit=%d", got, r.Verdict, code)
		}
		out, _ := kbdiagText(t, nil, "session", strconv.Itoa(int(l.waiter)), "--lock-wait-warn", "1")
		if !strings.Contains(out, fmt.Sprintf("\nsession %d\n", int(l.waiter))) || !strings.Contains(out, "\nwaiting for: 1\n") ||
			!strings.Contains(out, "\nblocking: 0\n") || !strings.Contains(out, fmt.Sprintf("%s  %d\n", "", int(l.holder))) {
			t.Errorf("waiter text lacks its header, its wait or its blocker %v:\n%s", l.holder, out)
		}
	})

	t.Run("holder shows whom it blocks", func(t *testing.T) {
		r, _ := kbdiag(t, nil, "session", strconv.Itoa(int(l.holder)), "--lock-wait-warn", "1")
		p := okProbe(t, r, "lock.list", lockColumns)
		var own, blocked int
		for _, row := range p.rowsOf() {
			switch {
			case row["pid"] == l.holder:
				own++
			case row["pid"] == l.waiter && reflect.DeepEqual(row["blocked_by"], []any{l.holder}):
				blocked++
			default:
				t.Errorf("unrelated lock row: %v", row)
			}
		}
		if own == 0 || blocked != 1 {
			t.Errorf("own=%d blocked=%d", own, blocked)
		}
		if got := findings(r, "lock.waiting", "waiter_pid", l.waiter); len(got) != 1 {
			t.Errorf("the blocked waiter is not flagged: %v", got)
		}
		self := fmt.Sprintf("kbdiag session %d", int(l.holder))
		for _, f := range r.Findings {
			for _, n := range f.Next {
				if n.Command == self {
					t.Errorf("finding %s sends the reader back to %s", f.ID, self)
				}
			}
		}
		out, _ := kbdiagText(t, nil, "session", strconv.Itoa(int(l.holder)), "--lock-wait-warn", "1")
		// "  <waiter>  " starts a table row; the finding line mentions the pid too
		if !strings.Contains(out, "\nblocking: 1\n") || !strings.Contains(out, "\n  "+fmt.Sprint(l.waiter)+"  ") {
			t.Errorf("holder text does not list waiter %v under blocking:\n%s", l.waiter, out)
		}
	})

	t.Run("no such pid", func(t *testing.T) {
		r, code := kbdiag(t, nil, "session", "2147483647")
		if r.Verdict != "UNKNOWN" || code != 3 || len(r.Data[sessionProbe].Rows) != 0 || len(r.Data["lock.list"].Rows) != 0 {
			t.Errorf("verdict=%s exit=%d data=%v", r.Verdict, code, r.Data)
		}
		if out, _ := kbdiagText(t, nil, "session", "2147483647"); !strings.Contains(out, "session 2147483647: not found") {
			t.Errorf("text does not say the pid is not found:\n%s", out)
		}
	})
}

// L3 + L4 for txn.prepared: the row matches ksql; flagged past the threshold.
func TestTxnPrepared(t *testing.T) {
	if role == "standby" {
		r, _ := kbdiag(t, nil, "txn")
		p := r.Data["txn.prepared"]
		if p.Status != "not_applicable" || p.Reason == nil || !strings.Contains(*p.Reason, "primary") || len(p.Rows) != 0 {
			t.Errorf("standby txn.prepared = %+v", p)
		}
		return
	}
	inject(t, "prepared")
	want := strings.Split(ksql(t, "select owner, database, transaction from sys_prepared_xacts where gid='kbdiag_inj_2pc'"), "|")
	time.Sleep(1500 * time.Millisecond)
	r, code := kbdiag(t, nil, "txn", "--prepared-warn", "1")
	row := okProbe(t, r, "txn.prepared", preparedColumns).row("gid", "kbdiag_inj_2pc")
	if row == nil {
		t.Fatal("injected gid not in txn.prepared")
	}
	if row["owner"] != want[0] || row["database"] != want[1] || fmt.Sprint(row["transaction"]) != want[2] {
		t.Errorf("row = %v, ksql says %v", row, want)
	}
	if age, _ := row["age_s"].(float64); age < 1 {
		t.Errorf("age_s = %v", row["age_s"])
	}
	if _, err := time.Parse(time.RFC3339, fmt.Sprint(row["prepared_at"])); err != nil {
		t.Errorf("prepared_at: %v", err)
	}
	if got := findings(r, "txn.prepared", "gid", "kbdiag_inj_2pc"); !reflect.DeepEqual(got, []string{"WARN"}) || r.Verdict != "WARN" || code != 1 {
		t.Errorf("findings=%v verdict=%s exit=%d", got, r.Verdict, code)
	}
	// The text names the 2PC in the prepared table and as the oldest xid. On
	// node1 (2026-09-26) nothing older was open: walsenders and the slot do not
	// show up there, and a repmgr query caught mid-flight has an xmin no older
	// than the 2PC's xid, so it can only join the holders.
	out, _ := kbdiagText(t, nil, "txn")
	oldest := textLine(out, "oldest")
	if !strings.Contains(out, "\nprepared: 1\n") || textLine(out, "kbdiag_inj_2pc") == "" ||
		!strings.HasPrefix(strings.TrimSpace(oldest), "oldest xid: "+want[2]+"  (") || !strings.Contains(oldest, "2PC kbdiag_inj_2pc") {
		t.Errorf("text lacks the prepared transaction as the oldest xid %s:\n%s", want[2], out)
	}

	r, _ = kbdiag(t, nil, "txn")
	if got := findings(r, "txn.prepared", "gid", "kbdiag_inj_2pc"); len(got) != 0 {
		t.Errorf("a seconds-old prepared transaction is flagged at the default: %v", got)
	}
}

// L4 for txn.long: WARN by the transaction's age, never FAIL.
func TestTxnLong(t *testing.T) {
	inject(t, "idle_txn")
	fp := idleTxn(t)
	time.Sleep(1500 * time.Millisecond)
	r, _ := kbdiag(t, nil, "txn", "--limit", "0", "--xact-warn", "1")
	row := okProbe(t, r, sessionProbe, sessionColumns).row("pid", fp.pid)
	if row == nil || row["backend_xid"] != fp.xid {
		t.Fatalf("injected row = %v", row)
	}
	if got := findings(r, "txn.long", "pid", fp.pid); !reflect.DeepEqual(got, []string{"WARN"}) || r.Verdict != "WARN" {
		t.Errorf("findings %v verdict=%s, want one WARN", got, r.Verdict)
	}
	// The text lists it among the open transactions with its xid; a standby
	// cannot assign one, so there the xid column shows "-".
	xid := "-"
	if fp.xid != nil {
		xid = fmt.Sprint(fp.xid)
	}
	out, _ := kbdiagText(t, nil, "txn", "--limit", "0")
	if !textRow(out, fmt.Sprint(fp.pid), "idle in transaction", xid) {
		t.Errorf("text does not list %v with xid %s:\n%s", fp.pid, xid, out)
	}
	r, _ = kbdiag(t, nil, "txn")
	if got := findings(r, "txn.long", "pid", fp.pid); len(got) != 0 {
		t.Errorf("a seconds-old transaction is flagged at the default: %v", got)
	}
}

// L3 + L4: the lock waiter shows up under its wait event.
func TestWaits(t *testing.T) {
	l := injectLock(t)
	r, code := kbdiag(t, nil, "waits")
	p := okProbe(t, r, "wait.summary", waitColumns)
	event := map[string]string{"relation": "relation", "advisory": "advisory"}[l.locktype]
	found := false
	for _, row := range p.rowsOf() {
		pids, _ := row["pids"].([]any)
		if row["sessions"] != float64(len(pids)) {
			t.Errorf("sessions %v != len(pids) %d", row["sessions"], len(pids))
		}
		for _, x := range pids {
			if x == l.waiter {
				found = true
				if row["wait_event_type"] != "Lock" || row["wait_event"] != event || row["state"] != "active" {
					t.Errorf("waiter's group = %v", row)
				}
			}
		}
	}
	if !found {
		t.Errorf("waiter %v in no group: %v", l.waiter, p.Rows)
	}
	if r.Verdict != "OK" || code != 0 || len(r.Findings) != 0 || len(r.Redacted) != 0 {
		t.Errorf("verdict=%s exit=%d findings=%d redacted=%v", r.Verdict, code, len(r.Findings), r.Redacted)
	}
	// The text shows the waiter's group; the idle holder and the background
	// processes are only counted (Activity waits are processes idling).
	out, _ := kbdiagText(t, nil, "waits")
	if !textRow(out, "Lock:"+event, "active", fmt.Sprint(l.waiter)) || !strings.Contains(out, "\nnot shown: ") {
		t.Errorf("waits text:\n%s", out)
	}
	// Activity waits (KES's Activity:KshMain included, whatever its state)
	// are processes idling: no table row names them. A second run may differ
	// by a group, so only pids seen in the JSON run are checked.
	for _, row := range p.rowsOf() {
		if row["wait_event_type"] != "Activity" {
			continue
		}
		pids, _ := row["pids"].([]any)
		for _, x := range pids {
			for _, line := range strings.Split(out, "\n") {
				if slices.Contains(strings.Fields(line), fmt.Sprint(x)) {
					t.Errorf("Activity:%v pid %v listed in text: %q", row["wait_event"], x, line)
				}
			}
		}
	}
	// A background process running with no wait event (the ksh writer on
	// its metric query) is not a busy session: no "(running)" row names one.
	bg := strings.Fields(ksql(t, "select string_agg(pid::text, ' ') from sys_stat_activity where backend_type not in ('client backend', 'parallel worker')"))
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) > 0 && f[0] == "(running)" {
			for _, x := range bg {
				if slices.Contains(f[3:], x) {
					t.Errorf("background pid %s listed as running: %q", x, line)
				}
			}
		}
	}
}

// L4: with track_activities off, waits is skipped and UNKNOWN.
func TestWaitsTrackActivitiesOff(t *testing.T) {
	inject(t, "track_off")
	r, code := kbdiag(t, nil, "waits")
	p := r.Data["wait.summary"]
	if p.Status != "skipped" || p.Reason == nil || !strings.Contains(*p.Reason, "track_activities=off") || r.Verdict != "UNKNOWN" || code != 3 {
		t.Errorf("status=%s reason=%v verdict=%s exit=%d", p.Status, p.Reason, r.Verdict, code)
	}
}

// L3: status agrees with ksql and df.
func TestStatus(t *testing.T) {
	want := strings.Split(ksql(t, `select floor(extract(epoch from sys_postmaster_start_time()))::bigint,
current_setting('max_connections'), current_setting('superuser_reserved_connections'),
current_setting('data_directory'), current_setting('port'), split_part(version(), ' ', 2),
(select string_agg(datname::text, ',' order by datname) from sys_database where not datistemplate)`), "|")
	r, code := kbdiag(t, nil, "status")
	if r.Verdict != "OK" || code != 0 || len(r.Findings) != 0 {
		t.Errorf("verdict=%s exit=%d findings=%d", r.Verdict, code, len(r.Findings))
	}
	info := okProbe(t, r, "inst.info", []string{"version", "start_time", "uptime_s", "connections", "max_connections", "superuser_reserved_connections", "data_directory", "port", "usable_connections"})
	if len(info.Rows) != 1 {
		t.Fatalf("inst.info rows = %v", info.Rows)
	}
	row := info.rowsOf()[0]
	start, err := time.Parse(time.RFC3339, fmt.Sprint(row["start_time"]))
	if err != nil || fmt.Sprint(start.Unix()) != want[0] {
		t.Errorf("start_time = %v, ksql epoch %s", row["start_time"], want[0])
	}
	if up, _ := row["uptime_s"].(float64); up < 1 || up-time.Since(start).Seconds() > 5 || time.Since(start).Seconds()-up > 5 {
		t.Errorf("uptime_s = %v, start %v", row["uptime_s"], start)
	}
	if fmt.Sprint(row["max_connections"]) != want[1] || fmt.Sprint(row["superuser_reserved_connections"]) != want[2] ||
		row["data_directory"] != want[3] || fmt.Sprint(row["port"]) != want[4] || row["version"] != want[5] {
		t.Errorf("row = %v, ksql says %v", row, want)
	}
	if u, _ := row["usable_connections"].(float64); u != row["max_connections"].(float64)-row["superuser_reserved_connections"].(float64) {
		t.Errorf("usable_connections = %v", row["usable_connections"])
	}
	if c, _ := row["connections"].(float64); c < 1 || c > 100 {
		t.Errorf("connections = %v", row["connections"])
	}
	if v, _ := row["version"].(string); !strings.HasPrefix(v, "V") || strings.Contains(v, " ") {
		t.Errorf("version = %v, want the short version number", row["version"])
	}
	var names []string
	for _, d := range okProbe(t, r, "inst.databases", []string{"datname", "size_bytes"}).rowsOf() {
		names = append(names, d["datname"].(string))
		if s, _ := d["size_bytes"].(float64); s <= 0 {
			t.Errorf("size of %v = %v", d["datname"], d["size_bytes"])
		}
	}
	if strings.Join(names, ",") != want[6] {
		t.Errorf("databases = %v, ksql says %s", names, want[6])
	}

	// inst.downstreams: one row per walsender, sync_state raw (quorum in the lab).
	wantDown := ksql(t, `select coalesce(string_agg(application_name || ':' || state || ':' || sync_state, ',' order by application_name, pid), '-')
from sys_stat_replication`)
	var down []string
	for _, d := range okProbe(t, r, "inst.downstreams", []string{"application_name", "client_addr", "state", "sync_state"}).rowsOf() {
		down = append(down, fmt.Sprintf("%v:%v:%v", d["application_name"], d["state"], d["sync_state"]))
	}
	if got := strings.Join(down, ","); (got == "" && wantDown != "-") || (got != "" && got != wantDown) {
		t.Errorf("downstreams = %s, ksql says %s", got, wantDown)
	}
	if (role == "primary") != (len(down) == 1) {
		t.Errorf("%s with %d downstreams: the lab has one standby", role, len(down))
	}

	// inst.upstream: not_applicable on the primary; on the standby it agrees
	// with sys_stat_wal_receiver.
	up := r.Data["inst.upstream"]
	if role == "primary" {
		if up.Status != "not_applicable" || up.Reason == nil || *up.Reason != "primary" || len(up.Rows) != 0 {
			t.Errorf("inst.upstream on the primary = %+v", up)
		}
	} else {
		wantUp := ksql(t, "select status || '|' || sender_host || '|' || sender_port || '|' || slot_name from sys_stat_wal_receiver")
		u := okProbe(t, r, "inst.upstream", []string{"status", "sender_host", "sender_port", "slot_name", "last_msg_age_s"}).rowsOf()
		if len(u) != 1 {
			t.Fatalf("inst.upstream rows = %v", u)
		}
		if got := fmt.Sprintf("%v|%v|%v|%v", u[0]["status"], u[0]["sender_host"], u[0]["sender_port"], u[0]["slot_name"]); got != wantUp {
			t.Errorf("upstream = %s, ksql says %s", got, wantUp)
		}
		if age, ok := u[0]["last_msg_age_s"].(float64); !ok || age < 0 || age > 60 {
			t.Errorf("last_msg_age_s = %v", u[0]["last_msg_age_s"])
		}
	}

	// inst.disk agrees with df on the data directory; used and avail move
	// between the two reads, so they only have to be close.
	df := strings.Fields(strings.TrimSpace(vmOut(t, "df", "-B1", "--output=size,used,avail", want[3])))
	disk := okProbe(t, r, "inst.disk", []string{"total_bytes", "used_bytes", "avail_bytes"}).rowsOf()
	if len(df) != 6 || len(disk) != 1 {
		t.Fatalf("df = %q, inst.disk = %v", df, disk)
	}
	for i, col := range []string{"total_bytes", "used_bytes", "avail_bytes"} {
		d, _ := strconv.ParseFloat(df[3+i], 64)
		if got, _ := disk[0][col].(float64); math.Abs(got-d) > 64<<20 {
			t.Errorf("%s = %.0f, df says %.0f", col, got, d)
		}
	}

	// Over TCP to the node's own address kbdiag cannot tell it is on the
	// database host, so the disk is not read.
	t.Run("disk not applicable over TCP", func(t *testing.T) {
		ip := map[string]string{"kes-node1": "192.168.105.10", "kes-node2": "192.168.105.11"}[node]
		if ip == "" {
			t.Skipf("no internal IP known for %s", node)
		}
		r, _ := kbdiag(t, []string{"PGPASSWORD=kbdiag_ro_T3st"}, "status", "--host", ip, "-U", "kbdiag_ro")
		if p := r.Data["inst.disk"]; p.Status != "not_applicable" || p.Reason == nil || *p.Reason != "remote connection" {
			t.Errorf("inst.disk = %+v", p)
		}
	})

	// DS-03: every connection ordinary users may open is taken; kbdiag still gets
	// in through the superuser reserve and says FAIL.
	t.Run("usable connections used up", func(t *testing.T) {
		inject(t, "conn")
		if n := ksql(t, "select count(*) from sys_stat_activity where application_name = 'kbdiag_inj_conn'"); n == "0" {
			t.Fatal("conn.sh opened no connections of its own; the lab was already full")
		}
		r, code := kbdiag(t, nil, "status")
		if got := findings(r, "inst.connections", "max_connections", row["max_connections"]); !reflect.DeepEqual(got, []string{"FAIL"}) || r.Verdict != "FAIL" || code != 2 {
			t.Fatalf("findings=%v verdict=%s exit=%d", got, r.Verdict, code)
		}
		ev := r.Findings[0].Evidence[0].Fields
		usable := ev["max_connections"].(float64) - ev["superuser_reserved_connections"].(float64)
		if c, _ := ev["connections"].(float64); c < usable {
			t.Errorf("connections = %v, usable %v", c, usable)
		}
	})
}

// L3 + L4: slots on the primary (the standby's slot), on the standby (none,
// or an injected one measured from the replay LSN).
func TestSlots(t *testing.T) {
	r, code := kbdiag(t, nil, "slots")
	p := okProbe(t, r, "slot.list", slotColumns)
	want := ksql(t, "select string_agg(slot_name::text || ':' || active, ',' order by slot_name) from sys_replication_slots")
	var got []string
	for _, row := range p.rowsOf() {
		got = append(got, fmt.Sprintf("%v:%v", row["slot_name"], row["active"]))
		if b, ok := row["retained_wal_bytes"].(float64); row["restart_lsn"] != nil && (!ok || b < 0) {
			t.Errorf("retained_wal_bytes = %v", row["retained_wal_bytes"])
		}
	}
	if strings.Join(got, ",") != want {
		t.Errorf("slots = %v, ksql says %q", got, want)
	}
	if want == "" || !strings.Contains(want, ":false") {
		if r.Verdict != "OK" || code != 0 {
			t.Errorf("no inactive slot, verdict=%s exit=%d", r.Verdict, code)
		}
	}

	if role == "standby" {
		t.Run("inactive slot on a standby", func(t *testing.T) {
			inject(t, "standby_slot")
			r, code := kbdiag(t, nil, "slots")
			row := okProbe(t, r, "slot.list", slotColumns).row("slot_name", "kbdiag_inj_slot")
			if row == nil || row["active"] != false || row["restart_lsn"] == nil {
				t.Fatalf("injected slot row = %v", row)
			}
			if b, ok := row["retained_wal_bytes"].(float64); !ok || b < 0 {
				t.Errorf("retained_wal_bytes = %v: must come from the replay LSN", row["retained_wal_bytes"])
			}
			if got := findings(r, "slot.inactive", "slot_name", "kbdiag_inj_slot"); !reflect.DeepEqual(got, []string{"WARN"}) || code != 1 {
				t.Errorf("findings=%v exit=%d", got, code)
			}
			out, _ := kbdiagText(t, nil, "slots")
			if !textRow(out, "kbdiag_inj_slot", "physical", "  no  ") {
				t.Errorf("text does not list the inactive slot:\n%s", out)
			}
		})
		return
	}
	t.Run("standby walreceiver paused", func(t *testing.T) {
		inject(t, "slot")
		r, code := kbdiag(t, nil, "slots")
		var name any
		for _, row := range okProbe(t, r, "slot.list", slotColumns).rowsOf() {
			if row["slot_type"] == "physical" && row["active"] == false && row["xmin"] != nil {
				name = row["slot_name"]
			}
		}
		if name == nil {
			t.Fatalf("no inactive physical slot with xmin: %+v", r.Data["slot.list"])
		}
		if got := findings(r, "slot.inactive", "slot_name", name); !reflect.DeepEqual(got, []string{"WARN"}) || r.Verdict != "WARN" || code != 1 {
			t.Errorf("findings=%v verdict=%s exit=%d", got, r.Verdict, code)
		}
		// inactive slots come first; this one also shows its xmin
		out, _ := kbdiagText(t, nil, "slots")
		if !textRow(out, fmt.Sprint(name), "  no  ") || !strings.Contains(out, "\nslots: ") {
			t.Errorf("text does not list %v as inactive:\n%s", name, out)
		}
	})
}

// L5 (one cell): a non-monitor user over TCP.
func TestNonMonitorUser(t *testing.T) {
	env := []string{"PGPASSWORD=kbdiag_ro_T3st"}
	ro := []string{"--host", "127.0.0.1", "-U", "kbdiag_ro"}

	t.Run("locks: a waiter's wait is hidden", func(t *testing.T) {
		l := injectLock(t)
		r, code := kbdiag(t, env, append([]string{"locks", "--lock-wait-warn", "1"}, ro...)...)
		w := okProbe(t, r, "lock.list", lockColumns).row("pid", l.waiter)
		if w == nil || w["wait_s"] != nil || !reflect.DeepEqual(w["blocked_by"], []any{l.holder}) {
			t.Fatalf("waiter row = %v", w)
		}
		if len(r.Redacted) != 1 || r.Redacted[0].Field != "wait_s" || r.Redacted[0].Reason != "insufficient_privilege" || r.Redacted[0].RowsAffected < 1 {
			t.Errorf("redacted = %+v", r.Redacted)
		}
		if r.Verdict != "UNKNOWN" || code != 3 {
			t.Errorf("verdict=%s exit=%d, want UNKNOWN/3", r.Verdict, code)
		}
	})

	t.Run("waits: masked sessions", func(t *testing.T) {
		r, code := kbdiag(t, env, append([]string{"waits"}, ro...)...)
		p := okProbe(t, r, "wait.summary", waitColumns)
		hidden := 0.0
		for _, row := range p.rowsOf() {
			if row["wait_event_type"] == nil && row["wait_event"] == nil && row["state"] == nil {
				hidden += row["sessions"].(float64)
			}
		}
		var fields []string
		for _, d := range r.Redacted {
			fields = append(fields, d.Field)
			if d.Reason != "insufficient_privilege" || float64(d.RowsAffected) != hidden {
				t.Errorf("redacted %+v, want %v sessions", d, hidden)
			}
		}
		if !reflect.DeepEqual(fields, []string{"wait_event_type", "wait_event", "state"}) || r.Verdict != "UNKNOWN" || code != 3 {
			t.Errorf("fields=%v verdict=%s exit=%d", fields, r.Verdict, code)
		}
	})

	// L5 cell 5: granting sys_monitor lifts every redaction.
	t.Run("with sys_monitor nothing is redacted", func(t *testing.T) {
		if role == "standby" {
			t.Skip("GRANT is not possible on a standby; the grant made on the primary replicates, so one node covers it")
		}
		l := injectLock(t)
		ksql(t, "grant sys_monitor to kbdiag_ro")
		t.Cleanup(func() { ksql(t, "revoke sys_monitor from kbdiag_ro") })
		for _, cmd := range []string{"sessions", "session", "locks", "txn", "waits"} {
			args := []string{cmd}
			if cmd == "session" {
				args = append(args, strconv.Itoa(int(l.waiter)))
			}
			r, code := kbdiag(t, env, append(args, ro...)...)
			if len(r.Redacted) != 0 || code == 3 {
				t.Errorf("%s: redacted=%+v verdict=%s exit=%d", cmd, r.Redacted, r.Verdict, code)
			}
		}
		r, _ := kbdiag(t, env, append([]string{"locks", "--lock-wait-warn", "1"}, ro...)...)
		if w := r.Data["lock.list"].row("pid", l.waiter); w == nil || w["wait_s"] == nil {
			t.Errorf("waiter row = %v", w)
		}
	})

	t.Run("slots still answers", func(t *testing.T) {
		r, code := kbdiag(t, env, append([]string{"slots"}, ro...)...)
		for id, p := range r.Data {
			if p.Status != "ok" {
				t.Errorf("%s status=%s reason=%v", id, p.Status, p.Reason)
			}
		}
		if r.Context.User != "kbdiag_ro" || r.Verdict != "OK" || code != 0 {
			t.Errorf("context=%+v verdict=%s exit=%d, want OK/0", r.Context, r.Verdict, code)
		}
	})

	// Unlike PG, KES V8R6 (V008R006C009B0014) shows a user without any
	// monitoring role all of sys_stat_replication, sys_stat_wal_receiver and
	// data_directory (seen 2026-09-26 on both nodes), while other sessions'
	// SQL stays masked. So nothing in status is redacted and the verdict is
	// the same as for system.
	t.Run("status", func(t *testing.T) {
		r, code := kbdiag(t, env, append([]string{"status"}, ro...)...)
		for _, id := range []string{"inst.info", "inst.databases", "inst.downstreams", "inst.disk"} {
			if p := r.Data[id]; p.Status != "ok" {
				t.Errorf("%s status=%s reason=%v", id, p.Status, p.Reason)
			}
		}
		up, wantUp := r.Data["inst.upstream"], "not_applicable"
		if role == "standby" {
			wantUp = "ok"
		}
		if up.Status != wantUp {
			t.Errorf("inst.upstream status=%s reason=%v, want %s", up.Status, up.Reason, wantUp)
		}
		for _, x := range up.rowsOf() {
			if x["status"] != "streaming" {
				t.Errorf("inst.upstream status column = %v, want streaming", x["status"])
			}
		}
		for _, x := range r.Data["inst.downstreams"].rowsOf() {
			if x["state"] == nil || x["sync_state"] == nil {
				t.Errorf("downstream %v hidden from kbdiag_ro", x)
			}
		}
		var fields []string
		for _, x := range r.Redacted {
			fields = append(fields, x.ProbeID+"."+x.Field)
		}
		wantVerdict, wantCode, wantFields := "OK", 0, []string(nil)
		if r.Context.User != "kbdiag_ro" || r.Verdict != wantVerdict || code != wantCode || !reflect.DeepEqual(fields, wantFields) {
			t.Errorf("context=%+v verdict=%s exit=%d redacted=%v, want %s/%d %v", r.Context, r.Verdict, code, fields, wantVerdict, wantCode, wantFields)
		}
	})
}
