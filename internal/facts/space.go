package facts

const (
	SpaceTablespacesID = "space.tablespaces"
	SpaceWALID         = "space.wal"
	SpaceDiskID        = "space.disk"
)

// Column contracts of the space probes (PRD §5.2).
var (
	TablespaceColumns = []string{"spcname", "location", "size_bytes"}
	WALColumns        = []string{"files", "bytes", "max_wal_size_bytes", "wal_keep_bytes", "wal_segment_bytes"}
	SpaceDiskColumns  = []string{"path_kind", "path", "total_bytes", "used_bytes", "avail_bytes"}
)

type Tablespace struct {
	Name      string
	Location  *string // empty or NULL for sys_default and sys_global: inside data_directory
	SizeBytes *int64  // NULL when this account may not read it
}

func (t Tablespace) Row() []any { return []any{t.Name, t.Location, t.SizeBytes} }

type SpaceTablespaces struct {
	Status Status
	Reason string
	Rows   []Tablespace
}

// Redacted reports tablespaces whose size we may not read.
func (s SpaceTablespaces) Redacted() []Redaction {
	n := 0
	for _, x := range s.Rows {
		if x.SizeBytes == nil {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return []Redaction{{ProbeID: SpaceTablespacesID, Field: "size_bytes", Reason: ReasonInsufficientPrivilege, RowsAffected: n}}
}

// WAL is the single row of space.wal: what sys_wal holds, and the settings
// that decide how much it may keep without anyone holding it back.
type WAL struct {
	Files           int64
	Bytes           int64
	MaxWALSizeBytes *int64
	WALKeepBytes    *int64 // wal_keep_segments × wal_segment_size
	WALSegmentBytes *int64
}

func (w WAL) Row() []any {
	return []any{w.Files, w.Bytes, w.MaxWALSizeBytes, w.WALKeepBytes, w.WALSegmentBytes}
}

type SpaceWAL struct {
	Status Status
	Reason string
	Rows   []WAL
}

// Mount is one directory kbdiag checked and the filesystem it lives on.
type Mount struct {
	Kind string // data_directory | wal | tablespace
	Path string
	Disk
}

func (m Mount) Row() []any { return []any{m.Kind, m.Path, m.TotalBytes, m.UsedBytes, m.AvailBytes} }

type SpaceDisk struct {
	Status Status
	Reason string
	Rows   []Mount
}
