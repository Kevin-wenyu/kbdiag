package rule

import (
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

var labVacuum = facts.VacuumSettings{Status: facts.StatusOK, Rows: []facts.VacuumSetting{{Autovacuum: "on", TrackCounts: "on", Threshold: 50, ScaleFactor: 0.2, NaptimeS: 60, MaxWorkers: 3}}}

func vt(dead int64, tuples float32, opts ...string) facts.VacuumTable {
	return facts.VacuumTable{Schemaname: "public", Relname: "t", NDeadTup: dead, Reltuples: tuples, Reloptions: opts}
}

func TestVacuumThreshold(t *testing.T) {
	s := labVacuum.Rows[0]
	cases := []struct {
		t    facts.VacuumTable
		want float32
		on   bool
	}{
		{vt(0, 10000), 2050, true},
		{vt(0, 0), 50, true},
		{vt(0, 10000, "autovacuum_enabled=off"), 2050, false},
		{vt(0, 10000, "autovacuum_enabled=false"), 2050, false},
		{vt(0, 10000, "autovacuum_enabled=OFF"), 2050, false},
		{vt(0, 10000, "autovacuum_enabled=0"), 2050, false},
		{vt(0, 10000, "autovacuum_enabled=true"), 2050, true},
		{vt(0, 10000, "autovacuum_vacuum_threshold=1000", "autovacuum_vacuum_scale_factor=0.01"), 1100, true},
		{vt(0, 10000, "toast.autovacuum_enabled=off", "fillfactor=70"), 2050, true},
		{vt(0, 10000, "garbage", "autovacuum_vacuum_scale_factor=x"), 2050, true},
		{vt(0, 1<<40), 50 + float32(0.2)*float32(1<<40), true},
		{vt(0, 10000, "autovacuum_enabled=fa"), 2050, false},
		{vt(0, 10000, "autovacuum_enabled= Of "), 2050, false},
		{vt(0, 10000, "autovacuum_enabled=o"), 2050, true},
	}
	for _, c := range cases {
		got, on := VacuumThreshold(c.t, s)
		if got != c.want || on != c.on {
			t.Errorf("%v: threshold %v on %v, want %v %v", c.t.Reloptions, got, on, c.want, c.on)
		}
	}
}

func TestVacuum(t *testing.T) {
	off := labVacuum
	off.Rows = []facts.VacuumSetting{{Autovacuum: "off", TrackCounts: "on", Threshold: 50, ScaleFactor: 0.2}}
	noCounts := labVacuum
	noCounts.Rows = []facts.VacuumSetting{{Autovacuum: "on", TrackCounts: "off", Threshold: 50, ScaleFactor: 0.2}}
	tables := func(rows ...facts.VacuumTable) facts.VacuumTables {
		return facts.VacuumTables{Status: facts.StatusOK, Rows: rows}
	}
	cases := []struct {
		name    string
		t       facts.VacuumTables
		s       facts.VacuumSettings
		role    string
		verdict Verdict
		ids     string
	}{
		{"clean", tables(vt(0, 10)), labVacuum, "primary", VerdictOK, ""},
		{"due with autovacuum on is its normal queue", tables(vt(5000, 10000)), labVacuum, "primary", VerdictOK, ""},
		{"table off, past the line (lab injection)", tables(vt(5000, 10000, "autovacuum_enabled=off")), labVacuum, "primary", VerdictWARN, "vacuum.table_disabled"},
		{"table off, at the line is not past it", tables(vt(2050, 10000, "autovacuum_enabled=off")), labVacuum, "primary", VerdictOK, ""},
		{"table off, below the line", tables(vt(10, 10000, "autovacuum_enabled=off")), labVacuum, "primary", VerdictOK, ""},
		{"autovacuum off", tables(vt(0, 10)), off, "primary", VerdictWARN, "vacuum.disabled"},
		{"track_counts off", tables(), noCounts, "primary", VerdictWARN, "vacuum.disabled"},
		{"both", tables(vt(5000, 10000, "autovacuum_enabled=off")), off, "primary", VerdictWARN, "vacuum.disabled,vacuum.table_disabled"},
		{"standby: nothing judged", facts.VacuumTables{Status: facts.StatusNotApplicable}, off, "standby", VerdictOK, ""},
		{"tables not collected", facts.VacuumTables{Status: facts.StatusSkipped}, labVacuum, "primary", VerdictUNKNOWN, ""},
		{"settings not collected: no threshold to judge by", tables(vt(5000, 10000, "autovacuum_enabled=off")), facts.VacuumSettings{Status: facts.StatusError}, "primary", VerdictUNKNOWN, ""},
		{"settings not collected", tables(vt(0, 1)), facts.VacuumSettings{Status: facts.StatusError}, "primary", VerdictUNKNOWN, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Vacuum(c.t, c.s, c.role, "test")
			var ids []string
			for _, f := range r.Findings {
				ids = append(ids, f.ID)
				if f.Level != LevelWARN {
					t.Errorf("level %s", f.Level)
				}
			}
			if r.Verdict != c.verdict || strings.Join(ids, ",") != c.ids {
				t.Errorf("verdict=%s ids=%v", r.Verdict, ids)
			}
		})
	}
}

func TestQuoteIdent(t *testing.T) {
	for in, want := range map[string]string{"orders": "orders", "t1_$x": "t1_$x", "Orders": `"Orders"`, "order": `"order"`, "user": `"user"`, "event": "event", "t\u200b": "\"t\u200b\"", "1t": `"1t"`, `a"b`: `"a""b"`, "my table": `"my table"`, "": `""`, "表": `"表"`} {
		if got := quoteIdent(in); got != want {
			t.Errorf("quoteIdent(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestParseBool(t *testing.T) {
	for in, want := range map[string]string{"on": "true", "OFF": "false", "of": "false", "o": "?", "t": "true", "fals": "false", "falsey": "?", " yes ": "true", "n": "false", "1": "true", "0": "false", "": "?", "2": "?"} {
		v, ok := parseBool(in)
		got := "?"
		if ok {
			got = map[bool]string{true: "true", false: "false"}[v]
		}
		if got != want {
			t.Errorf("parseBool(%q) = %s, want %s", in, got, want)
		}
	}
	if !hasControl("t\u200b") || !hasControl("t\x9b") || !hasControl("a\u2028") || hasControl("表") {
		t.Error("hasControl must match what the text escapes")
	}
}
