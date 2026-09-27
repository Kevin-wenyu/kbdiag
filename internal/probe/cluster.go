package probe

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// clusterSchemaSQL checks that the connected database holds repmgr's
// metadata. sys_namespace is readable by everyone; to_regclass on a schema
// without USAGE may error or return NULL, which would hide a missing grant.
const clusterSchemaSQL = `select exists(select 1 from sys_namespace where nspname = 'repmgr')`

// clusterNodesSQL is cluster.nodes: repmgr's list of nodes, without conninfo
// (it may carry a password). is_local marks this instance's node, found two
// ways: the node repmgr's own function names ($1, from
// repmgr.get_local_node_id(), when it exists and answers), or the node
// whose slot is this instance's primary_slot_name (repmgr names node N's
// slot repmgr_slot_N and writes it into node N's primary_slot_name when it
// clones or rejoins it: stage 0 node1 repmgr_slot_1, node2 repmgr_slot_2).
// A primary that was never a standby may have no primary_slot_name, hence
// the function first. Neither is verified beyond the lab.
// Source: repmgr's own schema (stage 0 capture cluster_*_nodes, database
// esrep); kbdiag_ro gets "permission denied for schema repmgr". Not run on a
// VM yet.
const clusterNodesSQL = `
select n.node_id, n.node_name::text, n.type::text, n.upstream_node_id, n.active, n.priority, n.location::text, n.slot_name::text,
       coalesce(n.node_id = $1::int, false)
         or coalesce(n.slot_name::text = (select setting from sys_settings where name = 'primary_slot_name'), false)
from repmgr.nodes n
order by n.node_id`

// clusterEventsSQL is cluster.events: repmgr's latest events (stage 0
// capture cluster_*_events: disconnects, reconnects, recoveries).
const clusterEventsSQL = `
select node_id, event::text, successful, event_timestamp,
       round(extract(epoch from now() - event_timestamp)::numeric, 1)::float8, details::text
from repmgr.events
order by event_timestamp desc
limit 20`

// ClusterNodes reads repmgr.nodes; a database without the repmgr schema is
// not_applicable (not a repmgr cluster, or the metadata is elsewhere).
func ClusterNodes(ctx context.Context, x *pgx.Conn, c facts.Context) facts.ClusterNodes {
	if st, reason, ok := repmgrSchema(ctx, x, c); !ok {
		return facts.ClusterNodes{Status: st, Reason: reason}
	}
	st, reason, out := collectArgs(ctx, x, clusterNodesSQL, []any{localNodeID(ctx, x)}, func(r pgx.CollectableRow) (facts.ClusterNode, error) {
		var n facts.ClusterNode
		err := r.Scan(&n.NodeID, &n.NodeName, &n.Type, &n.UpstreamNodeID, &n.Active, &n.Priority, &n.Location, &n.SlotName, &n.IsLocal)
		return n, err
	})
	return facts.ClusterNodes{Status: st, Reason: withGrantHint(st, reason), Rows: out}
}

func ClusterEvents(ctx context.Context, x *pgx.Conn, c facts.Context) facts.ClusterEvents {
	if st, reason, ok := repmgrSchema(ctx, x, c); !ok {
		return facts.ClusterEvents{Status: st, Reason: reason}
	}
	st, reason, out := collect(ctx, x, clusterEventsSQL, func(r pgx.CollectableRow) (facts.ClusterEvent, error) {
		var e facts.ClusterEvent
		err := r.Scan(&e.NodeID, &e.Event, &e.Successful, &e.Time, &e.AgeS, &e.Details)
		return e, err
	})
	return facts.ClusterEvents{Status: st, Reason: withGrantHint(st, reason), Rows: out}
}

// localNodeID asks repmgr's extension which node this is. It is set by
// repmgrd in shared memory, so it may be missing or NULL (-1 when unset in
// some versions); any failure just leaves the primary_slot_name match.
func localNodeID(ctx context.Context, x *pgx.Conn) *int32 {
	var exists bool
	err := x.QueryRow(ctx, `select exists(select 1 from sys_proc p join sys_namespace n on n.oid = p.pronamespace
		where n.nspname = 'repmgr' and p.proname = 'get_local_node_id')`).Scan(&exists)
	if err != nil || !exists {
		return nil
	}
	var id *int32
	if err := x.QueryRow(ctx, "select repmgr.get_local_node_id()").Scan(&id); err != nil || id == nil || *id <= 0 {
		return nil
	}
	return id
}

func repmgrSchema(ctx context.Context, x *pgx.Conn, c facts.Context) (facts.Status, string, bool) {
	var ok bool
	if err := x.QueryRow(ctx, clusterSchemaSQL).Scan(&ok); err != nil {
		st, reason := classify(err)
		return st, reason, false
	}
	if !ok {
		return facts.StatusNotApplicable, "no repmgr schema in database " + c.Database + ": not a repmgr cluster, or its metadata is in another database (use -d)", false
	}
	return facts.StatusOK, "", true
}

// withGrantHint says what to grant when repmgr's schema is refused.
func withGrantHint(st facts.Status, reason string) string {
	if st == facts.StatusSkipped && strings.HasPrefix(reason, "insufficient_privilege") {
		return reason + " (grant USAGE on schema repmgr and SELECT on its tables)"
	}
	return reason
}
