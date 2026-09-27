package facts

// SeqListID is the probe_id of the current database's sequences.
const SeqListID = "seq.list"

// SeqColumns is the column contract of seq.list (PRD §5.2).
var SeqColumns = []string{"schemaname", "sequencename", "data_type", "start_value", "min_value", "max_value", "increment_by", "cycle", "cache_size", "last_value", "readable"}

// Sequence is one row of sys_sequences. last_value is NULL both for a
// sequence never called and for one this account may not read (stage 0:
// kbdiag_ro sees every last_value NULL); readable tells them apart.
type Sequence struct {
	Schemaname   string
	Sequencename string
	DataType     string
	StartValue   int64
	MinValue     int64
	MaxValue     int64
	IncrementBy  int64
	Cycle        bool
	CacheSize    int64
	LastValue    *int64
	Readable     bool
}

func (s Sequence) Row() []any {
	return []any{s.Schemaname, s.Sequencename, s.DataType, s.StartValue, s.MinValue, s.MaxValue, s.IncrementBy, s.Cycle, s.CacheSize, s.LastValue, s.Readable}
}

type SeqList struct {
	Status Status
	Reason string
	Rows   []Sequence
}

// Redacted reports sequences whose last value this account may not read.
func (l SeqList) Redacted() []Redaction {
	n := 0
	for _, s := range l.Rows {
		if !s.Readable {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return []Redaction{{ProbeID: SeqListID, Field: "last_value", Reason: ReasonInsufficientPrivilege, RowsAffected: n}}
}
