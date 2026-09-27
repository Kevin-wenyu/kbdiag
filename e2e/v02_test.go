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
