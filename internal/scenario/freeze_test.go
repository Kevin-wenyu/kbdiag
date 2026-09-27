package scenario

import (
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

func u32(t *testing.T, s *string) uint32 { return uint32(*kI64(t, s)) }
func i32v(t *testing.T, s *string) int32 { return int32(*kI64(t, s)) }

// freezeFacts rebuilds freeze from the stage 0 captures. The capture's
// relation is regclass text, unqualified for pg_catalog (on the search
// path); the probe qualifies every name, so pg_catalog. is added back. The
// capture had "limit 30" and no relfrozenxid filter: its two rows with
// relfrozenxid 0 are what the probe leaves out. The capture measured
// pg_total_relation_size; the probe estimates relpages × block_size, and the
// capture's number stands in for it.
func freezeFacts(t *testing.T, node string) (facts.FreezeDatabases, facts.FreezeTables, facts.FreezeLimits) {
	t.Helper()
	d := facts.FreezeDatabases{Status: facts.StatusOK}
	for _, m := range loadKsql(t, "freeze_"+node+"_sys_db").rows {
		d.Rows = append(d.Rows, facts.FrozenDatabase{Datname: *m["datname"], DatFrozenXID: u32(t, m["datfrozenxid"]), XIDAge: i32v(t, m["xid_age"]),
			DatMinMXID: u32(t, m["datminmxid"]), MXIDAge: i32v(t, m["mxid_age"]), AllowConn: kBool(m["datallowconn"])})
	}
	tb := facts.FreezeTables{Status: facts.StatusOK}
	for _, m := range loadKsql(t, "freeze_"+node+"_sys_rel").rows {
		if *m["relfrozenxid"] == "0" {
			continue
		}
		rel := *m["rel"]
		if !strings.Contains(rel, ".") {
			rel = "pg_catalog." + rel
		}
		tb.Rows = append(tb.Rows, facts.FrozenTable{Relation: rel, Relkind: *m["relkind"], RelFrozenXID: u32(t, m["relfrozenxid"]), XIDAge: i32v(t, m["xid_age"]),
			RelMinMXID: u32(t, m["relminmxid"]), MXIDAge: i32v(t, m["mxid_age"]), HeapBytesEst: *kI64(t, m["bytes"])})
	}
	l := facts.FreezeLimits{Status: facts.StatusOK}
	set := map[string]int64{}
	for _, m := range loadKsql(t, "freeze_"+node+"_sys_settings").rows {
		if v := kI64OrNil(m["setting"]); v != nil {
			set[*m["name"]] = *v
		}
	}
	l.Rows = []facts.FreezeLimit{{FreezeMaxAge: set["autovacuum_freeze_max_age"], MultiFreezeMaxAge: set["autovacuum_multixact_freeze_max_age"], FreezeTableAge: set["vacuum_freeze_table_age"]}}
	return d, tb, l
}

func TestFreezeText(t *testing.T) {
	t.Run("primary", func(t *testing.T) {
		d, tb, l := freezeFacts(t, "node1")
		c := v02Context("primary", "system", "local")
		c.Database = "test"
		rep := Freeze(c, d, tb, l, FreezeOptions{Limit: 20})
		assertGolden(t, "freeze_primary", rep)
		if rep.Verdict != rule.VerdictOK {
			t.Errorf("verdict = %s", rep.Verdict)
		}
	})
	// hand-made: the lab's ages are a few thousand; this one is past
	// autovacuum_freeze_max_age, seen from a standby
	t.Run("warn", func(t *testing.T) {
		_, _, l := freezeFacts(t, "node2")
		d := facts.FreezeDatabases{Status: facts.StatusOK, Rows: []facts.FrozenDatabase{
			{Datname: "esrep", DatFrozenXID: 1078, XIDAge: 5364, DatMinMXID: 1, AllowConn: true},
			{Datname: "test", DatFrozenXID: 1078, XIDAge: 210_000_000, DatMinMXID: 1, AllowConn: true},
		}}
		tb := facts.FreezeTables{Status: facts.StatusOK, Rows: []facts.FrozenTable{
			{Relation: "public.orders", Relkind: "r", RelFrozenXID: 1078, XIDAge: 210_000_000, RelMinMXID: 1, HeapBytesEst: 64684032},
		}}
		c := v02Context("standby", "system", "local")
		c.Database = "test"
		rep := Freeze(c, d, tb, l, FreezeOptions{Limit: 20})
		assertGolden(t, "freeze_warn", rep)
	})
}

// At the stop limit (FAIL), limits and tables not collected, a database
// that takes no connections, a hostile name, no database name in context.
func TestFreezeTextEdges(t *testing.T) {
	d := facts.FreezeDatabases{Status: facts.StatusOK, Rows: []facts.FrozenDatabase{
		{Datname: "template0", XIDAge: facts.XIDStopAge + 5},
		{Datname: "bad\x1b[31m", XIDAge: 1<<31 - 1, MXIDAge: 1<<31 - 1, AllowConn: true},
	}}
	rep := Freeze(v02Context("primary", "system", "local"), d, facts.FreezeTables{Status: facts.StatusSkipped, Reason: "timeout 57014: canceling statement due to statement timeout"},
		facts.FreezeLimits{Status: facts.StatusError, Reason: "XX000: boom"}, FreezeOptions{Limit: 20})
	assertGolden(t, "freeze_edges", rep)
	if rep.Verdict != rule.VerdictFAIL || len(rep.Findings) != 2 {
		t.Errorf("verdict=%s findings=%d", rep.Verdict, len(rep.Findings))
	}
}
