package scenario

import (
	"testing"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// tableFacts rebuilds table from the stage 0 captures of the test table
// public.kbdiag_inj_tbl (20000 rows, a quarter deleted, a primary key, an
// index and a TOAST table). The capture has no now(): the collection time
// is set 60s after its last analyze.
func tableFacts(t *testing.T, node string) (facts.Context, facts.TableInfos, facts.TableStats, facts.TableIndexes) {
	t.Helper()
	cl := loadKsql(t, "table_"+node+"_sys_class").rows[0]
	sz := loadKsql(t, "table_"+node+"_sys_sizes").rows[0]
	xid, mxid := int32(*kI64(t, cl["frozen_age"])), int32(0)
	// the sizes capture has two columns named pg_total_relation_size: the
	// loader keeps the last (the TOAST table's)
	info := facts.TableInfo{OID: uint32(*kI64(t, cl["oid"])), Schemaname: "public", Relname: *cl["relname"], Relkind: *cl["relkind"], Relpersistence: *cl["relpersistence"],
		Reltuples: float32(*kF64(t, cl["reltuples"])), Relpages: int32(*kI64(t, cl["relpages"])), TotalBytes: 3555328, TableBytes: *kI64(t, sz["pg_relation_size"]),
		IndexBytes: *kI64(t, sz["pg_indexes_size"]), ToastBytes: kI64(t, sz["pg_total_relation_size"]), XIDAge: &xid, MXIDAge: &mxid}
	at := time.Date(2026, 9, 27, 22, 42, 52, 0, cst)
	c := v02Context("primary", "system", "local")
	if node == "node2" {
		c.Role = "standby"
	}
	c.CollectedAt, c.Database = at, "test"
	stats := facts.TableStats{Status: facts.StatusNotApplicable, Reason: "standby: table statistics are local to each node and stay 0 here; run on the primary"}
	if c.Role == "primary" {
		m := loadKsql(t, "table_"+node+"_sys_stat").rows[0]
		io := loadKsql(t, "table_"+node+"_sys_iostat").rows[0]
		analyzed := at.Sub(*kTime(t, m["last_analyze"])).Seconds()
		stats = facts.TableStats{Status: facts.StatusOK, Rows: []facts.TableStat{{NLiveTup: *kI64(t, m["n_live_tup"]), NDeadTup: *kI64(t, m["n_dead_tup"]),
			NModSinceAnalyze: *kI64(t, m["n_mod_since_analyze"]), LastAnalyzeAgeS: &analyzed, VacuumCount: *kI64(t, m["vacuum_count"]), AutovacuumCount: *kI64(t, m["autovacuum_count"]),
			AnalyzeCount: *kI64(t, m["analyze_count"]), AutoanalyzeN: *kI64(t, m["autoanalyze_count"]), SeqScan: *kI64(t, m["seq_scan"]), SeqTupRead: *kI64(t, m["seq_tup_read"]),
			IdxScan: kI64(t, m["idx_scan"]), IdxTupFetch: kI64(t, m["idx_tup_fetch"]), NTupIns: *kI64(t, m["n_tup_ins"]), NTupUpd: *kI64(t, m["n_tup_upd"]),
			NTupDel: *kI64(t, m["n_tup_del"]), NTupHotUpd: *kI64(t, m["n_tup_hot_upd"]), HeapBlksRead: kI64(t, io["heap_blks_read"]), HeapBlksHit: kI64(t, io["heap_blks_hit"]),
			IdxBlksRead: kI64(t, io["idx_blks_read"]), IdxBlksHit: kI64(t, io["idx_blks_hit"])}}}
	}
	scans := map[string]*int64{}
	for _, m := range loadKsql(t, "table_"+node+"_sys_idxstat").rows {
		if c.Role == "primary" {
			scans[*m["indexrelname"]] = kI64(t, m["idx_scan"])
		}
	}
	ix := facts.TableIndexes{Status: facts.StatusOK}
	for _, m := range loadKsql(t, "table_"+node+"_sys_indexes").rows {
		def := *m["def"]
		name := def[len("CREATE INDEX "):]
		if kBool(m["indisunique"]) {
			name = def[len("CREATE UNIQUE INDEX "):]
		}
		for i := range name {
			if name[i] == ' ' {
				name = name[:i]
				break
			}
		}
		ix.Rows = append(ix.Rows, facts.TableIndex{Name: name, Definition: def, Bytes: *kI64(t, m["size"]), IsUnique: kBool(m["indisunique"]),
			IsPrimary: kBool(m["indisprimary"]), IsValid: kBool(m["indisvalid"]), IdxScan: scans[name]})
	}
	// the probe orders by name
	if len(ix.Rows) == 2 && ix.Rows[0].Name > ix.Rows[1].Name {
		ix.Rows[0], ix.Rows[1] = ix.Rows[1], ix.Rows[0]
	}
	return c, facts.TableInfos{Status: facts.StatusOK, Rows: []facts.TableInfo{info}}, stats, ix
}

func freezeLimitsLab() facts.FreezeLimits {
	return facts.FreezeLimits{Status: facts.StatusOK, Rows: []facts.FreezeLimit{{FreezeMaxAge: 200_000_000, MultiFreezeMaxAge: 400_000_000, FreezeTableAge: 150_000_000}}}
}

func vacuumSettingsLab() facts.VacuumSettings {
	return facts.VacuumSettings{Status: facts.StatusOK, Rows: []facts.VacuumSetting{{Autovacuum: "on", TrackCounts: "on", Threshold: 50, ScaleFactor: 0.2, NaptimeS: 60, MaxWorkers: 3}}}
}

func TestTableText(t *testing.T) {
	for _, node := range []string{"node1", "node2"} {
		golden := map[string]string{"node1": "table_primary", "node2": "table_standby"}[node]
		t.Run(golden, func(t *testing.T) {
			c, i, s, x := tableFacts(t, node)
			rep, found := Table(c, i, s, x, freezeLimitsLab(), vacuumSettingsLab())
			assertGolden(t, golden, rep)
			if !found || rep.Verdict != rule.VerdictOK {
				t.Errorf("found=%v verdict=%s", found, rep.Verdict)
			}
		})
	}
	t.Run("missing", func(t *testing.T) {
		c, _, _, _ := tableFacts(t, "node1")
		rep, found := Table(c, facts.TableInfos{Status: facts.StatusOK}, facts.TableStats{Status: facts.StatusOK}, facts.TableIndexes{Status: facts.StatusOK}, freezeLimitsLab(), vacuumSettingsLab())
		assertGolden(t, "table_missing", rep)
		if found || rep.Verdict != rule.VerdictUNKNOWN {
			t.Errorf("found=%v verdict=%s", found, rep.Verdict)
		}
	})
}

// An index given as the name; an unlogged table past both lines with an
// invalid index and a hostile name; statistics not collected.
func TestTableTextEdges(t *testing.T) {
	c, _, _, _ := tableFacts(t, "node1")
	idx := facts.TableInfos{Status: facts.StatusOK, Rows: []facts.TableInfo{{Schemaname: "public", Relname: "orders_pkey", Relkind: "i"}}}
	rep, found := Table(c, idx, facts.TableStats{Status: facts.StatusNotApplicable}, facts.TableIndexes{Status: facts.StatusNotApplicable}, freezeLimitsLab(), vacuumSettingsLab())
	assertGolden(t, "table_index", rep)
	if found || rep.Verdict != rule.VerdictUNKNOWN {
		t.Errorf("found=%v verdict=%s", found, rep.Verdict)
	}

	old, mx := int32(250_000_000), int32(3)
	info := facts.TableInfos{Status: facts.StatusOK, Rows: []facts.TableInfo{{Schemaname: "app", Relname: "Q\x1b[2J", Relkind: "r", Relpersistence: "u", Reltuples: 1e6,
		TotalBytes: 5 << 30, TableBytes: 4 << 30, IndexBytes: 1 << 30, Reloptions: []string{"autovacuum_enabled=false", "fillfactor=70"}, XIDAge: &old, MXIDAge: &mx}}}
	st := facts.TableStats{Status: facts.StatusOK, Rows: []facts.TableStat{{NLiveTup: 1e6, NDeadTup: 600_000, LastAutovacuumAgeS: f64(8 * 86400), AutovacuumCount: 3}}}
	ix := facts.TableIndexes{Status: facts.StatusOK, Rows: []facts.TableIndex{{Name: "q_idx", Definition: "CREATE INDEX q_idx ON app.\"Q\" USING btree (a)", Bytes: 1 << 30, IsValid: false}}}
	rep, _ = Table(c, info, st, ix, freezeLimitsLab(), vacuumSettingsLab())
	assertGolden(t, "table_edges", rep)
	if rep.Verdict != rule.VerdictWARN || len(rep.Findings) != 2 {
		t.Errorf("verdict=%s findings=%+v", rep.Verdict, rep.Findings)
	}
	rep, _ = Table(c, info, facts.TableStats{Status: facts.StatusSkipped, Reason: "timeout 57014: canceling statement due to statement timeout"}, ix, freezeLimitsLab(), vacuumSettingsLab())
	if rep.Verdict != rule.VerdictWARN {
		t.Errorf("stats skipped, old table: %s", rep.Verdict)
	}
}
