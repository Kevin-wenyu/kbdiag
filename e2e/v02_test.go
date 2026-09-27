//go:build vm

package e2e

import (
	"strconv"
	"strings"
	"testing"
)

// v0.2 commands (plan 2026-09-27). Written in the cloud session without a
// VM: every assertion here is listed in the plan's stage 13 checklist and
// has not run yet.

var (
	spaceDiskColumns   = []string{"path_kind", "path", "total_bytes", "used_bytes", "avail_bytes"}
	spaceWALColumns    = []string{"files", "bytes", "max_wal_size_bytes", "wal_keep_bytes", "wal_segment_bytes"}
	spaceTblspcColumns = []string{"spcname", "location", "size_bytes"}
	roEnv              = []string{"PGPASSWORD=kbdiag_ro_T3st"}
	roArgs             = []string{"--host", "127.0.0.1", "-U", "kbdiag_ro"}
)

// num reads a JSON number cell.
func num(t *testing.T, v any) float64 {
	t.Helper()
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("not a number: %v", v)
	}
	return f
}

func TestSpace(t *testing.T) {
	r, code := kbdiag(t, nil, "space")
	if r.Verdict != "OK" || code != 0 || len(r.Findings) != 0 {
		t.Errorf("verdict=%s exit=%d findings=%v", r.Verdict, code, r.Findings)
	}
	// WAL: same count and size as ksql, within what a checkpoint may add or recycle
	w := okProbe(t, r, "space.wal", spaceWALColumns).rowsOf()[0]
	want := strings.Split(ksql(t, "select count(*) || ' ' || sum(size) from sys_ls_waldir()"), " ")
	files, _ := strconv.ParseFloat(want[0], 64)
	if d := num(t, w["files"]) - files; d > 2 || d < -2 {
		t.Errorf("wal files = %v, ksql %v", w["files"], files)
	}
	if num(t, w["max_wal_size_bytes"]) != 1024<<20 || num(t, w["wal_keep_bytes"]) != 512*16<<20 {
		t.Errorf("wal settings = %v", w)
	}
	// every tablespace, with a size for system
	ts := okProbe(t, r, "space.tablespaces", spaceTblspcColumns)
	if got := len(ts.Rows); strconv.Itoa(got) != ksql(t, "select count(*) from sys_tablespace") {
		t.Errorf("tablespaces = %d", got)
	}
	for _, row := range ts.rowsOf() {
		if row["size_bytes"] == nil {
			t.Errorf("system cannot see the size of %v", row["spcname"])
		}
	}
	// the socket run reads the disk: data_directory and sys_wal share one filesystem on the lab
	disk := okProbe(t, r, "space.disk", spaceDiskColumns).rowsOf()
	if len(disk) < 2 || disk[0]["path_kind"] != "data_directory" || disk[1]["path_kind"] != "wal" {
		t.Fatalf("disk rows = %v", disk)
	}
	out, _ := kbdiagText(t, nil, "space")
	if !strings.Contains(out, "\ndisk: 1 filesystem\n") || !textRow(out, "data_directory,", "wal") {
		t.Errorf("text does not merge data_directory and wal:\n%s", out)
	}

	t.Run("kbdiag_ro", func(t *testing.T) {
		r, code := kbdiag(t, roEnv, append([]string{"space"}, roArgs...)...)
		if p := r.Data["space.wal"]; p.Status != "skipped" || p.Reason == nil || !strings.Contains(*p.Reason, "sys_ls_waldir") {
			t.Errorf("space.wal = %+v", p)
		}
		if p := r.Data["space.disk"]; p.Status != "not_applicable" {
			t.Errorf("space.disk over TCP = %+v", p)
		}
		hidden := 0
		for _, row := range okProbe(t, r, "space.tablespaces", spaceTblspcColumns).rowsOf() {
			if row["size_bytes"] == nil {
				hidden++
			}
		}
		if hidden == 0 || len(r.Redacted) != 1 || r.Redacted[0].RowsAffected != hidden {
			t.Errorf("hidden=%d redacted=%+v", hidden, r.Redacted)
		}
		if r.Verdict != "UNKNOWN" || code != 3 {
			t.Errorf("verdict=%s exit=%d", r.Verdict, code)
		}
	})
}

var (
	freezeDBColumns    = []string{"datname", "datfrozenxid", "xid_age", "datminmxid", "mxid_age", "datallowconn"}
	freezeTableColumns = []string{"relation", "relkind", "relfrozenxid", "xid_age", "relminmxid", "mxid_age", "heap_bytes_est"}
	freezeLimitColumns = []string{"autovacuum_freeze_max_age", "autovacuum_multixact_freeze_max_age", "vacuum_freeze_table_age"}
)

