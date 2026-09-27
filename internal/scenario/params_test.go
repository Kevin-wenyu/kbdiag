package scenario

import (
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// paramsFacts rebuilds params.changed from the stage 0 capture of every
// non-default row, keeping what the probe keeps.
func paramsFacts(t *testing.T, name string) facts.ParamsChanged {
	t.Helper()
	p := facts.ParamsChanged{Status: facts.StatusOK}
	for _, m := range loadKsql(t, name).rows {
		switch *m["source"] {
		case "default", "override", "client", "session":
			continue
		}
		var line *int32
		if n := kI64(t, m["sourceline"]); n != nil {
			x := int32(*n)
			line = &x
		}
		p.Rows = append(p.Rows, facts.Param{Name: *m["name"], Setting: m["setting"], Unit: m["unit"], Source: *m["source"], Sourcefile: m["sourcefile"],
			Sourceline: line, BootVal: m["boot_val"], ResetVal: m["reset_val"], Context: *m["context"], PendingRestart: kBool(m["pending_restart"])})
	}
	return p
}

func TestParamsText(t *testing.T) {
	rep := Params(v02Context("primary", "system", "local"), paramsFacts(t, "params_node1_sys_nondefault"))
	assertGolden(t, "params_primary", rep)
	if rep.Verdict != rule.VerdictOK {
		t.Errorf("verdict = %s", rep.Verdict)
	}
	rep = Params(v02Context("primary", "kbdiag_ro", "remote"), paramsFacts(t, "params_node1_ro_nondefault"))
	assertGolden(t, "params_ro", rep)
	if rep.Verdict != rule.VerdictUNKNOWN {
		t.Errorf("verdict = %s", rep.Verdict)
	}
}

// A change waiting for a restart, a per-database and a per-role setting,
// an empty value, a hostile value, and a probe that failed.
func TestParamsTextEdges(t *testing.T) {
	file, line := "/home/kingbase/data/kingbase.auto.conf", int32(9)
	p := facts.ParamsChanged{Status: facts.StatusOK, Rows: []facts.Param{
		{Name: "max_connections", Setting: str("100"), Source: "configuration file", Sourcefile: &file, Sourceline: &line, Context: "kingbase", PendingRestart: true},
		{Name: "search_path", Setting: str("app, public"), Source: "database", Context: "user"},
		{Name: "work_mem", Setting: str("65536"), Unit: str("kB"), Source: "user", Context: "user"},
		{Name: "application_name", Setting: str(""), Source: "environment variable", Context: "user"},
		{Name: "log_line_prefix", Setting: str("%t \x1b[31m"), Source: "command line", Context: "sighup"},
	}}
	rep := Params(v02Context("primary", "system", "local"), p)
	assertGolden(t, "params_edges", rep)
	if rep.Verdict != rule.VerdictWARN || len(rep.Findings) != 1 {
		t.Errorf("verdict=%s findings=%d", rep.Verdict, len(rep.Findings))
	}
	if rep := Params(v02Context("primary", "system", "local"), facts.ParamsChanged{Status: facts.StatusError, Reason: "XX000: boom"}); rep.Verdict != rule.VerdictUNKNOWN {
		t.Errorf("verdict=%s", rep.Verdict)
	}
}
