package rule

import (
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func TestWaits(t *testing.T) {
	g := func(state string, n, masked int) facts.Wait {
		return facts.Wait{State: str(state), Sessions: n, PIDs: make([]int32, n), Masked: masked}
	}
	cases := []struct {
		name string
		in   facts.WaitSummary
		want Verdict
	}{
		{"empty", facts.WaitSummary{Status: facts.StatusOK}, VerdictOK},
		{"visible groups", facts.WaitSummary{Status: facts.StatusOK, Rows: []facts.Wait{g("active", 3, 0), g("idle", 10, 0)}}, VerdictOK},
		{"masked sessions", facts.WaitSummary{Status: facts.StatusOK, Rows: []facts.Wait{g("active", 1, 0), {Sessions: 7, Masked: 7}}}, VerdictUNKNOWN},
		{"untracked sessions", facts.WaitSummary{Status: facts.StatusOK, Rows: []facts.Wait{g("disabled", 2, 0)}}, VerdictUNKNOWN},
		{"skipped", facts.WaitSummary{Status: facts.StatusSkipped}, VerdictUNKNOWN},
		{"error", facts.WaitSummary{Status: facts.StatusError}, VerdictUNKNOWN},
	}
	for _, c := range cases {
		if r := Waits(c.in); r.Verdict != c.want || len(r.Findings) != 0 {
			t.Errorf("%s: verdict=%s findings=%d, want %s/0", c.name, r.Verdict, len(r.Findings), c.want)
		}
	}
}

func TestStatus(t *testing.T) {
	okInfo := facts.InstInfo{Status: facts.StatusOK}
	okDown := facts.InstDownstreams{Status: facts.StatusOK, Rows: []facts.Downstream{{ApplicationName: str("node2"), State: str("streaming"), SyncState: str("quorum")}}}
	maskedDown := facts.InstDownstreams{Status: facts.StatusOK, Rows: []facts.Downstream{{ApplicationName: str("node2")}}}
	noUp := facts.InstUpstream{Status: facts.StatusNotApplicable, Reason: "primary"}
	hidden := facts.InstDatabases{Status: facts.StatusOK, Rows: []facts.Database{{Datname: "secret"}}}
	cases := []struct {
		name string
		i    facts.InstInfo
		d    facts.InstDatabases
		n    facts.InstDownstreams
		u    facts.InstUpstream
		want Verdict
	}{
		{"all ok", okInfo, facts.InstDatabases{Status: facts.StatusOK}, okDown, noUp, VerdictOK},
		{"hidden database size is not judged", okInfo, hidden, okDown, noUp, VerdictOK},
		{"hidden downstream state is not judged", okInfo, hidden, maskedDown, noUp, VerdictOK},
		{"info error", facts.InstInfo{Status: facts.StatusError}, hidden, okDown, noUp, VerdictUNKNOWN},
		{"databases skipped", okInfo, facts.InstDatabases{Status: facts.StatusSkipped}, okDown, noUp, VerdictUNKNOWN},
		{"downstreams error", okInfo, hidden, facts.InstDownstreams{Status: facts.StatusError}, noUp, VerdictUNKNOWN},
		{"upstream error", okInfo, hidden, okDown, facts.InstUpstream{Status: facts.StatusError}, VerdictUNKNOWN},
		{"upstream skipped", okInfo, hidden, okDown, facts.InstUpstream{Status: facts.StatusSkipped}, VerdictUNKNOWN},
	}
	for _, c := range cases {
		if r := Status(c.i, c.d, c.n, c.u); r.Verdict != c.want || len(r.Findings) != 0 {
			t.Errorf("%s: verdict=%s findings=%d, want %s/0", c.name, r.Verdict, len(r.Findings), c.want)
		}
	}
}

func TestStatusConnections(t *testing.T) {
	info := func(conn, max, reserved int32) facts.InstInfo {
		return facts.InstInfo{Status: facts.StatusOK, Rows: []facts.Info{{Connections: conn, MaxConnections: max, SuperuserReserved: reserved}}}
	}
	d := facts.InstDatabases{Status: facts.StatusOK}
	n := facts.InstDownstreams{Status: facts.StatusOK}
	u := facts.InstUpstream{Status: facts.StatusNotApplicable}
	full := "97 connections in use, reaching the 97 ordinary users may open (max_connections 100 minus 3 reserved for superusers): ordinary users can no longer connect"
	cases := []struct {
		name    string
		i       facts.InstInfo
		d       facts.InstDatabases
		want    Verdict
		symptom string
	}{
		{"idle", info(6, 100, 3), d, VerdictOK, ""},
		{"80% is no longer a warning", info(78, 100, 3), d, VerdictOK, ""},
		{"one usable slot left", info(96, 100, 3), d, VerdictOK, ""},
		{"fail when usable slots are used up", info(97, 100, 3), d, VerdictFAIL, full},
		{"superusers past the usable limit", info(99, 100, 3), d, VerdictFAIL,
			"99 connections in use, reaching the 97 ordinary users may open (max_connections 100 minus 3 reserved for superusers): ordinary users can no longer connect"},
		{"no reserve", info(100, 100, 0), d, VerdictFAIL,
			"100 connections in use, reaching the 100 ordinary users may open (max_connections 100 minus 0 reserved for superusers): ordinary users can no longer connect"},
		{"no usable slots is not judged", info(3, 3, 3), d, VerdictOK, ""},
		{"finding survives an unknown elsewhere", info(97, 100, 3), facts.InstDatabases{Status: facts.StatusError}, VerdictFAIL, full},
		{"info error", facts.InstInfo{Status: facts.StatusError}, d, VerdictUNKNOWN, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Status(c.i, c.d, n, u)
			if r.Verdict != c.want {
				t.Errorf("verdict = %s, want %s", r.Verdict, c.want)
			}
			if c.symptom == "" {
				if len(r.Findings) != 0 {
					t.Errorf("findings = %+v, want none", r.Findings)
				}
				return
			}
			if len(r.Findings) != 1 {
				t.Fatalf("findings = %d, want 1", len(r.Findings))
			}
			f := r.Findings[0]
			if f.ID != "inst.connections" || f.Level != LevelFAIL || f.Symptom != c.symptom {
				t.Errorf("finding %s %s %q", f.ID, f.Level, f.Symptom)
			}
			for _, k := range []string{"connections", "max_connections", "superuser_reserved_connections"} {
				if _, ok := f.Evidence[0].Fields[k]; !ok || f.Evidence[0].ProbeID != facts.InstInfoID {
					t.Errorf("evidence missing %s", k)
				}
			}
			if len(f.Next) != 1 || f.Next[0].Command != "kbdiag sessions" {
				t.Errorf("next = %+v", f.Next)
			}
		})
	}
}

