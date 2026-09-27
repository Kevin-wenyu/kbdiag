package facts

const (
	ReplDownstreamsID = "repl.downstreams"
	ReplSyncID        = "repl.sync"
	ReplReplayID      = "repl.replay"
)

// Column contracts of the repl probes (PRD §5.2). inst.upstream is reused.
var (
	ReplDownstreamColumns = []string{"pid", "application_name", "client_addr", "state", "sync_state", "sync_priority", "sent_lsn", "write_lsn", "flush_lsn", "replay_lsn",
		"sent_lag_bytes", "flush_lag_bytes", "replay_lag_bytes", "write_lag_s", "flush_lag_s", "replay_lag_s", "reply_age_s"}
	ReplSyncColumns   = []string{"synchronous_standby_names", "synchronous_commit"}
	ReplReplayColumns = []string{"receive_lsn", "replay_lsn", "replay_gap_bytes", "last_replay_age_s", "replay_paused"}
)

// Replica is one walsender of sys_stat_replication, with how far each LSN is
// behind this node's current position (the replay LSN on a standby that
// feeds a cascade).
type Replica struct {
	PID             int32
	ApplicationName *string
	ClientAddr      *string
	State           *string
	SyncState       *string
	SyncPriority    *int32
	SentLSN         *string
	WriteLSN        *string
	FlushLSN        *string
	ReplayLSN       *string
	SentLagBytes    *int64
	FlushLagBytes   *int64
	ReplayLagBytes  *int64
	WriteLagS       *float64 // NULL once the standby is idle and caught up
	FlushLagS       *float64
	ReplayLagS      *float64
	ReplyAgeS       *float64
}

func (r Replica) Row() []any {
	return []any{r.PID, r.ApplicationName, r.ClientAddr, r.State, r.SyncState, r.SyncPriority, r.SentLSN, r.WriteLSN, r.FlushLSN, r.ReplayLSN,
		r.SentLagBytes, r.FlushLagBytes, r.ReplayLagBytes, r.WriteLagS, r.FlushLagS, r.ReplayLagS, r.ReplyAgeS}
}

type ReplDownstreams struct {
	Status Status
	Reason string
	Rows   []Replica
}

type SyncSetting struct {
	StandbyNames *string // NULL or '' (Oracle mode): no synchronous replication
	Commit       string
}

func (s SyncSetting) Row() []any { return []any{s.StandbyNames, s.Commit} }

type ReplSync struct {
	Status Status
	Reason string
	Rows   []SyncSetting
}

// Replay is the single row of repl.replay on a standby.
type Replay struct {
	ReceiveLSN     *string
	ReplayLSN      *string
	ReplayGapBytes *int64
	LastReplayAgeS *float64 // since the last replayed transaction: grows on an idle primary too
	ReplayPaused   bool
}

func (r Replay) Row() []any {
	return []any{r.ReceiveLSN, r.ReplayLSN, r.ReplayGapBytes, r.LastReplayAgeS, r.ReplayPaused}
}

type ReplReplay struct {
	Status Status
	Reason string
	Rows   []Replay
}