// The lab's ages are a few thousand: only the negative case and the values
// can be checked; WARN and FAIL are L1/L2 only.
func TestFreeze(t *testing.T) {
	r, code := kbdiag(t, nil, "freeze", "--limit", "0")
	if r.Verdict != "OK" || code != 0 || len(r.Findings) != 0 {
		t.Errorf("verdict=%s exit=%d findings=%v", r.Verdict, code, r.Findings)
	}
	okProbe(t, r, "freeze.limits", freezeLimitColumns)
	dbs := okProbe(t, r, "freeze.databases", freezeDBColumns)
	want := ksql(t, "select string_agg(datname || ':' || age(datfrozenxid), ',' order by datname) from sys_database")
	// ages move while the test runs: compare the count only
	if len(dbs.Rows) != len(strings.Split(want, ",")) {
		t.Errorf("databases %v, ksql %q", dbs.Rows, want)
	}
	tables := okProbe(t, r, "freeze.tables", freezeTableColumns)
	// independent of the probe's filter: relfrozenxid 0 reads as age 2147483647
	n := ksql(t, "select count(*) from sys_class where relkind in ('r','m','t') and age(relfrozenxid) < 2147483647")
	if strconv.Itoa(len(tables.Rows)) != n {
		t.Errorf("tables = %d, ksql %s", len(tables.Rows), n)
	}
	for _, row := range tables.rowsOf() {
		if num(t, row["xid_age"]) >= 1<<31-1 {
			t.Errorf("relfrozenxid 0 leaked in: %v", row)
		}
		if !strings.Contains(str(row["relation"]), ".") {
			t.Errorf("relation not qualified: %v", row["relation"])
		}
	}
	if r.Context.Database != "test" {
		t.Errorf("context.database = %q", r.Context.Database)
	}
}

var (
	vacuumTableColumns    = []string{"schemaname", "relname", "n_live_tup", "n_dead_tup", "reltuples", "reloptions", "last_vacuum_age_s", "last_autovacuum_age_s", "vacuum_count", "autovacuum_count"}
	vacuumProgressColumns = []string{"pid", "datname", "relation", "phase", "heap_blks_total", "heap_blks_scanned", "is_autovacuum", "xact_age_s"}
	vacuumSettingColumns  = []string{"autovacuum", "track_counts", "autovacuum_vacuum_threshold", "autovacuum_vacuum_scale_factor", "autovacuum_naptime_s", "autovacuum_max_workers"}
)

func TestVacuum(t *testing.T) {
	r, code := kbdiag(t, nil, "vacuum", "--limit", "0")
	okProbe(t, r, "vacuum.settings", vacuumSettingColumns)
	if role == "standby" {
		for _, id := range []string{"vacuum.tables", "vacuum.progress"} {
			if p := r.Data[id]; p.Status != "not_applicable" {
				t.Errorf("%s on a standby = %+v", id, p)
			}
		}
		if r.Verdict != "OK" || code != 0 {
			t.Errorf("verdict=%s exit=%d", r.Verdict, code)
		}
		return
	}
	tables := okProbe(t, r, "vacuum.tables", vacuumTableColumns)
	if n := ksql(t, "select count(*) from sys_stat_user_tables"); strconv.Itoa(len(tables.Rows)) != n {
		t.Errorf("tables = %d, ksql %s", len(tables.Rows), n)
	}
	okProbe(t, r, "vacuum.progress", vacuumProgressColumns)
	if r.Verdict != "OK" || code != 0 {
		t.Errorf("clean lab: verdict=%s exit=%d findings=%v", r.Verdict, code, r.Findings)
	}

	t.Run("autovacuum off for a table past its threshold", func(t *testing.T) {
		inject(t, "dead_tuples")
		r, code := kbdiag(t, nil, "vacuum")
		if got := findings(r, "vacuum.table_disabled", "relname", "kbdiag_inj_dead"); len(got) != 1 || got[0] != "WARN" || code != 1 {
			t.Errorf("findings=%v exit=%d", got, code)
		}
		out, _ := kbdiagText(t, nil, "vacuum")
		if !textRow(out, "public.kbdiag_inj_dead", "2050", "yes", "off") {
			t.Errorf("text does not list the table as due with autovacuum off:\n%s", out)
		}
	})
}

var (
	archiveStatusColumns = []string{"archive_mode", "archive_command", "archive_timeout_s", "archived_count", "last_archived_wal", "last_archived_time", "last_archived_age_s",
		"failed_count", "last_failed_wal", "last_failed_time", "last_failed_age_s", "stats_reset"}
	archiveReadyColumns = []string{"ready", "done", "oldest_ready_age_s"}
)

