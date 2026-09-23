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

const sessionProbe = "session.activity"

var sessionColumns = []string{
	"pid", "usename", "datname", "application_name", "client_addr", "backend_type",
	"state", "backend_xid", "backend_xmin", "xact_age_s", "query_age_s", "state_age_s",
	"wait_event_type", "wait_event", "query",
}

// fingerprint is what ksql, not kbdiag, sees of an injected session.
type fingerprint struct {
	pid float64
	xid any // float64 on a primary; nil on a standby, which cannot assign one
}

func injected(t *testing.T, tag string) fingerprint {
	t.Helper()
	out := ksql(t, "select pid, coalesce(backend_xid::text, '') from sys_stat_activity where application_name = 'kbdiag_inj_"+tag+"'")
	f := strings.Split(out, "|")
	if len(f) != 2 {
		t.Fatalf("want exactly one kbdiag_inj_%s session, ksql says %q", tag, out)
	}
	pid, err := strconv.Atoi(f[0])
	if err != nil {
		t.Fatalf("pid %q: %v", f[0], err)
	}
	fp := fingerprint{pid: float64(pid)}
	if f[1] != "" {
		x, err := strconv.ParseUint(f[1], 10, 32)
		if err != nil {
			t.Fatalf("xid %q: %v", f[1], err)
		}
		fp.xid = float64(x)
	}
	return fp
}

// idleTxn is the idle_txn fingerprint; on a primary it must hold an xid, or
// the backend_xid assertions would compare nil with nil.
func idleTxn(t *testing.T) fingerprint {
	t.Helper()
	fp := injected(t, "idle_txn")
	if (role == "primary") != (fp.xid != nil) {
		t.Fatalf("%s: injected backend_xid = %v", role, fp.xid)
	}
	return fp
}

// ages is what ksql computes for pid's xact/query/state ages right now.
func ages(t *testing.T, pid float64) map[string]float64 {
	t.Helper()
	out := ksql(t, fmt.Sprintf(`select extract(epoch from now()-xact_start)::float8,
extract(epoch from now()-query_start)::float8, extract(epoch from now()-state_change)::float8
from sys_stat_activity where pid = %d`, int(pid)))
	f := strings.Split(out, "|")
	if len(f) != 3 {
		t.Fatalf("ages of %v: ksql says %q", pid, out)
	}
	m := map[string]float64{}
	for i, k := range []string{"xact_age_s", "query_age_s", "state_age_s"} {
		v, err := strconv.ParseFloat(f[i], 64)
		if err != nil {
			t.Fatalf("%s %q: %v", k, f[i], err)
		}
		m[k] = v
	}
	return m
}

// xmin is pid's backend_xmin as system sees it: float64, or nil if unset.
func xmin(t *testing.T, pid float64) any {
	t.Helper()
	out := ksql(t, fmt.Sprintf("select coalesce(backend_xmin::text, '') from sys_stat_activity where pid = %d", int(pid)))
	if out == "" {
		return nil
	}
	v, err := strconv.ParseUint(out, 10, 32)
	if err != nil {
		t.Fatalf("backend_xmin %q: %v", out, err)
	}
	return float64(v)
}

// exitFor is the PRD §5 verdict → exit code mapping.
var exitFor = map[string]int{"OK": 0, "WARN": 1, "FAIL": 2, "UNKNOWN": 3}

// flagged returns the idle_in_txn findings naming pid, for the given report.
func flagged(r report, pid float64) (levels []string) {
	for _, f := range r.Findings {
		for _, e := range f.Evidence {
			if f.ID == "session.idle_in_txn" && e.Fields["pid"] == pid {
				levels = append(levels, f.Level)
			}
		}
	}
	return levels
}

