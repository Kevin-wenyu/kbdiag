package rule

import (
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func labNodes(localID int32) facts.ClusterNodes {
	one := int32(1)
	return facts.ClusterNodes{Status: facts.StatusOK, Rows: []facts.ClusterNode{
		{NodeID: 1, NodeName: "node1", Type: "primary", Active: true, IsLocal: localID == 1},
		{NodeID: 2, NodeName: "node2", Type: "standby", UpstreamNodeID: &one, Active: true, IsLocal: localID == 2},
	}}
}

func downs(names ...string) facts.InstDownstreams {
	d := facts.InstDownstreams{Status: facts.StatusOK}
	for _, n := range names {
		n := n
		d.Rows = append(d.Rows, facts.Downstream{ApplicationName: &n, State: sp("streaming")})
	}
	return d
}

func TestCluster(t *testing.T) {
	na := facts.InstDownstreams{Status: facts.StatusNotApplicable}
	cases := []struct {
		name    string
		n       facts.ClusterNodes
		d       facts.InstDownstreams
		role    string
		verdict Verdict
		ids     string
	}{
		{"lab primary", labNodes(1), downs("node2"), "primary", VerdictOK, ""},
		{"lab standby", labNodes(2), na, "standby", VerdictOK, ""},
		{"standby paused: not attached (stage 0)", labNodes(1), downs(), "primary", VerdictWARN, "cluster.detached"},
		{"names compare case-insensitively", labNodes(1), downs("NODE2"), "primary", VerdictOK, ""},
		{"repmgr says primary, the database is a standby", labNodes(1), na, "standby", VerdictWARN, "cluster.role_mismatch"},
		{"repmgr says standby, the database is a primary", labNodes(2), downs(), "primary", VerdictWARN, "cluster.role_mismatch"},
		{"this node not identified", labNodes(0), downs("node2"), "primary", VerdictUNKNOWN, ""},
		{"downstreams not collected", labNodes(1), facts.InstDownstreams{Status: facts.StatusError}, "primary", VerdictUNKNOWN, ""},
		{"not a repmgr cluster", facts.ClusterNodes{Status: facts.StatusNotApplicable}, downs(), "primary", VerdictOK, ""},
		{"no permission", facts.ClusterNodes{Status: facts.StatusSkipped}, downs(), "primary", VerdictUNKNOWN, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Cluster(c.n, c.d, syncNA, c.role)
			var ids []string
			for _, f := range r.Findings {
				ids = append(ids, f.ID)
				if f.Level != LevelWARN {
					t.Errorf("level %s", f.Level)
				}
			}
			if r.Verdict != c.verdict || strings.Join(ids, ",") != c.ids {
				t.Errorf("verdict=%s ids=%v", r.Verdict, ids)
			}
		})
	}
}

func TestClusterInactiveAndTwoPrimaries(t *testing.T) {
	n := labNodes(1)
	n.Rows[1].Active = false
	r := Cluster(n, downs(), syncNA, "primary")
	if len(r.Findings) != 1 || r.Findings[0].ID != "cluster.inactive" {
		t.Errorf("inactive standby: %+v (an inactive node is not also reported detached)", r.Findings)
	}
	n = labNodes(1)
	n.Rows[1].Type = "primary"
	n.Rows[1].UpstreamNodeID = nil
	r = Cluster(n, downs(), syncNA, "primary")
	var ids []string
	for _, f := range r.Findings {
		ids = append(ids, f.ID)
	}
	if strings.Join(ids, ",") != "cluster.primaries" {
		t.Errorf("two primaries: %v", ids)
	}
}

func TestClusterCascadeAndBoth(t *testing.T) {
	// node3 follows node2 (a cascade): it is not the primary's to attach
	n := labNodes(1)
	two := int32(2)
	n.Rows = append(n.Rows, facts.ClusterNode{NodeID: 3, NodeName: "node3", Type: "standby", UpstreamNodeID: &two, Active: true})
	if r := Cluster(n, downs("node2"), syncNA, "primary"); len(r.Findings) != 0 || r.Verdict != VerdictOK {
		t.Errorf("cascade: %s %+v", r.Verdict, r.Findings)
	}
	// two primaries and a missing standby: both reported
	n = labNodes(1)
	n.Rows = append(n.Rows, facts.ClusterNode{NodeID: 4, NodeName: "node4", Type: "primary", Active: true})
	r := Cluster(n, downs(), syncNA, "primary")
	var ids []string
	for _, f := range r.Findings {
		ids = append(ids, f.ID)
	}
	if strings.Join(ids, ",") != "cluster.primaries,cluster.detached" {
		t.Errorf("ids = %v", ids)
	}
	// a local witness: nothing to compare, and not UNKNOWN
	n = labNodes(0)
	n.Rows = append(n.Rows, facts.ClusterNode{NodeID: 5, NodeName: "witness", Type: "witness", Active: true, IsLocal: true})
	if r := Cluster(n, downs(), syncNA, "primary"); r.Verdict != VerdictOK {
		t.Errorf("witness: %s %+v", r.Verdict, r.Findings)
	}
}

