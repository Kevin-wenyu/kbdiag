package scenario

import (
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// The one running operation stage 0 caught: CREATE INDEX on public.orders
// (progress_node1_sys_index_running). The capture has no start time: 4s is
// made up.
func TestProgressText(t *testing.T) {
	m := loadKsql(t, "progress_node1_sys_index_running").rows[0]
	rel := "public.orders"
	op := facts.Operation{PID: int32(*kI64(t, m["pid"])), Command: *m["command"], Datname: m["datname"], Relation: &rel, Phase: m["phase"],
		Done: kI64(t, m["blocks_done"]), Total: kI64(t, m["blocks_total"]), Unit: "blocks", RunningS: f64(4.2), WaitingLockers: i64(0)}
	rep := Progress(v02Context("primary", "system", "local"), facts.ProgressList{Status: facts.StatusOK, Rows: []facts.Operation{op}})
	assertGolden(t, "progress_index", rep)
	if rep.Verdict != rule.VerdictOK {
		t.Errorf("verdict = %s", rep.Verdict)
	}
	// every view empty on node2 (progress_node2_sys_*)
	assertGolden(t, "progress_none", Progress(v02Context("standby", "system", "local"), facts.ProgressList{Status: facts.StatusOK}))
}

// An autovacuum, a CREATE INDEX CONCURRENTLY waiting for transactions, a
// VACUUM FULL in another database, a checkpoint, and one hidden from this
// account.
func TestProgressTextEdges(t *testing.T) {
	ops := []facts.Operation{
		{PID: 10, Command: "autovacuum", Datname: str("test"), Relation: str("public.big"), Phase: str("scanning heap"), Done: i64(250), Total: i64(1000), Unit: "blocks", RunningS: f64(3700)},
		{PID: 11, Command: "CREATE INDEX CONCURRENTLY", Datname: str("test"), Relation: str("public.t"), Phase: str("waiting for old snapshots"), Done: i64(0), Total: i64(0), Unit: "tuples", RunningS: f64(90), WaitingLockers: i64(2)},
		{PID: 12, Command: "VACUUM FULL", Datname: str("other"), Relation: str("16384"), Phase: str("seq scanning heap"), Done: i64(1), Total: i64(3), Unit: "blocks", RunningS: f64(5)},
		{PID: 13, Command: "CHECKPOINT", Phase: str("flushing\x1b[2J"), Done: i64(10), Total: i64(16384), Unit: "buffers", RunningS: f64(1)},
		{PID: 14, Command: "VACUUM", Datname: str("test"), Relation: str("public.x")},
	}
	rep := Progress(v02Context("primary", "kbdiag_ro", "remote"), facts.ProgressList{Status: facts.StatusOK, Rows: ops})
	assertGolden(t, "progress_edges", rep)
	if rep.Verdict != rule.VerdictUNKNOWN || len(rep.Redacted) != 1 {
		t.Errorf("verdict=%s redacted=%v", rep.Verdict, rep.Redacted)
	}
}