// L3: the probe runs on a real instance, scans, and returns the values ksql sees.
func TestSessionsProbeContract(t *testing.T) {
	inject(t, "idle_txn")
	fp := idleTxn(t)
	time.Sleep(1500 * time.Millisecond) // ages of at least 1s, so an always-0 age fails
	// The injection spaces the four timestamps about 1s apart (idle_txn.sh).

	before := ages(t, fp.pid)
	r, code := kbdiag(t, nil, "sessions", "--limit", "0")
	after := ages(t, fp.pid)
	if want, ok := exitFor[r.Verdict]; !ok || code != want {
		t.Errorf("verdict %s exit %d", r.Verdict, code)
	}
	if r.Command != "sessions" || r.Context.Role != role || r.Context.Location != "local" || r.Context.User != "system" ||
		!strings.HasPrefix(r.Context.Version, "KingbaseES V") {
		t.Errorf("context = %+v, command = %q", r.Context, r.Command)
	}
	p, ok := r.Data[sessionProbe]
	if !ok {
		t.Fatalf("no %s in data: %v", sessionProbe, r.Data)
	}
	if p.Status != "ok" || p.Reason != nil || p.Truncated != 0 {
		t.Fatalf("probe status=%s reason=%v truncated=%d", p.Status, p.Reason, p.Truncated)
	}
	if !reflect.DeepEqual(p.Columns, sessionColumns) {
		t.Errorf("columns = %v", p.Columns)
	}
	if len(r.Redacted) != 0 {
		t.Errorf("superuser sees redactions: %+v", r.Redacted)
	}
	if p.row("application_name", "kbdiag") != nil {
		t.Error("kbdiag lists its own session")
	}

	row := p.row("pid", fp.pid)
	if row == nil {
		t.Fatalf("injected pid %v not in rows", fp.pid)
	}
	want := map[string]any{
		"usename": "system", "datname": "test", "application_name": "kbdiag_inj_idle_txn",
		"client_addr": nil, "backend_type": "client backend", "state": "idle in transaction",
		"backend_xid": fp.xid, "wait_event_type": "Client", "wait_event": "ClientRead",
	}
	for k, v := range want {
		if row[k] != v {
			t.Errorf("%s = %#v, want %#v", k, row[k], v)
		}
	}
	// kbdiag's ages (rounded to 0.1s) must fall between ksql's before and after.
	for _, k := range []string{"xact_age_s", "query_age_s", "state_age_s"} {
		s, ok := row[k].(float64)
		if !ok || before[k] < 1 || s < before[k]-0.1 || s > after[k]+0.1 {
			t.Errorf("%s = %#v, ksql says %.2f before and %.2f after", k, row[k], before[k], after[k])
		}
	}
	if q, _ := row["query"].(string); !strings.Contains(q, "kbdiag_last") {
		t.Errorf("query = %#v, want the injected last statement", row["query"])
	}
}

// L4: an idle-in-transaction session is flagged once past the threshold, and
// only then; an active long query next to it is not.
func TestSessionsIdleInTxn(t *testing.T) {
	inject(t, "idle_txn")
	inject(t, "long_query") // decoy: old, but running, not idle in a transaction
	fp := idleTxn(t)
	decoy := injected(t, "long_query")

	t.Run("below the default threshold", func(t *testing.T) {
		r, _ := kbdiag(t, nil, "sessions", "--limit", "0")
		// A negative is only meaningful if the session was seen.
		if p := r.Data[sessionProbe]; p.Status != "ok" || p.row("pid", fp.pid) == nil || r.Verdict == "UNKNOWN" {
			t.Fatalf("status=%s verdict=%s, injected pid %v in rows: %v", p.Status, r.Verdict, fp.pid, p.row("pid", fp.pid) != nil)
		}
		if got := flagged(r, fp.pid); len(got) != 0 {
			t.Errorf("a session idle for seconds is flagged at the 300s default: %v", got)
		}
	})

	t.Run("past a scaled threshold", func(t *testing.T) {
		time.Sleep(1500 * time.Millisecond) // let state_age_s pass the 1s threshold
		r, code := kbdiag(t, nil, "sessions", "--idle-in-txn-warn", "1")
		if got := flagged(r, fp.pid); !reflect.DeepEqual(got, []string{"WARN"}) {
			t.Fatalf("injected pid %v: findings %v, want one WARN", fp.pid, got)
		}
		// sessions has no FAIL rule, so any WARN finding makes the verdict exactly WARN.
		if r.Verdict != "WARN" || code != 1 {
			t.Errorf("verdict %s exit %d, want WARN/1", r.Verdict, code)
		}
		for _, f := range r.Findings {
			for _, e := range f.Evidence {
				if e.Fields["pid"] == decoy.pid {
					t.Errorf("decoy long query %v in finding %s", decoy.pid, f.ID)
				}
				if e.Fields["pid"] == fp.pid && e.Fields["backend_xid"] != fp.xid {
					t.Errorf("evidence backend_xid = %v, want %v", e.Fields["backend_xid"], fp.xid)
				}
			}
		}
	})

	t.Run("--active hides the row but still judges it", func(t *testing.T) {
		r, code := kbdiag(t, nil, "sessions", "--active", "--limit", "0", "--idle-in-txn-warn", "1")
		if r.Data[sessionProbe].row("pid", fp.pid) != nil {
			t.Errorf("idle-in-txn pid %v shown under --active", fp.pid)
		}
		if got := flagged(r, fp.pid); !reflect.DeepEqual(got, []string{"WARN"}) || code != 1 {
			t.Errorf("findings %v exit %d, want one WARN and exit 1", got, code)
		}
	})
}

