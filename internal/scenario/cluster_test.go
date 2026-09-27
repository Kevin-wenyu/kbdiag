package scenario

import (
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// clusterFacts rebuilds cluster.nodes and cluster.events from the stage 0
// captures (repmgr.nodes and the latest events, read from database esrep).
// is_local compares slot_name with the node's primary_slot_name
// (params_<node>_sys_nondefault).
func clusterFacts(t *testing.T, node string) (facts.ClusterNodes, facts.ClusterEvents) {
	t.Helper()
	slot := ""
	for _, m := range loadKsql(t, "params_"+node+"_sys_nondefault").rows {
		if *m["name"] == "primary_slot_name" {
			slot = *m["setting"]
		}
	}
	n := facts.ClusterNodes{Status: facts.StatusOK}
	for _, m := range loadKsql(t, "cluster_"+node+"_sys_nodes").rows {
		var up *int32
		if v := kI64(t, m["upstream_node_id"]); v != nil {
			x := int32(*v)
			up = &x
		}
		prio := int32(*kI64(t, m["priority"]))
		n.Rows = append(n.Rows, facts.ClusterNode{NodeID: int32(*kI64(t, m["node_id"])), NodeName: *m["node_name"], Type: *m["type"], UpstreamNodeID: up,
			Active: kBool(m["active"]), Priority: &prio, Location: m["location"], SlotName: m["slot_name"], IsLocal: *m["slot_name"] == slot})
	}
	e := facts.ClusterEvents{Status: facts.StatusOK}
	at := v02Context("primary", "system", "local").CollectedAt
	for i, m := range loadKsql(t, "cluster_"+node+"_sys_events").rows {
		if i == facts.ClusterEventLimit {
			break
		}
		ts := *kTime(t, m["event_timestamp"])
		e.Rows = append(e.Rows, facts.ClusterEvent{NodeID: int32(*kI64(t, m["node_id"])), Event: *m["event"], Successful: kBool(m["successful"]),
			Time: ts, AgeS: at.Sub(ts).Seconds(), Details: m["details"]})
	}
	return n, e
}

func esrep(role, user, location string) facts.Context {
	c := v02Context(role, user, location)
	c.Database = "esrep"
	return c
}

func TestClusterText(t *testing.T) {
	n1, e1 := clusterFacts(t, "node1")
	n2, e2 := clusterFacts(t, "node2")
	na := facts.InstDownstreams{Status: facts.StatusNotApplicable, Reason: "standby"}
	node2 := str("node2")
	attached := facts.InstDownstreams{Status: facts.StatusOK, Rows: []facts.Downstream{{ApplicationName: node2, State: str("streaming"), SyncState: str("quorum")}}}
	for _, x := range []struct {
		golden  string
		c       facts.Context
		n       facts.ClusterNodes
		e       facts.ClusterEvents
		d       facts.InstDownstreams
		verdict rule.Verdict
	}{
		{"cluster_primary", esrep("primary", "system", "local"), n1, e1, attached, rule.VerdictOK},
		{"cluster_standby", esrep("standby", "system", "local"), n2, e2, na, rule.VerdictOK},
		// repl_node1_sys_stat_paused: no walsender while node2's walreceiver is paused
		{"cluster_primary_paused", esrep("primary", "system", "local"), n1, e1, facts.InstDownstreams{Status: facts.StatusOK}, rule.VerdictWARN},
		{"cluster_ro", esrep("primary", "kbdiag_ro", "remote"),
			facts.ClusterNodes{Status: facts.StatusSkipped, Reason: "insufficient_privilege 42501: permission denied for schema repmgr (grant USAGE on schema repmgr and SELECT on its tables)"},
			facts.ClusterEvents{Status: facts.StatusSkipped, Reason: "insufficient_privilege 42501: permission denied for schema repmgr (grant USAGE on schema repmgr and SELECT on its tables)"}, attached, rule.VerdictUNKNOWN},
	} {
		t.Run(x.golden, func(t *testing.T) {
			rep := Cluster(x.c, x.n, x.e, x.d)
			assertGolden(t, x.golden, rep)
			if rep.Verdict != x.verdict {
				t.Errorf("verdict = %s", rep.Verdict)
			}
		})
	}
}

// Two active primaries, an inactive node, this node not identified, an
// event of an unknown node with long hostile details; then a database
// without repmgr's schema.
func TestClusterTextEdges(t *testing.T) {
	one := int32(1)
	n := facts.ClusterNodes{Status: facts.StatusOK, Rows: []facts.ClusterNode{
		{NodeID: 1, NodeName: "node1", Type: "primary", Active: true, SlotName: str("repmgr_slot_1")},
		{NodeID: 2, NodeName: "node2", Type: "primary", Active: true, SlotName: str("repmgr_slot_2")},
		{NodeID: 3, NodeName: "node\x1b[2J3", Type: "standby", UpstreamNodeID: &one, Active: false},
	}}
	at := v02Context("primary", "system", "local").CollectedAt
	e := facts.ClusterEvents{Status: facts.StatusOK, Rows: []facts.ClusterEvent{
		{NodeID: 9, Event: "standby_promote", Successful: false, Time: at.Add(-90e9), AgeS: 90, Details: str("promotion failed:\n" + strings.Repeat("x", 120))},
	}}
	rep := Cluster(esrep("primary", "system", "local"), n, e, facts.InstDownstreams{Status: facts.StatusOK})
	assertGolden(t, "cluster_edges", rep)
	if rep.Verdict != rule.VerdictWARN || len(rep.Findings) != 2 {
		t.Errorf("verdict=%s findings=%+v", rep.Verdict, rep.Findings)
	}
	c := esrep("primary", "system", "local")
	c.Database = "test"
	reason := "no repmgr schema in database test: not a repmgr cluster, or its metadata is in another database (use -d)"
	rep = Cluster(c, facts.ClusterNodes{Status: facts.StatusNotApplicable, Reason: reason}, facts.ClusterEvents{Status: facts.StatusNotApplicable, Reason: reason}, facts.InstDownstreams{Status: facts.StatusOK})
	assertGolden(t, "cluster_norepmgr", rep)
	if rep.Verdict != rule.VerdictOK {
		t.Errorf("verdict=%s", rep.Verdict)
	}
}
