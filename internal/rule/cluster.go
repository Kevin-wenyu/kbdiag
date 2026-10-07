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
// standbys repmgr says follow this node but that are not attached; on a
// primary, whether repmgr is keeping the synchronous mode its repmgr.conf
// asks for. Whether nodes are reachable is not visible from one connection.
func Cluster(n facts.ClusterNodes, d facts.InstDownstreams, s facts.ClusterSyncs, role string) Result {
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
	sj, su := collected(s.Status)
	unknown = unknown || su
	// the consequence after its cause: cluster.detached first
	var degraded []Finding
	if sj && len(s.Rows) == 1 {
		if f, ok := syncDegraded(s.Rows[0], n, d, dj, local); ok {
			degraded = append(degraded, f)
		}
	}
	if !dj {
		fs = append(fs, degraded...)
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
	fs = append(fs, degraded...)
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

// SyncModes are repmgr's synchronous values that make repmgrd fill
// synchronous_standby_names (repmgrd accepts async, sync, quorum, all,
// custom). custom leaves the list to the user, so it is not judged.
var SyncModes = map[string]bool{"sync": true, "quorum": true, "all": true}

// syncDegraded: repmgr.conf asks for synchronous commits, but the primary's
// synchronous_standby_names is empty, so commits are asynchronous. repmgrd
// does this while the synchronous standby is gone and undoes it when the
// standby returns (lab hamgr.log, 2026-10-07); with the standbys attached
// again and the list still empty, repmgrd has not restored it.
func syncDegraded(s facts.ClusterSync, n facts.ClusterNodes, d facts.InstDownstreams, dj bool, local *facts.ClusterNode) (Finding, bool) {
	if s.Synchronous == nil || !SyncModes[strings.ToLower(*s.Synchronous)] || (s.StandbyNames != nil && strings.TrimSpace(*s.StandbyNames) != "") {
		return Finding{}, false
	}
	why := "while a synchronous standby is away repmgrd switches to asynchronous; it switches back when the standby returns"
	next := []Next{{Kind: "verify", Command: "kbdiag repl", Note: "which standbys stream and what synchronous_standby_names says"}}
	if dj {
		var away []string
		for _, x := range Followers(n, local.NodeID) {
			if !Attached(d, x.NodeName) {
				away = append(away, x.NodeName)
			}
		}
		if len(away) > 0 {
			why = "repmgrd switched to asynchronous because " + strings.Join(away, ", ") + " is not attached; it switches back when it returns"
		} else {
			// right after a standby returns repmgrd needs a moment to notice
			// (lab hamgr.log: the same second as its reconnect notice); only
			// a list that stays empty means it is not doing its job
			why = "yet every standby repmgr lists for this node is attached: repmgrd has not switched back yet; it normally does within seconds of the standby returning, so if a second run still shows this, check repmgrd on this node"
			next = append(next, Next{Kind: "verify", Command: "ps -C repmgrd -o pid,args", Note: "repmgrd restores synchronous_standby_names; without it the list stays empty"})
		}
	}
	return Finding{
		ID:    "cluster.sync_degraded",
		Level: LevelWARN,
		Symptom: fmt.Sprintf("repmgr is configured for %s replication (%s), but synchronous_standby_names is empty: commits do not wait for any standby, so a failover now can lose committed transactions; %s",
			*s.Synchronous, s.ConfPath, why),
		Evidence: []Evidence{{ProbeID: facts.ClusterSyncID, Fields: map[string]any{"synchronous": *s.Synchronous, "synchronous_standby_names": "", "conf_path": s.ConfPath}}},
		Next:     next,
	}, true
}