var syncNA = facts.ClusterSyncs{Status: facts.StatusNotApplicable}

func syncOf(mode, names *string) facts.ClusterSyncs {
	return facts.ClusterSyncs{Status: facts.StatusOK, Rows: []facts.ClusterSync{{ConfPath: "/kb/etc/repmgr.conf", Synchronous: mode, StandbyNames: names}}}
}

func TestClusterSyncDegraded(t *testing.T) {
	empty := sp("")
	cases := []struct {
		name    string
		s       facts.ClusterSyncs
		d       facts.InstDownstreams
		role    string
		verdict Verdict
		ids     string
		symptom string
	}{
		{"lab: quorum and the list set", syncOf(sp("quorum"), sp("ANY 1( node2)")), downs("node2"), "primary", VerdictOK, "", ""},
		{"standby away: repmgrd degraded (lab inj-slot)", syncOf(sp("quorum"), empty), downs(), "primary", VerdictWARN, "cluster.detached,cluster.sync_degraded", "because node2 is not attached"},
		{"oracle mode: empty list reads NULL", syncOf(sp("quorum"), nil), downs(), "primary", VerdictWARN, "cluster.detached,cluster.sync_degraded", "can lose committed transactions"},
		{"standby back, list still empty: not restored", syncOf(sp("sync"), empty), downs("node2"), "primary", VerdictWARN, "cluster.sync_degraded", "has not switched back yet"},
		{"blank list counts as empty", syncOf(sp("all"), sp("  ")), downs("node2"), "primary", VerdictWARN, "cluster.sync_degraded", ""},
		{"mode case-insensitive", syncOf(sp("QUORUM"), empty), downs("node2"), "primary", VerdictWARN, "cluster.sync_degraded", ""},
		{"configured async", syncOf(sp("async"), empty), downs("node2"), "primary", VerdictOK, "", ""},
		{"custom: the list is the user's", syncOf(sp("custom"), empty), downs("node2"), "primary", VerdictOK, "", ""},
		{"unknown mode not judged", syncOf(sp("bogus"), empty), downs("node2"), "primary", VerdictOK, "", ""},
		{"synchronous not set in the file", syncOf(nil, empty), downs("node2"), "primary", VerdictOK, "", ""},
		{"downstreams not collected: still judged, cause unknown", syncOf(sp("quorum"), empty), facts.InstDownstreams{Status: facts.StatusError}, "primary", VerdictWARN, "cluster.sync_degraded", "while a synchronous standby is away"},
		{"file not read (remote): UNKNOWN", facts.ClusterSyncs{Status: facts.StatusSkipped, Reason: "remote connection"}, downs("node2"), "primary", VerdictUNKNOWN, "", ""},
		{"file not read, other WARN stands", facts.ClusterSyncs{Status: facts.StatusSkipped}, downs(), "primary", VerdictWARN, "cluster.detached", ""},
		{"standby: not applicable", syncNA, facts.InstDownstreams{Status: facts.StatusNotApplicable}, "standby", VerdictOK, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			local := int32(1)
			if c.role == "standby" {
				local = 2
			}
			r := Cluster(labNodes(local), c.d, c.s, c.role)
			var ids []string
			for _, f := range r.Findings {
				ids = append(ids, f.ID)
				if f.ID == "cluster.sync_degraded" && !strings.Contains(f.Symptom, c.symptom) {
					t.Errorf("symptom %q lacks %q", f.Symptom, c.symptom)
				}
			}
			if r.Verdict != c.verdict || strings.Join(ids, ",") != c.ids {
				t.Errorf("verdict=%s ids=%v", r.Verdict, ids)
			}
		})
	}
}
