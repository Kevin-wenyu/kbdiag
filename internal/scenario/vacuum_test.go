package scenario

import (
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// vacuumFacts rebuilds vacuum from the stage 0 captures of node1 with the
// dead-tuple table. The capture has no reltuples per table: n_live_tup stands
// in for it, except kbdiag_inj_dead, whose reltuples (10000) and reloptions
// come from vacuum_node1_sys_reloptions_dead.
func vacuumFacts(t *testing.T) (facts.VacuumTables, facts.VacuumProgress, facts.VacuumSettings) {
	t.Helper()
	opts := map[string][]string{}
	tuples := map[string]float32{}
	for _, m := range loadKsql(t, "vacuum_node1_sys_reloptions_dead").rows {
		// ksql prints the text[] as {a=b,c=d}
		opts[*m["rel"]] = strings.Split(strings.Trim(*m["reloptions"], "{}"), ",")
		tuples[*m["rel"]] = float32(*kF64(t, m["reltuples"]))
	}
	tb := facts.VacuumTables{Status: facts.StatusOK}
	for _, m := range loadKsql(t, "vacuum_node1_sys_tables_dead").rows {
		rel := *m["relname"]
		n, ok := tuples[rel]
		if !ok {
			n = float32(*kI64(t, m["n_live_tup"]))
		}
		tb.Rows = append(tb.Rows, facts.VacuumTable{Schemaname: *m["schemaname"], Relname: rel, NLiveTup: *kI64(t, m["n_live_tup"]),
			NDeadTup: *kI64(t, m["n_dead_tup"]), Reltuples: n, Reloptions: opts[rel],
			VacuumCount: *kI64(t, m["vacuum_count"]), AutovacuumCount: *kI64(t, m["autovacuum_count"])})
	}
	set := map[string]string{}
	for _, m := range loadKsql(t, "vacuum_node1_sys_settings").rows {
		set[*m["name"]] = *m["setting"]
	}
	s := facts.VacuumSetting{Autovacuum: set["autovacuum"], TrackCounts: set["track_counts"], Threshold: *kI64OrNil(str(set["autovacuum_vacuum_threshold"])),
		ScaleFactor: *kF64(t, str(set["autovacuum_vacuum_scale_factor"])), NaptimeS: *kI64OrNil(str(set["autovacuum_naptime"])), MaxWorkers: 3}
	return tb, facts.VacuumProgress{Status: facts.StatusOK}, facts.VacuumSettings{Status: facts.StatusOK, Rows: []facts.VacuumSetting{s}}
}

func TestVacuumText(t *testing.T) {
	tb, p, s := vacuumFacts(t)
	t.Run("primary", func(t *testing.T) {
		c := v02Context("primary", "system", "local")
		c.Database = "test"
		rep := Vacuum(c, tb, p, s, VacuumOptions{Limit: 20})
		assertGolden(t, "vacuum_primary", rep)
		if rep.Verdict != rule.VerdictWARN || len(rep.Findings) != 1 {
			t.Errorf("verdict=%s findings=%d", rep.Verdict, len(rep.Findings))
		}
	})
	// node2: the same table reads all zeros there (vacuum_node2_sys_tables_dead)
	t.Run("standby", func(t *testing.T) {
		c := v02Context("standby", "system", "local")
		c.Database = "test"
		na := facts.VacuumTables{Status: facts.StatusNotApplicable, Reason: "standby: table statistics are local to each node and stay 0 here; run on the primary"}
		np := facts.VacuumProgress{Status: facts.StatusNotApplicable, Reason: "standby: autovacuum does not run here; run on the primary"}
		rep := Vacuum(c, na, np, s, VacuumOptions{Limit: 20})
		assertGolden(t, "vacuum_standby", rep)
		if rep.Verdict != rule.VerdictOK {
			t.Errorf("verdict=%s", rep.Verdict)
		}
	})
}

// Running vacuums (autovacuum, manual in another database, masked),
// settings not collected, autovacuum off globally, a hostile table name,
// reltuples in the billions.
func TestVacuumTextEdges(t *testing.T) {
	c := v02Context("primary", "kbdiag_ro", "local")
	c.Database = "test"
	tb := facts.VacuumTables{Status: facts.StatusOK, Rows: []facts.VacuumTable{
		{Schemaname: "public", Relname: "big", NLiveTup: 3_000_000_000, NDeadTup: 700_000_000, Reltuples: 3_000_000_000, LastAutovacuumAgeS: f64(90061), AutovacuumCount: 12},
		{Schemaname: "public", Relname: "x\x1b[2J", NDeadTup: 60, Reltuples: 0, Reloptions: []string{"autovacuum_enabled=false"}, LastVacuumAgeS: f64(5)},
	}}
	p := facts.VacuumProgress{Status: facts.StatusOK, Rows: []facts.VacuumRun{
		{PID: 101, Datname: str("test"), Relation: str("public.big"), Phase: str("scanning heap"), HeapBlksTotal: i64(1000), HeapBlksScanned: i64(250), IsAutovacuum: boolp(true), XactAgeS: f64(3700)},
		{PID: 102, Datname: str("other"), Relation: str("16384"), Phase: str("vacuuming indexes"), HeapBlksTotal: i64(0), HeapBlksScanned: i64(0), IsAutovacuum: boolp(false), XactAgeS: f64(2)},
		{PID: 103, Datname: str("test"), Relation: str("public.t")},
	}}
	off := facts.VacuumSettings{Status: facts.StatusOK, Rows: []facts.VacuumSetting{{Autovacuum: "off", TrackCounts: "on", Threshold: 50, ScaleFactor: 0.2, NaptimeS: 60, MaxWorkers: 3}}}
	rep := Vacuum(c, tb, p, off, VacuumOptions{Limit: 20})
	assertGolden(t, "vacuum_edges", rep)
	if rep.Verdict != rule.VerdictWARN || len(rep.Findings) != 2 || len(rep.Redacted) != 1 {
		t.Errorf("verdict=%s findings=%d redacted=%v", rep.Verdict, len(rep.Findings), rep.Redacted)
	}
	rep = Vacuum(c, tb, facts.VacuumProgress{Status: facts.StatusOK}, facts.VacuumSettings{Status: facts.StatusSkipped, Reason: "timeout 57014: canceling statement due to statement timeout"}, VacuumOptions{Limit: 1})
	assertGolden(t, "vacuum_nosettings", rep)
	if rep.Verdict != rule.VerdictUNKNOWN {
		t.Errorf("verdict=%s", rep.Verdict)
	}
}

func boolp(b bool) *bool { return &b }