func TestStatusUpstream(t *testing.T) {
	info := facts.InstInfo{Status: facts.StatusOK}
	d := facts.InstDatabases{Status: facts.StatusOK}
	n := facts.InstDownstreams{Status: facts.StatusOK}
	up := func(status *string) facts.InstUpstream {
		return facts.InstUpstream{Status: facts.StatusOK, Rows: []facts.Upstream{{Status: status, SenderHost: str("192.168.105.10"), SenderPort: i32(54321), SlotName: str("repmgr_slot_2"), LastMsgAgeS: f64(8)}}}
	}
	cases := []struct {
		name    string
		u       facts.InstUpstream
		want    Verdict
		symptom string
		status  any
	}{
		{"streaming", up(str("streaming")), VerdictOK, "", nil},
		{"no walreceiver", facts.InstUpstream{Status: facts.StatusOK}, VerdictWARN,
			"the standby has no WAL receiver and is not receiving WAL from the primary: if the primary fails now, no standby can take over", nil},
		{"stopping", up(str("stopping")), VerdictWARN,
			"the standby's WAL receiver is stopping, not streaming, so it is not receiving WAL from the primary: if the primary fails now, no standby can take over", "stopping"},
		{"starting", up(str("starting")), VerdictWARN,
			"the standby's WAL receiver is starting, not streaming, so it is not receiving WAL from the primary: if the primary fails now, no standby can take over", "starting"},
		{"status hidden", up(nil), VerdictUNKNOWN, "", nil},
		{"primary", facts.InstUpstream{Status: facts.StatusNotApplicable, Reason: "primary"}, VerdictOK, "", nil},
		{"error", facts.InstUpstream{Status: facts.StatusError}, VerdictUNKNOWN, "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Status(info, d, n, c.u)
			if r.Verdict != c.want {
				t.Errorf("verdict = %s, want %s", r.Verdict, c.want)
			}
			if c.symptom == "" {
				if len(r.Findings) != 0 {
					t.Errorf("findings = %+v, want none", r.Findings)
				}
				return
			}
			if len(r.Findings) != 1 {
				t.Fatalf("findings = %d, want 1", len(r.Findings))
			}
			f := r.Findings[0]
			if f.ID != "inst.upstream" || f.Level != LevelWARN || f.Symptom != c.symptom {
				t.Errorf("finding %s %s %q", f.ID, f.Level, f.Symptom)
			}
			ev := f.Evidence[0]
			if st, ok := ev.Fields["status"]; ev.ProbeID != facts.InstUpstreamID || !ok || st != c.status {
				t.Errorf("evidence = %+v", ev)
			}
			if len(f.Next) != 1 || f.Next[0].Kind != "verify" || f.Next[0].Command != "kbdiag slots" || !strings.Contains(f.Next[0].Note, "primary") {
				t.Errorf("next = %+v", f.Next)
			}
		})
	}
}

