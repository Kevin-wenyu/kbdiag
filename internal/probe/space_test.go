package probe

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func TestSpaceDisk(t *testing.T) {
	dir := "/data"
	info := facts.InstInfo{Status: facts.StatusOK, Rows: []facts.Info{{DataDirectory: &dir}}}
	loc, empty := "/ts/one", ""
	ts := facts.SpaceTablespaces{Status: facts.StatusOK, Rows: []facts.Tablespace{
		{Name: "sys_default", Location: &empty}, {Name: "sys_global"}, {Name: "one", Location: &loc},
	}}
	orig, origDev := statfs, deviceOf
	defer func() { statfs, deviceOf = orig, origDev }()
	deviceOf = func(p string) (string, error) { return "dev " + p, nil }
	var statted []string
	fail := ""
	statfs = func(p string) (facts.Disk, error) {
		statted = append(statted, p)
		if p == fail {
			return facts.Disk{}, errors.New("statfs " + p + ": permission denied")
		}
		return facts.Disk{TotalBytes: 100, FSID: p}, nil
	}

	d := SpaceDisk(info, ts, true, false)
	if d.Status != facts.StatusOK || len(d.Rows) != 3 || strings.Join(statted, ",") != "/data,/data/sys_wal,/ts/one" {
		t.Fatalf("status=%s rows=%+v statted=%v", d.Status, d.Rows, statted)
	}
	if d.Rows[1].Kind != "wal" || d.Rows[2].Kind != "tablespace" || d.Rows[2].Path != loc {
		t.Errorf("rows = %+v", d.Rows)
	}

	// tablespaces not collected: data_directory and WAL are still checked
	statted = nil
	if d := SpaceDisk(info, facts.SpaceTablespaces{Status: facts.StatusSkipped}, true, false); d.Status != facts.StatusOK || len(d.Rows) != 2 {
		t.Errorf("status=%s rows=%d", d.Status, len(d.Rows))
	}

	// a directory that cannot be statted fails the probe, never drops a row
	fail = "/ts/one"
	if d := SpaceDisk(info, ts, true, false); d.Status != facts.StatusError || !strings.Contains(d.Reason, "/ts/one") || len(d.Rows) != 0 {
		t.Errorf("status=%s reason=%q rows=%d", d.Status, d.Reason, len(d.Rows))
	}

	// remote: same rules as inst.disk, nothing statted
	statted = nil
	if d := SpaceDisk(info, ts, false, false); d.Status != facts.StatusNotApplicable || len(statted) != 0 {
		t.Errorf("status=%s statted=%v", d.Status, statted)
	}
}
