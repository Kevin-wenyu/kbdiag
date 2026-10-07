package probe

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

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

// clusterSyncSQL finds this node's repmgr.conf: repmgr keeps each node's
// sys_bindir in its conf table (lab: the only key there), and repmgrd and
// kbha run with -f $sys_bindir/../etc/repmgr.conf. The data directory and
// synchronous_standby_names come along to check the file is this
// instance's and to compare.
const clusterSyncSQL = `
select (select value from repmgr.conf where node_id = $1::int and key = 'sys_bindir'),
       current_setting('data_directory'),
       current_setting('synchronous_standby_names')`

// readFile is swapped out in tests.
var readFile = readConf

// readConf reads a regular file of at most 1 MiB: the path comes from the
// database, and a FIFO or device there must not hang kbdiag or fill memory.
func readConf(path string) ([]byte, error) {
	// open blocks on a FIFO before Stat can see it: check first, and open
	// non-blocking in case it is swapped in between
	if st, err := os.Stat(path); err != nil {
		return nil, err
	} else if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	const max = 1 << 20
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err == nil && len(b) > max {
		return nil, fmt.Errorf("%s is larger than 1 MiB", path)
	}
	return b, err
}

// ClusterSync reads the synchronous mode repmgr is configured to keep from
// this node's repmgr.conf, on a primary only (where synchronous_standby_names
// takes effect). Like inst.disk it reads the database host's files, so it
// runs only locally; a file that is not this node's (node_id or
// data_directory differ) is not used.
func ClusterSync(ctx context.Context, x *pgx.Conn, c facts.Context, n facts.ClusterNodes, socket, loopback bool) facts.ClusterSyncs {
	if n.Status != facts.StatusOK {
		return facts.ClusterSyncs{Status: n.Status, Reason: "cluster.nodes was not collected"}
	}
	if c.Role != "primary" {
		return facts.ClusterSyncs{Status: facts.StatusNotApplicable, Reason: "standby: synchronous_standby_names takes effect on the primary"}
	}
	var local *facts.ClusterNode
	for i := range n.Rows {
		if n.Rows[i].IsLocal {
			local = &n.Rows[i]
		}
	}
	if local == nil {
		return facts.ClusterSyncs{Status: facts.StatusSkipped, Reason: "this node is not identified in repmgr.nodes, so its repmgr.conf is unknown"}
	}
	if !socket && !loopback {
		return facts.ClusterSyncs{Status: facts.StatusSkipped, Reason: "remote connection: repmgr.conf is read on the database host only"}
	}
	var bindir, datadir, names *string
	if err := x.QueryRow(ctx, clusterSyncSQL, local.NodeID).Scan(&bindir, &datadir, &names); err != nil {
		st, reason := classify(err)
		return facts.ClusterSyncs{Status: st, Reason: withGrantHint(st, reason)}
	}
	if bindir == nil {
		return facts.ClusterSyncs{Status: facts.StatusSkipped, Reason: "repmgr.conf table has no sys_bindir for node " + strconv.Itoa(int(local.NodeID))}
	}
	return clusterSyncFile(*bindir, local.NodeID, datadir, names)
}

// clusterSyncFile reads and checks the file found from sys_bindir.
func clusterSyncFile(bindir string, nodeID int32, datadir, names *string) facts.ClusterSyncs {
	local := facts.ClusterNode{NodeID: nodeID}
	// not filepath.Clean: repmgrd's -f $sys_bindir/../etc resolves ".."
	// through the filesystem, so a symlinked bin must be followed the same way
	path := strings.TrimRight(bindir, "/") + "/../etc/repmgr.conf"
	b, err := readFile(path)
	if err != nil {
		return facts.ClusterSyncs{Status: facts.StatusSkipped, Reason: err.Error()}
	}
	conf := parseRepmgrConf(string(b))
	if id, ok := conf["node_id"]; ok && id != strconv.Itoa(int(local.NodeID)) {
		return facts.ClusterSyncs{Status: facts.StatusSkipped, Reason: path + " is for node_id " + id + ", not this node (" + strconv.Itoa(int(local.NodeID)) + ")"}
	}
	if d, ok := conf["data_directory"]; ok && datadir != nil && filepath.Clean(d) != filepath.Clean(*datadir) {
		return facts.ClusterSyncs{Status: facts.StatusSkipped, Reason: path + " is for data_directory " + d + ", not this instance's"}
	}
	row := facts.ClusterSync{ConfPath: path, StandbyNames: names}
	if v, ok := conf["synchronous"]; ok {
		row.Synchronous = &v
	}
	return facts.ClusterSyncs{Status: facts.StatusOK, Rows: []facts.ClusterSync{row}}
}

// parseRepmgrConf reads repmgr.conf's lines, key=value or key value (its
// parser takes the = as optional): # starts a comment outside quotes,
// values may be in single or double quotes; a later line overrides an
// earlier one, as in repmgr.
func parseRepmgrConf(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if i := strings.IndexAny(line, " \t"); i >= 0 && (!ok || i < len(k)) {
			if pre := strings.TrimSpace(line[i:]); !strings.HasPrefix(pre, "=") {
				k, v, ok = line[:i], pre, true
			}
		}
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) > 0 && (v[0] == '\'' || v[0] == '"') {
			if end := strings.IndexByte(v[1:], v[0]); end >= 0 {
				v = v[1 : end+1]
			}
		} else if i := strings.IndexByte(v, '#'); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		out[k] = v
	}
	return out
}