func TestSlots(t *testing.T) {
	i64 := func(v int64) *int64 { return &v }
	slot := func(name string, active bool) facts.Slot {
		return facts.Slot{Name: name, Type: "physical", Active: active, Xmin: xid(5859), RetainedWALBytes: i64(50331648)}
	}
	cases := []struct {
		name    string
		in      facts.SlotList
		want    Verdict
		symptom []string
	}{
		{"no slots", facts.SlotList{Status: facts.StatusOK}, VerdictOK, nil},
		{"active slot", facts.SlotList{Status: facts.StatusOK, Rows: []facts.Slot{slot("a", true)}}, VerdictOK, nil},
		{"inactive with xmin", facts.SlotList{Status: facts.StatusOK, Rows: []facts.Slot{slot("a", true), slot("repmgr_slot_2", false)}}, VerdictWARN,
			[]string{"replication slot repmgr_slot_2 is inactive, retaining 48 MB WAL, xmin 5859 holding back the vacuum horizon"}},
		{"inactive, no xmin, never reserved WAL", facts.SlotList{Status: facts.StatusOK, Rows: []facts.Slot{{Name: "b"}}}, VerdictWARN,
			[]string{"replication slot b is inactive, retaining no WAL"}},
		{"inactive, zero WAL", facts.SlotList{Status: facts.StatusOK, Rows: []facts.Slot{{Name: "c", RetainedWALBytes: i64(0)}}}, VerdictWARN,
			[]string{"replication slot c is inactive, retaining 0 bytes WAL"}},
		{"inactive, under a megabyte", facts.SlotList{Status: facts.StatusOK, Rows: []facts.Slot{{Name: "d", RetainedWALBytes: i64(45720)}}}, VerdictWARN,
			[]string{"replication slot d is inactive, retaining 45 kB WAL"}},
		{"inactive logical slot", facts.SlotList{Status: facts.StatusOK, Rows: []facts.Slot{{Name: "l", Type: "logical", CatalogXmin: xid(90), RetainedWALBytes: i64(0)}}}, VerdictWARN,
			[]string{"replication slot l is inactive, retaining 0 bytes WAL, catalog_xmin 90 holding back the system catalogs' horizon"}},
		{"inactive, replay ahead of restart_lsn", facts.SlotList{Status: facts.StatusOK, Rows: []facts.Slot{{Name: "n", RetainedWALBytes: i64(-5)}}}, VerdictWARN,
			[]string{"replication slot n is inactive, retaining 0 bytes WAL"}},
		{"inactive, gigabytes", facts.SlotList{Status: facts.StatusOK, Rows: []facts.Slot{{Name: "e", RetainedWALBytes: i64(12 << 30)}}}, VerdictWARN,
			[]string{"replication slot e is inactive, retaining 12 GB WAL"}},
		{"error", facts.SlotList{Status: facts.StatusError}, VerdictUNKNOWN, nil},
		{"skipped", facts.SlotList{Status: facts.StatusSkipped}, VerdictUNKNOWN, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Slots(c.in)
			if r.Verdict != c.want {
				t.Errorf("verdict = %s, want %s", r.Verdict, c.want)
			}
			var got []string
			for _, f := range r.Findings {
				if f.ID != "slot.inactive" || f.Level != LevelWARN {
					t.Errorf("finding %s/%s", f.ID, f.Level)
				}
				for _, k := range []string{"slot_name", "active", "xmin", "catalog_xmin", "retained_wal_bytes"} {
					if _, ok := f.Evidence[0].Fields[k]; !ok {
						t.Errorf("evidence missing %s", k)
					}
				}
				if len(f.Next) != 1 || f.Next[0].Command != "kbdiag status" || !strings.Contains(f.Next[0].Note, "inst.upstream") || !strings.Contains(f.Next[0].Note, "downstream") {
					t.Errorf("next = %+v", f.Next)
				}
				got = append(got, f.Symptom)
			}
			if len(got) != len(c.symptom) || (len(got) > 0 && got[0] != c.symptom[0]) {
				t.Errorf("symptoms = %q, want %q", got, c.symptom)
			}
		})
	}
}
