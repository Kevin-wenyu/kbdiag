package rule

import (
	"fmt"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// Cluster compares repmgr's view with the database's own, all WARN: repmgr
// acts (fails over, follows) on its metadata, so a disagreement is a risk
// before it hurts. Checked: more than one active primary; inactive nodes;
// this node's type against its recovery role; on a primary, the active
// standbys repmgr says follow this node but that are not attached. Whether
// nodes are reachable is not visible from one connection.
func Cluster(n facts.ClusterNodes, d facts.InstDownstreams, role string) Result {
	judge, unknown := collected(n.Status)
	if !judge {
		return Result{Verdict: verdictOf(nil, unknown)}
	}
	names := map[int32]string{}
	var primaries []string
	var local *facts.ClusterNode
	for i, x := range n.Rows {
		names[x.NodeID] = x.NodeName
		if x.Active && x.Type == "primary" {
			primaries = append(primaries, x.NodeName)
		}
		if x.IsLocal {
			local = &n.Rows[i]
		}
	}
	var fs []Finding
	if len(primaries) > 1 {
		fs = append(fs, Finding{
			ID:       "cluster.primaries",
			Level:    LevelWARN,
			Symptom:  fmt.Sprintf("repmgr lists %d active primaries (%s): its metadata says split brain; whether both accept writes shows only on each node", len(primaries), strings.Join(primaries, ", ")),
			Evidence: []Evidence{{ProbeID: facts.ClusterNodesID, Fields: map[string]any{"primaries": primaries}}},
			Next:     []Next{{Kind: "verify", Command: "kbdiag status", Note: "run on each of them: which one is out of recovery"}},
		})
	}
	for _, x := range n.Rows {
		if x.Active {
			continue
		}
		fs = append(fs, Finding{
			ID:       "cluster.inactive",
			Level:    LevelWARN,
			Symptom:  fmt.Sprintf("repmgr marks %s node %s inactive: it counts it as failed or removed, so the cluster has one node less to fail over to", x.Type, x.NodeName),
			Evidence: []Evidence{{ProbeID: facts.ClusterNodesID, Fields: map[string]any{"node_id": x.NodeID, "node_name": x.NodeName, "type": x.Type, "active": false}}},
			Next:     []Next{{Kind: "verify", Command: "kbdiag status", Note: "run on " + x.NodeName + ": is it up, and in which role"}},
		})
	}
	if local == nil {
		return Result{Verdict: verdictOf(fs, true), Findings: fs}
	}
	if want := map[string]string{"primary": "primary", "standby": "standby"}[local.Type]; want != "" && want != role {
		fs = append(fs, Finding{
			ID:    "cluster.role_mismatch",
			Level: LevelWARN,
			Symptom: fmt.Sprintf("repmgr says this node (%s) is a %s, but the database is a %s: repmgr acts on the wrong role until its metadata is corrected",
				local.NodeName, local.Type, role),
			Evidence: []Evidence{{ProbeID: facts.ClusterNodesID, Fields: map[string]any{"node_id": local.NodeID, "node_name": local.NodeName, "type": local.Type, "role": role}}},
			Next:     []Next{{Kind: "verify", Command: "kbdiag repl", Note: "what this node streams to or from"}},
		})
	}
	if role != "primary" || local.Type != "primary" {
		return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
	}
	dj, du := collected(d.Status)
	if !dj {
		return Result{Verdict: verdictOf(fs, unknown || du), Findings: fs}
	}
	for _, x := range Followers(n, local.NodeID) {
		if Attached(d, x.NodeName) {
			continue
		}
		fs = append(fs, Finding{
			ID:    "cluster.detached",
			Level: LevelWARN,
			Symptom: fmt.Sprintf("repmgr lists %s as an active standby of this node (%s), but no walsender here has that application_name: it is not attached (repmgr cluster show would report it so), unless its conninfo sets another application_name",
				x.NodeName, local.NodeName),
			Evidence: []Evidence{{ProbeID: facts.ClusterNodesID, Fields: map[string]any{"node_id": x.NodeID, "node_name": x.NodeName, "upstream_node_id": local.NodeID}}},
			Next: []Next{
				{Kind: "verify", Command: "kbdiag status", Note: "run on " + x.NodeName + ": is it up, is it receiving WAL"},
				{Kind: "verify", Command: "kbdiag slots", Note: "whether its slot here is inactive"},
			},
		})
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}

// Followers are the active standbys repmgr says stream from node id.
func Followers(n facts.ClusterNodes, id int32) []facts.ClusterNode {
	var out []facts.ClusterNode
	for _, x := range n.Rows {
		if x.Active && x.Type == "standby" && x.UpstreamNodeID != nil && *x.UpstreamNodeID == id {
			out = append(out, x)
		}
	}
	return out
}

// Attached reports a walsender for the node: repmgr sets application_name
// to the node name (stage 0: node2 = node2), compared case-insensitively.
func Attached(d facts.InstDownstreams, name string) bool {
	for _, r := range d.Rows {
		if r.ApplicationName != nil && strings.EqualFold(*r.ApplicationName, name) {
			return true
		}
	}
	return false
}
