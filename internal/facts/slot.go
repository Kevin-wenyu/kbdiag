package facts

// SlotListID is the probe_id of the sys_replication_slots probe.
const SlotListID = "slot.list"

// SlotColumns is the column contract of slot.list (PRD §5.1).
var SlotColumns = []string{"slot_name", "slot_type", "active", "active_pid", "xmin", "catalog_xmin", "xmin_age", "restart_lsn", "retained_wal_bytes"}

type Slot struct {
	Name             string
	Type             string
	Active           bool
	ActivePID        *int32
	Xmin             *uint32
	CatalogXmin      *uint32
	XminAge          *int32
	RestartLSN       *string
	RetainedWALBytes *int64 // from the current LSN on a primary, the replay LSN on a standby
}

func (s Slot) Row() []any {
	return []any{s.Name, s.Type, s.Active, s.ActivePID, s.Xmin, s.CatalogXmin, s.XminAge, s.RestartLSN, s.RetainedWALBytes}
}

type SlotList struct {
	Status Status
	Reason string
	Rows   []Slot
}
