package probe

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func TestVersionNumber(t *testing.T) {
	cases := map[string]string{
		"KingbaseES V008R006C009B0014 on x86_64-pc-linux-gnu, compiled by gcc (GCC) 4.1.2, 64-bit": "V008R006C009B0014",
		"KingbaseES V008R006C009B0014": "V008R006C009B0014",
		"KingbaseES":                   "KingbaseES",
		"":                             "",
		"PostgreSQL 12.1 on x86_64":    "PostgreSQL 12.1 on x86_64",
	}
	for in, want := range cases {
		if got := versionNumber(in); got != want {
			t.Errorf("versionNumber(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInstUpstreamOnPrimary(t *testing.T) {
	// Never reaches the connection: a primary has no upstream.
	u := InstUpstream(t.Context(), nil, facts.Context{Role: "primary"})
	if u.Status != facts.StatusNotApplicable || u.Reason != "primary" || len(u.Rows) != 0 {
		t.Errorf("upstream = %+v", u)
	}
}

func TestInstDisk(t *testing.T) {
	dir := "/home/kingbase/cluster/install/kingbase/data"
	info := facts.InstInfo{Status: facts.StatusOK, Rows: []facts.Info{{DataDirectory: &dir}}}
	node1 := facts.Disk{TotalBytes: 213452304384, UsedBytes: 15089946624, AvailBytes: 198362357760}
	statErr := errors.New("stat /home/kingbase/cluster/install/kingbase/data: no such file or directory")
	cases := []struct {
		name             string
		info             facts.InstInfo
		socket, loopback bool
		fs               facts.Disk
		fsErr            error
		want             facts.Status
		reason           string
		statted          bool
	}{
		{"socket", info, true, false, node1, nil, facts.StatusOK, "", true},
		{"localhost", info, false, true, node1, nil, facts.StatusOK, "", true},
		{"remote host is never statted", info, false, false, node1, nil, facts.StatusNotApplicable, "remote connection", false},
		{"localhost forwarded elsewhere", info, false, true, facts.Disk{}, statErr, facts.StatusNotApplicable, "data_directory 在本机不可访问", true},
		{"socket but stat fails", info, true, false, facts.Disk{}, statErr, facts.StatusError, "no such file", true},
		{"data_directory hidden", facts.InstInfo{Status: facts.StatusOK, Rows: []facts.Info{{}}}, true, false, node1, nil, facts.StatusSkipped, "insufficient_privilege", false},
		{"inst.info not collected", facts.InstInfo{Status: facts.StatusError}, true, false, node1, nil, facts.StatusSkipped, "inst.info", false},
		{"remote wins over missing info", facts.InstInfo{Status: facts.StatusError}, false, false, node1, nil, facts.StatusNotApplicable, "remote connection", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			statted := false
			orig := statfs
			statfs = func(p string) (facts.Disk, error) {
				statted = true
				if p != dir {
					t.Errorf("statfs(%q)", p)
				}
				return c.fs, c.fsErr
			}
			defer func() { statfs = orig }()
			d := InstDisk(c.info, c.socket, c.loopback)
			if d.Status != c.want || !strings.Contains(d.Reason, c.reason) || statted != c.statted {
				t.Fatalf("status=%s reason=%q statted=%v", d.Status, d.Reason, statted)
			}
			var want []facts.Disk
			if c.want == facts.StatusOK {
				want = []facts.Disk{c.fs}
			}
			if !reflect.DeepEqual(d.Rows, want) {
				t.Errorf("rows = %+v, want %+v", d.Rows, want)
			}
		})
	}
}

// The real statfs on a directory that exists: numbers must be consistent.
func TestStatfsOS(t *testing.T) {
	d, err := statfsOS(t.TempDir())
	if err != nil {
		t.Skip("statfs not supported here:", err)
	}
	if d.TotalBytes == 0 || d.UsedBytes > d.TotalBytes || d.AvailBytes > d.TotalBytes || d.UsedBytes+d.AvailBytes > d.TotalBytes {
		t.Errorf("disk = %+v", d)
	}
	if _, err := statfsOS("/nonexistent/kbdiag"); err == nil {
		t.Error("statfs of a missing path succeeded")
	}
}
