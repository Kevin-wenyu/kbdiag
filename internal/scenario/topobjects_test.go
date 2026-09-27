package scenario

import (
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// topObjectsFacts rebuilds both probes from the stage 0 captures (limit 30
// and 20 there; the probes read every relation).
func topObjectsFacts(t *testing.T, nodeUser string) (facts.ObjectTables, facts.ObjectIndexes) {
	t.Helper()
	tb := facts.ObjectTables{Status: facts.StatusOK}
	for _, m := range loadKsql(t, "topobj_"+nodeUser+"_rels").rows {
		tb.Rows = append(tb.Rows, facts.ObjectTable{Schemaname: *m["nspname"], Relname: *m["relname"], Relkind: *m["relkind"], TotalBytes: *kI64(t, m["total"]),
			TableBytes: *kI64(t, m["main"]), IndexBytes: *kI64(t, m["idx"]), ToastBytes: kI64(t, m["toast"]), Reltuples: *kI64(t, m["reltuples"])})
	}
	ix := facts.ObjectIndexes{Status: facts.StatusOK}
	for _, m := range loadKsql(t, "topobj_"+nodeUser+"_idx").rows {
		ix.Rows = append(ix.Rows, facts.ObjectIndex{Schemaname: *m["nspname"], Relname: *m["relname"], TableName: *m["tbl"], Bytes: *kI64(t, m["pg_relation_size"])})
	}
	return tb, ix
}

func TestTopObjectsText(t *testing.T) {
	tb, ix := topObjectsFacts(t, "node1_sys")
	c := v02Context("primary", "system", "local")
	c.Database = "test"
	rep := TopObjects(c, tb, ix, TopObjectsOptions{Limit: 20})
	assertGolden(t, "topobjects_primary", rep)
	if rep.Verdict != rule.VerdictOK || len(rep.Data[facts.ObjectTablesID].Rows) != 20 || rep.Data[facts.ObjectTablesID].Truncated != 10 {
		t.Errorf("verdict=%s rows=%d", rep.Verdict, len(rep.Data[facts.ObjectTablesID].Rows))
	}
}

// A table locked by a VACUUM FULL (the probe stops at lock_timeout), an
// empty database, sizes in TB, a hostile name.
func TestTopObjectsTextEdges(t *testing.T) {
	c := v02Context("primary", "system", "local")
	c.Database = "test"
	// the probe orders by total size; the partitioned parent (0) comes last
	big := facts.ObjectTables{Status: facts.StatusOK, Rows: []facts.ObjectTable{
		{Schemaname: "public", Relname: "events_2026", Relkind: "r", TotalBytes: 3 << 40, TableBytes: 3<<40 - 1<<30, IndexBytes: 1 << 30},
		{Schemaname: "app", Relname: "log", Relkind: "m", TotalBytes: 5 << 30, TableBytes: 4 << 30, IndexBytes: 1 << 30, ToastBytes: i64(0), Reltuples: 12_000_000_000},
		{Schemaname: "public", Relname: "events\x1b[2J", Relkind: "p"},
	}}
	rep := TopObjects(c, big, facts.ObjectIndexes{Status: facts.StatusSkipped, Reason: "timeout 55P03: canceling statement due to lock timeout"}, TopObjectsOptions{Limit: 20})
	assertGolden(t, "topobjects_edges", rep)
	if rep.Verdict != rule.VerdictUNKNOWN {
		t.Errorf("verdict = %s", rep.Verdict)
	}
	rep = TopObjects(c, facts.ObjectTables{Status: facts.StatusOK}, facts.ObjectIndexes{Status: facts.StatusOK}, TopObjectsOptions{Limit: 0})
	if rep.Verdict != rule.VerdictOK {
		t.Errorf("empty: %s", rep.Verdict)
	}
}
