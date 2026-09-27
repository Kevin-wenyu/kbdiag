package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// Checkpoint only shows: how many requested checkpoints are too many has
// no objective line in cumulative counters.
func Checkpoint(c facts.Context, s facts.CheckpointStats, l facts.CheckpointLast, set facts.CheckpointSettings) *report.Report {
	rep := report.New("checkpoint", c, rule.Display(false, s.Status, l.Status, set.Status))
	rep.AddProbe(facts.CheckpointLastID, l.Status, l.Reason, facts.CheckpointLastColumns, rows(l.Rows), 0)
	rep.AddProbe(facts.CheckpointStatsID, s.Status, s.Reason, facts.CheckpointStatsColumns, rows(s.Rows), 0)
	rep.AddProbe(facts.CheckpointSettingsID, set.Status, set.Reason, facts.CheckpointSettingsColumns, rows(set.Rows), 0)
	rep.SetCheckpoint(s, l, set)
	return rep
}
