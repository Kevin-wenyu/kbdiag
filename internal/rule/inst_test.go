package rule

import (
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
	okDown := facts.InstDownstreams{Status: facts.StatusOK, Rows: []int64{1}}
	hidden := facts.InstDatabases{Status: facts.StatusOK, Rows: []facts.Database{{Datname: "secret"}}}
	cases := []struct {
		name string
		i    facts.InstInfo
		d    facts.InstDatabases
		n    facts.InstDownstreams
		want Verdict
	}{
		{"all ok", okInfo, facts.InstDatabases{Status: facts.StatusOK}, okDown, VerdictOK},
		{"hidden database size is not judged", okInfo, hidden, okDown, VerdictOK},
		{"info error", facts.InstInfo{Status: facts.StatusError}, hidden, okDown, VerdictUNKNOWN},
		{"databases skipped", okInfo, facts.InstDatabases{Status: facts.StatusSkipped}, okDown, VerdictUNKNOWN},
		{"downstreams error", okInfo, hidden, facts.InstDownstreams{Status: facts.StatusError}, VerdictUNKNOWN},
		{"not applicable does not count", okInfo, hidden, facts.InstDownstreams{Status: facts.StatusNotApplicable}, VerdictOK},
	}
	for _, c := range cases {
		if r := Status(c.i, c.d, c.n); r.Verdict != c.want {
			t.Errorf("%s: verdict=%s, want %s", c.name, r.Verdict, c.want)
		}
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
		{"inactive with xmin", facts.SlotList{Status: facts.StatusOK, Rows: []facts.Slot{slot("a", true), slot("repmgr_slot_2", false)}}, VerdictFAIL,
			[]string{"复制槽 repmgr_slot_2 未激活，保留 48 MB WAL，xmin 5859 压着视界"}},
		{"inactive, no xmin, never reserved WAL", facts.SlotList{Status: facts.StatusOK, Rows: []facts.Slot{{Name: "b"}}}, VerdictFAIL,
			[]string{"复制槽 b 未激活，未保留 WAL"}},
		{"inactive, zero WAL", facts.SlotList{Status: facts.StatusOK, Rows: []facts.Slot{{Name: "c", RetainedWALBytes: i64(0)}}}, VerdictFAIL,
			[]string{"复制槽 c 未激活，保留 0 MB WAL"}},
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
				if f.ID != "slot.inactive" || f.Level != LevelFAIL {
					t.Errorf("finding %s/%s", f.ID, f.Level)
				}
				for _, k := range []string{"slot_name", "active", "xmin", "retained_wal_bytes"} {
					if _, ok := f.Evidence[0].Fields[k]; !ok {
						t.Errorf("evidence missing %s", k)
					}
				}
				got = append(got, f.Symptom)
			}
			if len(got) != len(c.symptom) || (len(got) > 0 && got[0] != c.symptom[0]) {
				t.Errorf("symptoms = %q, want %q", got, c.symptom)
			}
		})
	}
}
