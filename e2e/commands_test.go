//go:build vm

package e2e

import (
	"fmt"
	"reflect"
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
	})

	t.Run("no such pid", func(t *testing.T) {
		r, code := kbdiag(t, nil, "session", "2147483647")
		if r.Verdict != "UNKNOWN" || code != 3 || len(r.Data[sessionProbe].Rows) != 0 || len(r.Data["lock.list"].Rows) != 0 {
			t.Errorf("verdict=%s exit=%d data=%v", r.Verdict, code, r.Data)
		}
	})
}

// L3 + L4 for txn.prepared: the row matches ksql; flagged past the threshold.
func TestTxnPrepared(t *testing.T) {
	if role == "standby" {
		r, _ := kbdiag(t, nil, "txn")
		p := r.Data["txn.prepared"]
		if p.Status != "not_applicable" || p.Reason == nil || !strings.Contains(*p.Reason, "主库") || len(p.Rows) != 0 {
			t.Errorf("standby txn.prepared = %+v", p)
		}
		return
	}
	inject(t, "prepared")
	want := strings.Split(ksql(t, "select owner, database, transaction from sys_prepared_xacts where gid='kbdiag_inj_2pc'"), "|")
	time.Sleep(1500 * time.Millisecond)
	r, code := kbdiag(t, nil, "txn", "--prepared-fail", "1")
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
	if got := findings(r, "txn.prepared", "gid", "kbdiag_inj_2pc"); !reflect.DeepEqual(got, []string{"FAIL"}) || r.Verdict != "FAIL" || code != 2 {
		t.Errorf("findings=%v verdict=%s exit=%d", got, r.Verdict, code)
	}

	r, _ = kbdiag(t, nil, "txn")
	if got := findings(r, "txn.prepared", "gid", "kbdiag_inj_2pc"); len(got) != 0 {
		t.Errorf("a seconds-old prepared transaction is flagged at the default: %v", got)
	}
}

// L4 for txn.long: WARN and FAIL by the transaction's age.
func TestTxnLong(t *testing.T) {
	inject(t, "idle_txn")
	fp := idleTxn(t)
	time.Sleep(1500 * time.Millisecond)
	r, _ := kbdiag(t, nil, "txn", "--limit", "0", "--xact-warn", "1", "--xact-fail", "3600")
	row := okProbe(t, r, sessionProbe, sessionColumns).row("pid", fp.pid)
	if row == nil || row["backend_xid"] != fp.xid {
		t.Fatalf("injected row = %v", row)
	}
	if got := findings(r, "txn.long", "pid", fp.pid); !reflect.DeepEqual(got, []string{"WARN"}) {
		t.Errorf("WARN band: %v", got)
	}
	r, _ = kbdiag(t, nil, "txn", "--xact-warn", "1", "--xact-fail", "1")
	if got := findings(r, "txn.long", "pid", fp.pid); !reflect.DeepEqual(got, []string{"FAIL"}) || r.Verdict != "FAIL" {
		t.Errorf("FAIL band: %v verdict=%s", got, r.Verdict)
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

// L3: status agrees with ksql.
func TestStatus(t *testing.T) {
	want := strings.Split(ksql(t, `select floor(extract(epoch from sys_postmaster_start_time()))::bigint,
current_setting('max_connections'), current_setting('superuser_reserved_connections'),
current_setting('data_directory'), (select count(*) from sys_stat_replication),
(select string_agg(datname::text, ',' order by datname) from sys_database where not datistemplate)`), "|")
	r, code := kbdiag(t, nil, "status")
	if r.Verdict != "OK" || code != 0 || len(r.Findings) != 0 {
		t.Errorf("verdict=%s exit=%d findings=%d", r.Verdict, code, len(r.Findings))
	}
	info := okProbe(t, r, "inst.info", []string{"version", "start_time", "uptime_s", "connections", "max_connections", "superuser_reserved_connections", "data_directory"})
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
	if fmt.Sprint(row["max_connections"]) != want[1] || fmt.Sprint(row["superuser_reserved_connections"]) != want[2] || row["data_directory"] != want[3] {
		t.Errorf("row = %v, ksql says %v", row, want)
	}
	if c, _ := row["connections"].(float64); c < 1 || c > 100 {
		t.Errorf("connections = %v", row["connections"])
	}
	if v, _ := row["version"].(string); !strings.HasPrefix(v, "KingbaseES V") {
		t.Errorf("version = %v", row["version"])
	}
	var names []string
	for _, d := range okProbe(t, r, "inst.databases", []string{"datname", "size_bytes"}).rowsOf() {
		names = append(names, d["datname"].(string))
		if s, _ := d["size_bytes"].(float64); s <= 0 {
			t.Errorf("size of %v = %v", d["datname"], d["size_bytes"])
		}
	}
	if strings.Join(names, ",") != want[5] {
		t.Errorf("databases = %v, ksql says %s", names, want[5])
	}
	down := okProbe(t, r, "inst.downstreams", []string{"downstreams"})
	if len(down.Rows) != 1 || fmt.Sprint(down.Rows[0][0]) != want[4] {
		t.Errorf("downstreams = %v, ksql says %s", down.Rows, want[4])
	}
	if (role == "primary") != (want[4] != "0") {
		t.Errorf("%s with %s downstreams: the lab has one standby", role, want[4])
	}
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
			if got := findings(r, "slot.inactive", "slot_name", "kbdiag_inj_slot"); !reflect.DeepEqual(got, []string{"FAIL"}) || code != 2 {
				t.Errorf("findings=%v exit=%d", got, code)
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
		if got := findings(r, "slot.inactive", "slot_name", name); !reflect.DeepEqual(got, []string{"FAIL"}) || r.Verdict != "FAIL" || code != 2 {
			t.Errorf("findings=%v verdict=%s exit=%d", got, r.Verdict, code)
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

	t.Run("status and slots still answer", func(t *testing.T) {
		for _, cmd := range []string{"status", "slots"} {
			r, code := kbdiag(t, env, append([]string{cmd}, ro...)...)
			for id, p := range r.Data {
				if p.Status != "ok" {
					t.Errorf("%s: %s status=%s reason=%v", cmd, id, p.Status, p.Reason)
				}
			}
			if r.Context.User != "kbdiag_ro" || r.Verdict != "OK" || code != 0 {
				t.Errorf("%s: context=%+v verdict=%s exit=%d, want OK/0", cmd, r.Context, r.Verdict, code)
			}
		}
	})
}
