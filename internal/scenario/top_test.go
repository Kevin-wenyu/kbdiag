package scenario

import (
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// topFacts rebuilds sql.top from the stage 0 capture taken with
// sys_stat_statements.track=top set in one session (the lab's server
// setting is none, so kbdiag itself would see it skipped). userid 10 is
// system and dbid 13778 is test (space_node1_sys_dbsize).
func topFacts(t *testing.T, user string) facts.SQLTop {
	t.Helper()
	// kbdiag's own connection sees the server's track=none, as it would have
	top := facts.SQLTop{Status: facts.StatusOK, Track: "none"}
	for _, m := range loadKsql(t, "top_node1_"+user+"_stmts_tracked").rows {
		ms := func(k string) float64 { return *kF64(t, m[k]) / 1000 }
		top.Rows = append(top.Rows, facts.Statement{QueryID: kI64(t, m["queryid"]), Username: str("system"), Datname: str("test"), Calls: *kI64(t, m["calls"]),
			TotalExecS: ms("total_exec_time"), MeanExecS: ms("mean_exec_time"), MaxExecS: ms("max_exec_time"), Rows: *kI64(t, m["rows"]),
			SharedBlksHit: *kI64(t, m["shared_blks_hit"]), SharedBlksRead: *kI64(t, m["shared_blks_read"]), TempBlksWritten: *kI64(t, m["temp_blks_written"]), Query: m["query"]})
	}
	return top
}

func TestTopText(t *testing.T) {
	for _, x := range []struct {
		golden, user string
		c            facts.Context
		verdict      rule.Verdict
	}{
		{"top_tracked", "sys", v02Context("primary", "system", "local"), rule.VerdictOK},
		{"top_tracked_ro", "ro", v02Context("primary", "kbdiag_ro", "remote"), rule.VerdictUNKNOWN},
	} {
		t.Run(x.golden, func(t *testing.T) {
			rep := Top(x.c, topFacts(t, x.user), TopOptions{Limit: 20, By: "time"})
			assertGolden(t, x.golden, rep)
			if rep.Verdict != x.verdict {
				t.Errorf("verdict = %s", rep.Verdict)
			}
		})
	}
	t.Run("top_none", func(t *testing.T) {
		rep := Top(v02Context("primary", "system", "local"), facts.SQLTop{Status: facts.StatusSkipped,
			Reason: "sys_stat_statements.track=none: statements are not being collected; set it to top or all"}, TopOptions{Limit: 20, By: "time"})
		assertGolden(t, "top_none", rep)
		if rep.Verdict != rule.VerdictUNKNOWN {
			t.Errorf("verdict = %s", rep.Verdict)
		}
	})
}

// Each --by order, --limit, a long and hostile statement, times past a
// minute, and nothing recorded yet.
func TestTopTextEdges(t *testing.T) {
	c := v02Context("primary", "system", "local")
	top := topFacts(t, "sys")
	top.Rows = append(top.Rows, facts.Statement{Calls: 1_000_000, TotalExecS: 3725.5, MeanExecS: 0.0037, MaxExecS: 12, Rows: 1_000_000, SharedBlksRead: 9_000_000,
		Username: str("app"), Datname: str("prod\x1b[2J"), Query: str("select *\n  from   big‮ where id = $1")})
	for _, by := range []string{"mean", "calls", "io", "temp"} {
		t.Run(by, func(t *testing.T) {
			assertGolden(t, "top_by_"+by, Top(c, top, TopOptions{Limit: 3, By: by}))
		})
	}
	top.Track = "all"
	assertGolden(t, "top_track_all", Top(c, top, TopOptions{Limit: 2, By: "time"}))
	rep := Top(c, facts.SQLTop{Status: facts.StatusOK, Track: "top"}, TopOptions{Limit: 20, By: "time"})
	if rep.Verdict != rule.VerdictOK {
		t.Errorf("empty: %s", rep.Verdict)
	}
}