// The lab's archive_command fails on node1 (stage 0): no injection needed.
// On node2 it succeeded last, so both outcomes are checked against ksql.
func TestArchive(t *testing.T) {
	// the archiver retries every minute: sample before and after kbdiag and
	// accept either
	failingSQL := "select coalesce(last_failed_time > last_archived_time or (last_archived_time is null and last_failed_time is not null), false) from sys_stat_archiver"
	readySQL := "select count(*) from sys_ls_archive_statusdir() where name like '%.ready'"
	failBefore, readyBefore := ksql(t, failingSQL), ksql(t, readySQL)
	r, code := kbdiag(t, nil, "archive")
	failAfter, readyAfter := ksql(t, failingSQL), ksql(t, readySQL)
	a := okProbe(t, r, "archive.status", archiveStatusColumns).rowsOf()[0]
	if a["archive_mode"] != ksql(t, "show archive_mode") {
		t.Errorf("archive_mode = %v", a["archive_mode"])
	}
	if failBefore != failAfter {
		t.Skipf("the archiver changed state during the run (%s -> %s)", failBefore, failAfter)
	}
	failing := failAfter
	wantWARN := failing == "t" && a["archive_mode"] != "off" && a["archive_command"] != nil && a["archive_command"] != "" && (role == "primary" || a["archive_mode"] == "always")
	if got := len(r.Findings) == 1 && r.Findings[0].ID == "archive.failing" && r.Findings[0].Level == "WARN"; got != wantWARN || (wantWARN && code != 1) {
		t.Errorf("failing=%s mode=%v findings=%+v exit=%d", failing, a["archive_mode"], r.Findings, code)
	}
	ready := okProbe(t, r, "archive.ready", archiveReadyColumns).rowsOf()[0]
	if n := str(ready["ready"]); n != readyBefore && n != readyAfter {
		t.Errorf("ready = %v", ready["ready"])
	}

	t.Run("kbdiag_ro", func(t *testing.T) {
		r, code := kbdiag(t, roEnv, append([]string{"archive"}, roArgs...)...)
		okProbe(t, r, "archive.status", archiveStatusColumns)
		if p := r.Data["archive.ready"]; p.Status != "skipped" {
			t.Errorf("archive.ready = %+v", p)
		}
		if (r.Verdict == "WARN") != wantWARN || code == 3 {
			t.Errorf("verdict=%s exit=%d: archive.ready must not make it UNKNOWN", r.Verdict, code)
		}
	})
}

var paramColumns = []string{"name", "setting", "unit", "source", "sourcefile", "sourceline", "boot_val", "reset_val", "context", "pending_restart"}

func TestParams(t *testing.T) {
	r, code := kbdiag(t, nil, "params")
	p := okProbe(t, r, "params.changed", paramColumns)
	// independent of the probe's filter: set in es_rep.conf on the lab, and
	// fixed at initdb
	if row := p.row("name", "max_connections"); row == nil || row["source"] != "configuration file" || row["setting"] != ksql(t, "show max_connections") {
		t.Errorf("max_connections row = %v", row)
	}
	for _, name := range []string{"block_size", "data_checksums", "lock_timeout"} {
		if p.row("name", name) != nil {
			t.Errorf("%s is listed: not set by anyone (or set by this connection)", name)
		}
	}
	if r.Verdict != "OK" || code != 0 {
		t.Errorf("clean lab: verdict=%s exit=%d", r.Verdict, code)
	}

	if role == "primary" {
		t.Run("pending restart", func(t *testing.T) {
			inject(t, "pending_restart")
			r, code := kbdiag(t, nil, "params")
			if got := findings(r, "params.pending_restart", "name", "max_files_per_process"); len(got) != 1 || got[0] != "WARN" || code != 1 {
				t.Errorf("findings=%v exit=%d", got, code)
			}
			out, _ := kbdiagText(t, nil, "params")
			if !strings.Contains(out, "\npending restart: 1\n") || !textRow(out, "max_files_per_process", "kingbase.auto.conf:") {
				t.Errorf("text:\n%s", out)
			}
		})
	}

	t.Run("kbdiag_ro", func(t *testing.T) {
		r, code := kbdiag(t, roEnv, append([]string{"params"}, roArgs...)...)
		okProbe(t, r, "params.changed", paramColumns)
		if len(r.Redacted) != 2 || r.Redacted[0].Field != "sourcefile" || r.Verdict != "UNKNOWN" || code != 3 {
			t.Errorf("redacted=%+v verdict=%s exit=%d", r.Redacted, r.Verdict, code)
		}
	})
}
