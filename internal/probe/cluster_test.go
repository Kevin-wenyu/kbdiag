package probe

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func TestParseRepmgrConf(t *testing.T) {
	// the lab's file (2026-10-07), trimmed, plus the shapes repmgr accepts
	conf := parseRepmgrConf(`# repmgr.conf
node_id=1
node_name='node1'
conninfo='host=192.168.105.10 user=esrep dbname=esrep port=54321 connect_timeout=10'
data_directory='/home/kingbase/cluster/install/kingbase/data'

failover='automatic'
synchronous='quorum'
reconnect_attempts=10   # trailing comment
  spaced  =  "double quoted # not a comment"
#synchronous='sync'
empty=
no_equals_line
=no_key
`)
	want := map[string]string{
		"node_id":            "1",
		"node_name":          "node1",
		"conninfo":           "host=192.168.105.10 user=esrep dbname=esrep port=54321 connect_timeout=10",
		"data_directory":     "/home/kingbase/cluster/install/kingbase/data",
		"failover":           "automatic",
		"synchronous":        "quorum",
		"reconnect_attempts": "10",
		"spaced":             "double quoted # not a comment",
		"empty":              "",
	}
	for k, v := range want {
		if conf[k] != v {
			t.Errorf("%s = %q, want %q", k, conf[k], v)
		}
	}
	if len(conf) != len(want) {
		t.Errorf("extra keys: %v", conf)
	}
	// repmgr's parser takes the = as optional
	if c := parseRepmgrConf("synchronous quorum\nnode_id\t2 # c\nfailover = 'manual'\n"); c["synchronous"] != "quorum" || c["node_id"] != "2" || c["failover"] != "manual" {
		t.Errorf("without =: %v", c)
	}
	// a later line wins, as in repmgr
	if c := parseRepmgrConf("synchronous='sync'\nsynchronous=async\n"); c["synchronous"] != "async" {
		t.Errorf("later line: %q", c["synchronous"])
	}
	// an unterminated quote keeps the raw value rather than guessing
	if c := parseRepmgrConf("synchronous='quorum\n"); c["synchronous"] != "'quorum" {
		t.Errorf("unterminated: %q", c["synchronous"])
	}
}

// Everything ClusterSync decides before it touches the database.
func TestClusterSyncGuards(t *testing.T) {
	primary := facts.Context{Role: "primary"}
	nodes := facts.ClusterNodes{Status: facts.StatusOK, Rows: []facts.ClusterNode{{NodeID: 1, NodeName: "node1", Type: "primary", IsLocal: true}}}
	cases := []struct {
		name   string
		c      facts.Context
		n      facts.ClusterNodes
		local  bool
		status facts.Status
	}{
		{"not a repmgr cluster", primary, facts.ClusterNodes{Status: facts.StatusNotApplicable}, true, facts.StatusNotApplicable},
		{"nodes not readable", primary, facts.ClusterNodes{Status: facts.StatusSkipped}, true, facts.StatusSkipped},
		{"standby", facts.Context{Role: "standby"}, nodes, true, facts.StatusNotApplicable},
		{"this node not identified", primary, facts.ClusterNodes{Status: facts.StatusOK, Rows: []facts.ClusterNode{{NodeID: 1}}}, true, facts.StatusSkipped},
		{"remote: the file is not here", primary, nodes, false, facts.StatusSkipped},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := ClusterSync(t.Context(), nil, c.c, c.n, c.local, false)
			if s.Status != c.status || s.Reason == "" || len(s.Rows) != 0 {
				t.Errorf("status=%s reason=%q rows=%d", s.Status, s.Reason, len(s.Rows))
			}
		})
	}
}

func TestClusterSyncFile(t *testing.T) {
	const lab = "node_id=1\ndata_directory='/kb/data'\nsynchronous='quorum'\n"
	files := map[string]string{}
	readFile = func(p string) ([]byte, error) {
		if s, ok := files[p]; ok {
			return []byte(s), nil
		}
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
	defer func() { readFile = os.ReadFile }()
	dir, names := sp("/kb/data"), sp("ANY 1( node2)")
	cases := []struct {
		name, file string
		dir        *string
		status     facts.Status
		reason     string
		mode       *string
	}{
		{"lab", lab, dir, facts.StatusOK, "", sp("quorum")},
		{"trailing slash on data_directory", "node_id=1\ndata_directory='/kb/data/'\nsynchronous=sync\n", dir, facts.StatusOK, "", sp("sync")},
		{"synchronous not set", "node_id=1\n", dir, facts.StatusOK, "", nil},
		{"another node's file", "node_id=2\nsynchronous='quorum'\n", dir, facts.StatusSkipped, "node_id 2", nil},
		{"another instance's file", "node_id=1\ndata_directory='/other'\n", dir, facts.StatusSkipped, "data_directory /other", nil},
		{"data_directory hidden: node_id still checked", lab, nil, facts.StatusOK, "", sp("quorum")},
		{"no file", "", dir, facts.StatusSkipped, "/kb/bin/../etc/repmgr.conf", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clear(files)
			if c.file != "" {
				files["/kb/bin/../etc/repmgr.conf"] = c.file
			}
			s := clusterSyncFile("/kb/bin/", 1, c.dir, names)
			if s.Status != c.status || !strings.Contains(s.Reason, c.reason) {
				t.Fatalf("status=%s reason=%q", s.Status, s.Reason)
			}
			if c.status != facts.StatusOK {
				return
			}
			r := s.Rows[0]
			if r.ConfPath != "/kb/bin/../etc/repmgr.conf" || r.StandbyNames != names || (r.Synchronous == nil) != (c.mode == nil) || (c.mode != nil && *r.Synchronous != *c.mode) {
				t.Errorf("row = %+v", r)
			}
		})
	}
}
func sp(s string) *string { return &s }

// The path comes from the database: only a small regular file is read.
func TestReadConf(t *testing.T) {
	dir := t.TempDir()
	ok := filepath.Join(dir, "repmgr.conf")
	if err := os.WriteFile(ok, []byte("synchronous=quorum\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, err := readConf(ok); err != nil || string(b) != "synchronous=quorum\n" {
		t.Errorf("regular: %q %v", b, err)
	}
	big := filepath.Join(dir, "big")
	if err := os.WriteFile(big, make([]byte, 1<<20+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readConf(big); err == nil || !strings.Contains(err.Error(), "1 MiB") {
		t.Errorf("big: %v", err)
	}
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip(err)
	}
	done := make(chan error, 1)
	go func() { _, err := readConf(fifo); done <- err }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("fifo: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("readConf blocked on a FIFO")
	}
	if _, err := readConf(dir); err == nil {
		t.Error("a directory was read")
	}
}
