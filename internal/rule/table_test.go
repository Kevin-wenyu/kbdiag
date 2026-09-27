package rule

import (
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func TestTable(t *testing.T) {
	age := func(n int32) *int32 { return &n }
	info := func(xid int32, opts ...string) facts.TableInfos {
		return facts.TableInfos{Status: facts.StatusOK, Rows: []facts.TableInfo{{Schemaname: "public", Relname: "t", Relkind: "r", Reltuples: 10000, XIDAge: age(xid), MXIDAge: age(0), Reloptions: opts}}}
	}
	stats := func(dead int64) facts.TableStats {
		return facts.TableStats{Status: facts.StatusOK, Rows: []facts.TableStat{{NDeadTup: dead}}}
	}
	cases := []struct {
		name    string
		i       facts.TableInfos
		s       facts.TableStats
		role    string
		verdict Verdict
		ids     string
	}{
		{"lab table (due, autovacuum on)", info(5), stats(5000), "primary", VerdictOK, ""},
		{"autovacuum off for it, past the line", info(5, "autovacuum_enabled=off"), stats(5000), "primary", VerdictWARN, "vacuum.table_disabled"},
		{"old", info(250_000_000), stats(0), "primary", VerdictWARN, "freeze.table_age"},
		{"at the stop limit", info(facts.XIDStopAge), stats(0), "primary", VerdictFAIL, "freeze.table_age"},
		{"both", info(250_000_000, "autovacuum_enabled=off"), stats(5000), "primary", VerdictWARN, "freeze.table_age,vacuum.table_disabled"},
		{"standby: ages judged, statistics not", info(250_000_000, "autovacuum_enabled=off"), facts.TableStats{Status: facts.StatusNotApplicable}, "standby", VerdictWARN, "freeze.table_age"},
		{"no frozen xid (partitioned)", facts.TableInfos{Status: facts.StatusOK, Rows: []facts.TableInfo{{Schemaname: "public", Relname: "p", Relkind: "p"}}}, facts.TableStats{Status: facts.StatusOK}, "primary", VerdictOK, ""},
		{"info not collected", facts.TableInfos{Status: facts.StatusError}, stats(0), "primary", VerdictUNKNOWN, ""},
		{"stats not collected", info(5), facts.TableStats{Status: facts.StatusSkipped}, "primary", VerdictUNKNOWN, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Table(c.i, c.s, labLimits, labVacuum, c.role, "test")
			var ids []string
			for _, f := range r.Findings {
				ids = append(ids, f.ID)
			}
			if r.Verdict != c.verdict || strings.Join(ids, ",") != c.ids {
				t.Errorf("verdict=%s ids=%v", r.Verdict, ids)
			}
		})
	}
}
