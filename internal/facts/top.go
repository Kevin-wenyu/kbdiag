package facts

// SQLTopID is the probe_id of the cumulative statement statistics.
const SQLTopID = "sql.top"

// TopColumns is the column contract of sql.top (PRD §5.2). Times are
// seconds, converted from sys_stat_statements' milliseconds.
var TopColumns = []string{"queryid", "username", "datname", "calls", "total_exec_s", "mean_exec_s", "max_exec_s", "rows", "shared_blks_hit", "shared_blks_read", "temp_blks_written", "query"}

// Statement is one row of sys_stat_statements: counters accumulated since
// the last sys_stat_statements_reset() (or since it was installed).
type Statement struct {
	QueryID         *int64 // NULL for another user's statement without the right to see it
	Username        *string
	Datname         *string
	Calls           int64
	TotalExecS      float64
	MeanExecS       float64
	MaxExecS        float64
	Rows            int64
	SharedBlksHit   int64
	SharedBlksRead  int64
	TempBlksWritten int64
	Query           *string // "<insufficient privilege>" when masked
}

func (s Statement) Row() []any {
	return []any{s.QueryID, s.Username, s.Datname, s.Calls, s.TotalExecS, s.MeanExecS, s.MaxExecS, s.Rows, s.SharedBlksHit, s.SharedBlksRead, s.TempBlksWritten, s.Query}
}

// Masked reports a statement whose text this account may not read.
func (s Statement) Masked() bool { return s.Query != nil && *s.Query == "<insufficient privilege>" }

type SQLTop struct {
	Status Status
	Reason string
	Rows   []Statement
	// Track is sys_stat_statements.track as this connection sees it (a role
	// or database may set another); not in the JSON.
	Track string
}

// Redacted reports statements whose text and queryid are hidden.
func (t SQLTop) Redacted() []Redaction {
	n := 0
	for _, s := range t.Rows {
		if s.Masked() {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return []Redaction{
		{ProbeID: SQLTopID, Field: "query", Reason: ReasonInsufficientPrivilege, RowsAffected: n},
		{ProbeID: SQLTopID, Field: "queryid", Reason: ReasonInsufficientPrivilege, RowsAffected: n},
	}
}
