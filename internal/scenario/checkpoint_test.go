package scenario

import (
	"testing"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// checkpointFacts rebuilds the probes from the stage 0 captures; the
// collection time is set to 22:50:00, the capture has no now().
func checkpointFacts(t *testing.T, node string) (facts.Context, facts.CheckpointStats, facts.CheckpointLast, facts.CheckpointSettings) {
	t.Helper()
	at := time.Date(2026, 9, 27, 22, 50, 0, 0, cst)
	c := v02Context("primary", "system", "local")
	if node == "node2" {
		c.Role = "standby"
	}
	c.CollectedAt = at
	b := loadKsql(t, "checkpoint_"+node+"_sys_bgwriter").rows[0]
	reset := kTime(t, b["stats_reset"])
	resetAge := at.Sub(*reset).Seconds()
	st := facts.CheckpointStats{Status: facts.StatusOK, Rows: []facts.BGWriter{{CheckpointsTimed: *kI64(t, b["checkpoints_timed"]), CheckpointsReq: *kI64(t, b["checkpoints_req"]),
		CheckpointWriteS: *kF64(t, b["checkpoint_write_time"]) / 1000, CheckpointSyncS: *kF64(t, b["checkpoint_sync_time"]) / 1000,
		BuffersCheckpoint: *kI64(t, b["buffers_checkpoint"]), BuffersClean: *kI64(t, b["buffers_clean"]), MaxwrittenClean: *kI64(t, b["maxwritten_clean"]),
		BuffersBackend: *kI64(t, b["buffers_backend"]), BuffersBackendFsync: *kI64(t, b["buffers_backend_fsync"]), BuffersAlloc: *kI64(t, b["buffers_alloc"]),
		StatsReset: reset, StatsResetAgeS: &resetAge}}}
	m := loadKsql(t, "checkpoint_"+node+"_sys_control").rows[0]
	ct, err := time.Parse("2006-01-02 15:04:05-07", *m["checkpoint_time"])
	if err != nil {
		t.Fatal(err)
	}
	ct = ct.In(cst)
	age := at.Sub(ct).Seconds()
	l := facts.CheckpointLast{Status: facts.StatusOK, Rows: []facts.LastCheckpoint{{Time: &ct, AgeS: &age, LSN: m["checkpoint_lsn"], RedoLSN: m["redo_lsn"], RedoWALFile: m["redo_wal_file"]}}}
	s := facts.CheckpointSettings{Status: facts.StatusOK, Rows: []facts.CheckpointSetting{{TimeoutS: 300, MaxWALSizeBytes: 1 << 30, CompletionTarget: 0.5, WarningS: 30, LogCheckpoints: "on"}}}
	return c, st, l, s
}

func TestCheckpointText(t *testing.T) {
	c, st, l, s := checkpointFacts(t, "node1")
	rep := Checkpoint(c, st, l, s)
	assertGolden(t, "checkpoint_primary", rep)
	if rep.Verdict != rule.VerdictOK {
		t.Errorf("verdict = %s", rep.Verdict)
	}
}

// node2: restartpoints, 9 backend fsyncs; then nothing collected but the
// settings, and counters that are all zero.
func TestCheckpointTextEdges(t *testing.T) {
	c, st, l, s := checkpointFacts(t, "node2")
	assertGolden(t, "checkpoint_standby", Checkpoint(c, st, l, s))
	zero := facts.CheckpointStats{Status: facts.StatusOK, Rows: []facts.BGWriter{{}}}
	rep := Checkpoint(c, zero, facts.CheckpointLast{Status: facts.StatusError, Reason: "XX000: boom"}, s)
	assertGolden(t, "checkpoint_edges", rep)
	if rep.Verdict != rule.VerdictUNKNOWN {
		t.Errorf("verdict = %s", rep.Verdict)
	}
}
