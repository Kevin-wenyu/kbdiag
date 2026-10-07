package facts

import "time"

const (
	ClusterNodesID  = "cluster.nodes"
	ClusterEventsID = "cluster.events"
	ClusterSyncID   = "cluster.sync"
)

// ClusterEventLimit is how many of repmgr's latest events cluster.events
// reads: the definition of the probe, not a truncation.
const ClusterEventLimit = 20

// Column contracts of the cluster probes (PRD §5.2).
var (
	ClusterNodeColumns  = []string{"node_id", "node_name", "type", "upstream_node_id", "active", "priority", "location", "slot_name", "is_local"}
	ClusterEventColumns = []string{"node_id", "event", "successful", "event_time", "event_age_s", "details"}
	ClusterSyncColumns  = []string{"conf_path", "synchronous", "synchronous_standby_names"}
)

// ClusterNode is one row of repmgr.nodes. conninfo is not read: it may
// carry a password.
type ClusterNode struct {
	NodeID         int32
	NodeName       string
	Type           string // primary | standby | witness
	UpstreamNodeID *int32
	Active         bool
	Priority       *int32
	Location       *string
	SlotName       *string
	// IsLocal: the node's slot_name is this instance's primary_slot_name.
	// repmgr names node N's slot repmgr_slot_N and writes it into node N's
	// primary_slot_name (stage 0: node1 repmgr_slot_1, node2 repmgr_slot_2).
	IsLocal bool
}

func (n ClusterNode) Row() []any {
	return []any{n.NodeID, n.NodeName, n.Type, n.UpstreamNodeID, n.Active, n.Priority, n.Location, n.SlotName, n.IsLocal}
}

type ClusterNodes struct {
	Status Status
	Reason string
	Rows   []ClusterNode
}

type ClusterEvent struct {
	NodeID     int32
	Event      string
	Successful bool
	Time       time.Time
	AgeS       float64
	Details    *string
}

func (e ClusterEvent) Row() []any {
	return []any{e.NodeID, e.Event, e.Successful, e.Time.Format(time.RFC3339), e.AgeS, e.Details}
}

type ClusterEvents struct {
	Status Status
	Reason string
	Rows   []ClusterEvent
}

// ClusterSync is what repmgr is configured to keep (synchronous in this
// node's repmgr.conf) next to what the primary has now. repmgrd switches
// synchronous_standby_names to "" while the synchronous standby is gone and
// back when it returns (lab hamgr.log, 2026-10-07); the configured mode is
// only in the file, not in repmgr's tables.
type ClusterSync struct {
	ConfPath    string
	Synchronous *string // NULL: not set in the file
	// StandbyNames is the primary's synchronous_standby_names now; both ''
	// (what KES returned when repmgrd emptied it, VM 2026-10-07) and NULL
	// mean empty.
	StandbyNames *string
}

func (s ClusterSync) Row() []any { return []any{s.ConfPath, s.Synchronous, s.StandbyNames} }

type ClusterSyncs struct {
	Status Status
	Reason string
	Rows   []ClusterSync
}