// L4: a session with track_activities off for itself (as ALTER ROLE ... SET
// would do) shows state='disabled' while kbdiag's own setting is on; its real
// state is unseen, so the verdict is UNKNOWN, never OK.
func TestSessionsUntrackedSession(t *testing.T) {
	inject(t, "untracked")
	fp := injected(t, "untracked")
	r, code := kbdiag(t, nil, "sessions", "--limit", "0")
	p := r.Data[sessionProbe]
	if p.Status != "ok" || !reflect.DeepEqual(p.Columns, sessionColumns) {
		t.Fatalf("probe status=%s reason=%v columns=%v, want ok: kbdiag's own track_activities is on", p.Status, p.Reason, p.Columns)
	}
	if row := p.row("pid", fp.pid); row == nil || row["state"] != "disabled" {
		t.Fatalf("injected row = %v, want state disabled", row)
	}
	n := 0
	for _, rw := range p.Rows {
		if rw[6] == "disabled" { // state
			n++
		}
	}
	var fields []string
	for _, d := range r.Redacted {
		if d.Reason == "track_activities_off" {
			fields = append(fields, d.Field)
			if d.ProbeID != sessionProbe || d.RowsAffected != n {
				t.Errorf("redacted entry %+v, want %d rows", d, n)
			}
		}
	}
	if want := []string{"state", "state_age_s", "wait_event_type", "wait_event", "query"}; !reflect.DeepEqual(fields, want) {
		t.Errorf("track_activities_off fields = %v, want %v", fields, want)
	}
	if r.Verdict != "UNKNOWN" || code != 3 {
		t.Errorf("verdict=%s exit=%d, want UNKNOWN/3", r.Verdict, code)
	}
}

// L4: with track_activities off, the probe reports skipped and the verdict is
// UNKNOWN, never an empty OK.
func TestSessionsTrackActivitiesOff(t *testing.T) {
	inject(t, "track_off")
	r, code := kbdiag(t, nil, "sessions")
	p := r.Data[sessionProbe]
	if p.Status != "skipped" || p.Reason == nil || !strings.Contains(*p.Reason, "track_activities=off") {
		t.Errorf("probe status=%s reason=%v", p.Status, p.Reason)
	}
	if len(p.Rows) != 0 || len(r.Findings) != 0 {
		t.Errorf("skipped probe carries rows=%d findings=%d", len(p.Rows), len(r.Findings))
	}
	if r.Verdict != "UNKNOWN" || code != 3 {
		t.Errorf("verdict=%s exit=%d, want UNKNOWN/3", r.Verdict, code)
	}
}

// L5 (one cell): a non-monitor user over TCP sees masked rows; kbdiag lists
// the masked columns in redacted[] and cannot claim OK.
func TestSessionsRedactedForNonMonitorUser(t *testing.T) {
	inject(t, "idle_txn")
	fp := idleTxn(t)
	env := []string{"PGPASSWORD=kbdiag_ro_T3st"}
	r, code := kbdiag(t, env, "sessions", "--limit", "0", "--host", "127.0.0.1", "-U", "kbdiag_ro")

	if r.Context.Location != "remote" || r.Context.User != "kbdiag_ro" {
		t.Errorf("context = %+v", r.Context)
	}
	if r.Verdict != "UNKNOWN" || code != 3 {
		t.Errorf("verdict=%s exit=%d, want UNKNOWN/3", r.Verdict, code)
	}
	p := r.Data[sessionProbe]
	// A missing column would read as nil below and pass for every masked one.
	if p.Status != "ok" || !reflect.DeepEqual(p.Columns, sessionColumns) {
		t.Fatalf("probe status=%s columns=%v", p.Status, p.Columns)
	}
	row := p.row("pid", fp.pid)
	if row == nil {
		t.Fatalf("injected pid %v not visible to kbdiag_ro", fp.pid)
	}
	masked := []string{"client_addr", "backend_type", "state", "xact_age_s", "query_age_s",
		"state_age_s", "wait_event_type", "wait_event", "query"}
	// Every column has an expected value: masked ones NULL (query carries the
	// marker), visible ones equal to what system sees via ksql.
	want := map[string]any{
		"pid": fp.pid, "usename": "system", "datname": "test", "application_name": "kbdiag_inj_idle_txn",
		"backend_xid": fp.xid, "backend_xmin": xmin(t, fp.pid),
	}
	for _, c := range masked {
		want[c] = nil
	}
	want["query"] = "<insufficient privilege>"
	if len(want) != len(sessionColumns) {
		t.Fatalf("expectations cover %d of %d columns", len(want), len(sessionColumns))
	}
	for _, c := range sessionColumns {
		if row[c] != want[c] {
			t.Errorf("%s = %#v, want %#v", c, row[c], want[c])
		}
	}
	// With --limit 0 every row is in the output, so the count is checkable.
	n := 0
	for _, rw := range p.Rows {
		if rw[len(rw)-1] == "<insufficient privilege>" {
			n++
		}
	}
	var fields []string
	for _, d := range r.Redacted {
		if d.ProbeID != sessionProbe || d.Reason != "insufficient_privilege" || d.RowsAffected != n {
			t.Errorf("redacted entry %+v", d)
		}
		fields = append(fields, d.Field)
	}
	if !reflect.DeepEqual(fields, masked) {
		t.Errorf("redacted fields = %v, want %v", fields, masked)
	}
}
