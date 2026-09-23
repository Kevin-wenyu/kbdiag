package facts

import "time"

const (
	InstInfoID        = "inst.info"
	InstDatabasesID   = "inst.databases"
	InstDownstreamsID = "inst.downstreams"
)

// Column contracts of the status probes (PRD §5.1).
var (
	InfoColumns        = []string{"version", "start_time", "uptime_s", "connections", "max_connections", "superuser_reserved_connections", "data_directory"}
	DatabaseColumns    = []string{"datname", "size_bytes"}
	DownstreamsColumns = []string{"downstreams"}
)

// Info is the single row of inst.info.
type Info struct {
	Version           string
	StartTime         time.Time
	UptimeS           float64
	Connections       int32 // backends connected to a database (sys_stat_database.numbackends)
	MaxConnections    int32
	SuperuserReserved int32
	DataDirectory     *string
}

func (i Info) Row() []any {
	return []any{i.Version, i.StartTime.Truncate(time.Second).Format(time.RFC3339), i.UptimeS,
		i.Connections, i.MaxConnections, i.SuperuserReserved, i.DataDirectory}
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

type InstDownstreams struct {
	Status Status
	Reason string
	Rows   []int64
}
