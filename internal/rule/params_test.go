package rule

import (
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func TestParams(t *testing.T) {
	file, line, v := "/data/kingbase.auto.conf", int32(3), "100"
	pending := facts.Param{Name: "max_connections", Setting: &v, Source: "configuration file", Sourcefile: &file, Sourceline: &line, Context: "kingbase", PendingRestart: true}
	clean := facts.Param{Name: "work_mem", Setting: &v, Source: "configuration file", Sourcefile: &file, Sourceline: &line, Context: "user"}
	hidden := clean
	hidden.Sourcefile, hidden.Sourceline = nil, nil
	cases := []struct {
		name    string
		p       facts.ParamsChanged
		verdict Verdict
		n       int
	}{
		{"clean", facts.ParamsChanged{Status: facts.StatusOK, Rows: []facts.Param{clean}}, VerdictOK, 0},
		{"pending", facts.ParamsChanged{Status: facts.StatusOK, Rows: []facts.Param{clean, pending, pending}}, VerdictWARN, 2},
		{"account without the file: some parameters are not visible", facts.ParamsChanged{Status: facts.StatusOK, Rows: []facts.Param{hidden}}, VerdictUNKNOWN, 0},
		{"pending seen by that account still WARN", facts.ParamsChanged{Status: facts.StatusOK, Rows: []facts.Param{hidden, pending}}, VerdictWARN, 1},
		{"not collected", facts.ParamsChanged{Status: facts.StatusSkipped}, VerdictUNKNOWN, 0},
		{"nothing changed", facts.ParamsChanged{Status: facts.StatusOK}, VerdictOK, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Params(c.p)
			if r.Verdict != c.verdict || len(r.Findings) != c.n {
				t.Fatalf("verdict=%s findings=%+v", r.Verdict, r.Findings)
			}
			for _, f := range r.Findings {
				if f.ID != "params.pending_restart" || f.Level != LevelWARN || !strings.Contains(f.Symptom, "max_connections") || !strings.Contains(f.Symptom, "kingbase.auto.conf:3") {
					t.Errorf("%+v", f)
				}
			}
		})
	}
}
