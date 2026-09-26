package facts

import "time"

const (
	InstInfoID        = "inst.info"
	InstDatabasesID   = "inst.databases"
	InstDownstreamsID = "inst.downstreams"
	InstUpstreamID    = "inst.upstream"
	InstDiskID        = "inst.disk"
)

// Column contracts of the status probes (PRD §5.1).
var (
	InfoColumns        = []string{"version", "start_time", "uptime_s", "connections", "max_connections", "superuser_reserved_connections", "data_directory", "port", "usable_connections"}
	DatabaseColumns    = []string{"datname", "size_bytes"}
	DownstreamsColumns = []string{"application_name", "client_addr", "state", "sync_state"}
	UpstreamColumns    = []string{"status", "sender_host", "sender_port", "slot_name", "last_msg_age_s"}
	DiskColumns        = []string{"total_bytes", "used_bytes", "avail_bytes"}
)

// Info is the single row of inst.info.
type Info struct {
	Version           string // short version number, e.g. V008R006C009B0014
	StartTime         time.Time
	UptimeS           float64
	Connections       int32 // backends connected to a database (sys_stat_database.numbackends)
	MaxConnections    int32
	SuperuserReserved int32
	DataDirectory     *string
	Port              int32
}

// Usable is how many connections ordinary users may open: the rest of
// max_connections is reserved for superusers.
func (i Info) Usable() int32 { return i.MaxConnections - i.SuperuserReserved }

func (i Info) Row() []any {
	return []any{i.Version, i.StartTime.Truncate(time.Second).Format(time.RFC3339), i.UptimeS,
		i.Connections, i.MaxConnections, i.SuperuserReserved, i.DataDirectory, i.Port, i.Usable()}
}

type Database struct {
	Datname   string
	SizeBytes *int64 // NULL without CONNECT on the database
}

func (d Database) Row() []any { return []any{d.Datname, d.SizeBytes} }

type InstInfo struct {
	Status Status
	Reason string
	Rows   []Info
}

type InstDatabases struct {
	Status Status
	Reason string
	Rows   []Database
}

// Redacted reports databases whose size we may not read.
func (d InstDatabases) Redacted() []Redaction {
	n := 0
	for _, x := range d.Rows {
		if x.SizeBytes == nil {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return []Redaction{{ProbeID: InstDatabasesID, Field: "size_bytes", Reason: ReasonInsufficientPrivilege, RowsAffected: n}}
}

// Downstream is one walsender row of sys_stat_replication. PG shows a user
// without sys_monitor only the application name; KES V8R6 shows everything,
// but the NULL case is kept.
type Downstream struct {
	ApplicationName *string // '' reads as NULL in Oracle mode
	ClientAddr      *string // NULL for a Unix socket, or masked
	State           *string
	SyncState       *string // raw: async | sync | potential | quorum
}

func (d Downstream) Row() []any { return []any{d.ApplicationName, d.ClientAddr, d.State, d.SyncState} }

type InstDownstreams struct {
	Status Status
	Reason string
	Rows   []Downstream
}

// Redacted reports downstreams whose state we may not read. state is never
// NULL for a visible walsender; client_addr is NULL on a socket anyway, so
// it cannot tell masking apart and is not counted.
func (d InstDownstreams) Redacted() []Redaction {
	n := 0
	for _, x := range d.Rows {
		if x.State == nil {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return []Redaction{
		{ProbeID: InstDownstreamsID, Field: "state", Reason: ReasonInsufficientPrivilege, RowsAffected: n},
		{ProbeID: InstDownstreamsID, Field: "sync_state", Reason: ReasonInsufficientPrivilege, RowsAffected: n},
	}
}

// Upstream is the WAL receiver row of sys_stat_wal_receiver. PG shows a user
// without sys_monitor the row with every column NULL; KES V8R6 shows
// everything, but the NULL case is kept.
type Upstream struct {
	Status      *string
	SenderHost  *string
	SenderPort  *int32
	SlotName    *string
	LastMsgAgeS *float64
}

func (u Upstream) Row() []any {
	return []any{u.Status, u.SenderHost, u.SenderPort, u.SlotName, u.LastMsgAgeS}
}

// InstUpstream has no row when the standby runs no WAL receiver.
type InstUpstream struct {
	Status Status
	Reason string
	Rows   []Upstream
}

// Redacted reports a WAL receiver whose status we may not read.
func (u InstUpstream) Redacted() []Redaction {
	n := 0
	for _, x := range u.Rows {
		if x.Status == nil {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return []Redaction{{ProbeID: InstUpstreamID, Field: "status", Reason: ReasonInsufficientPrivilege, RowsAffected: n}}
}

// Disk is the filesystem holding data_directory, as df reports it: used
// plus avail is less than total by the blocks reserved for root.
type Disk struct {
	TotalBytes uint64
	UsedBytes  uint64
	AvailBytes uint64
}

func (d Disk) Row() []any { return []any{d.TotalBytes, d.UsedBytes, d.AvailBytes} }

type InstDisk struct {
	Status Status
	Reason string
	Rows   []Disk
}
