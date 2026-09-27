package scenario

import (
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// seqFacts rebuilds seq.list from the stage 0 capture. readable is what
// the probe adds: false for every row kbdiag_ro read (all last_value NULL).
func seqFacts(t *testing.T, name string, readable bool) facts.SeqList {
	t.Helper()
	l := facts.SeqList{Status: facts.StatusOK}
	for _, m := range loadKsql(t, name).rows {
		l.Rows = append(l.Rows, facts.Sequence{Schemaname: *m["schemaname"], Sequencename: *m["sequencename"], DataType: *m["data_type"],
			StartValue: *kI64(t, m["start_value"]), MinValue: *kI64(t, m["min_value"]), MaxValue: *kI64(t, m["max_value"]), IncrementBy: *kI64(t, m["increment_by"]),
			Cycle: kBool(m["cycle"]), CacheSize: *kI64(t, m["cache_size"]), LastValue: kI64(t, m["last_value"]), Readable: readable})
	}
	return l
}

func TestSeqText(t *testing.T) {
	c := v02Context("primary", "system", "local")
	c.Database = "test"
	rep := Seq(c, seqFacts(t, "seq_node1_sys_list", true), SeqOptions{Limit: 20})
	assertGolden(t, "seq_primary", rep)
	if rep.Verdict != rule.VerdictOK {
		t.Errorf("verdict = %s", rep.Verdict)
	}
	ro := v02Context("primary", "kbdiag_ro", "remote")
	ro.Database = "test"
	rep = Seq(ro, seqFacts(t, "seq_node1_ro_list", false), SeqOptions{Limit: 3})
	assertGolden(t, "seq_ro", rep)
	if rep.Verdict != rule.VerdictUNKNOWN {
		t.Errorf("verdict = %s", rep.Verdict)
	}
}

// An exhausted int sequence (FAIL), one that cycles at its limit, a
// descending one past its MINVALUE by the next step, a bigint with a
// lowered MAXVALUE and a hostile name.
func TestSeqTextEdges(t *testing.T) {
	c := v02Context("primary", "system", "local")
	c.Database = "test"
	v := func(n int64) *int64 { return &n }
	l := facts.SeqList{Status: facts.StatusOK, Rows: []facts.Sequence{
		{Schemaname: "public", Sequencename: "orders_id_seq", DataType: "integer", StartValue: 1, MinValue: 1, MaxValue: 2147483647, IncrementBy: 1, LastValue: v(2147483647), Readable: true},
		{Schemaname: "public", Sequencename: "ring", DataType: "smallint", StartValue: 1, MinValue: 1, MaxValue: 32767, IncrementBy: 1, Cycle: true, LastValue: v(32767), Readable: true},
		{Schemaname: "public", Sequencename: "down", DataType: "bigint", StartValue: -1, MinValue: -1000, MaxValue: -1, IncrementBy: -10, LastValue: v(-991), Readable: true},
		{Schemaname: "app", Sequencename: "Capped\x1b[2J", DataType: "bigint", StartValue: 1, MinValue: 1, MaxValue: 100, IncrementBy: 1, LastValue: v(100), Readable: true},
	}}
	rep := Seq(c, l, SeqOptions{Limit: 20})
	assertGolden(t, "seq_edges", rep)
	if rep.Verdict != rule.VerdictFAIL || len(rep.Findings) != 3 {
		t.Errorf("verdict=%s findings=%+v", rep.Verdict, rep.Findings)
	}
}
