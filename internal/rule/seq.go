package rule

import (
	"fmt"
	"math/big"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// SeqLeft is how many more values nextval can hand out before the sequence
// passes its limit, and how much of its range is used. Exact arithmetic:
// bigint sequences span the whole int64 range. ok is false for a sequence
// never called (or not readable).
func SeqLeft(s facts.Sequence) (left *big.Int, used float64, ok bool) {
	if s.LastValue == nil || s.IncrementBy == 0 {
		return nil, 0, false
	}
	last, inc := big.NewInt(*s.LastValue), big.NewInt(s.IncrementBy)
	var room, span *big.Int
	if s.IncrementBy > 0 {
		room = new(big.Int).Sub(big.NewInt(s.MaxValue), last)
		span = new(big.Int).Sub(big.NewInt(s.MaxValue), big.NewInt(s.StartValue))
	} else {
		room = new(big.Int).Sub(last, big.NewInt(s.MinValue))
		span = new(big.Int).Sub(big.NewInt(s.StartValue), big.NewInt(s.MinValue))
		inc.Neg(inc)
	}
	if room.Sign() < 0 {
		room.SetInt64(0)
	}
	left = new(big.Int).Quo(room, inc)
	if span.Sign() <= 0 {
		return left, 1, true
	}
	r, _ := new(big.Float).Quo(new(big.Float).SetInt(new(big.Int).Sub(span, room)), new(big.Float).SetInt(span)).Float64()
	// a value before the start (a cycle wrapped, a setval below it) counts
	// as nothing used
	return left, min(1, max(0, r)), true
}

// Seq fails a sequence that cannot hand out another value: nextval errors,
// so every insert that uses it fails now. The line is the sequence's own
// limit. One that cycles wraps instead. How close is too close has no
// objective line and is only shown. Not judged on a standby: its copy of a
// sequence is the WAL record, written up to 32 values ahead of the
// primary's, so it can read "at the limit" while the primary still has
// values, and a standby inserts nothing anyway.
func Seq(l facts.SeqList, role string) Result {
	judge, unknown := collected(l.Status)
	if !judge {
		return Result{Verdict: verdictOf(nil, unknown)}
	}
	unknown = len(l.Redacted()) > 0
	if role == "standby" {
		return Result{Verdict: verdictOf(nil, unknown)}
	}
	var fs []Finding
	for _, s := range l.Rows {
		left, _, ok := SeqLeft(s)
		if !ok || s.Cycle || left.Sign() > 0 {
			continue
		}
		name := s.Schemaname + "." + s.Sequencename
		limit := s.MaxValue
		if s.IncrementBy < 0 {
			limit = s.MinValue
		}
		fs = append(fs, Finding{
			ID:    "seq.exhausted",
			Level: LevelFAIL,
			Symptom: fmt.Sprintf("sequence %s (%s) is at %d, its limit is %d: nextval fails, so inserts that use it fail",
				name, s.DataType, *s.LastValue, limit),
			Evidence: []Evidence{{ProbeID: facts.SeqListID, Fields: map[string]any{
				"schemaname": s.Schemaname, "sequencename": s.Sequencename, "data_type": s.DataType, "last_value": *s.LastValue,
				"min_value": s.MinValue, "max_value": s.MaxValue, "increment_by": s.IncrementBy,
			}}},
			Next: []Next{seqFix(s)},
		})
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}

// seqFix moves the limit that stops the sequence: its own MAXVALUE
// (MINVALUE descending) when it was set short of the type's bound, else the
// type, int and smallint to bigint (the column behind them first). A name
// the text shows escaped is left out of the SQL.
func seqFix(s facts.Sequence) Next {
	name, note := qualified(s.Schemaname, s.Sequencename), ""
	if hasControl(s.Schemaname + s.Sequencename) {
		name, note = "<sequence>", "the name has control characters and is shown escaped: take it from kbdiag seq --json; "
	}
	lo, hi := typeBounds(s.DataType)
	own := s.IncrementBy > 0 && s.MaxValue < hi || s.IncrementBy < 0 && s.MinValue > lo
	if !own && s.DataType != "bigint" {
		return Next{Kind: "fix", SQL: "ALTER SEQUENCE " + name + " AS bigint", Note: note + "the column it feeds must become bigint first (ALTER TABLE ... ALTER COLUMN ... TYPE bigint rewrites the table)"}
	}
	clause, what := "MAXVALUE <higher>", "its MAXVALUE"
	if s.IncrementBy < 0 {
		clause, what = "MINVALUE <lower>", "its MINVALUE"
	}
	if !own {
		return Next{Kind: "fix", SQL: "ALTER SEQUENCE " + name + " " + clause, Note: note + "a bigint sequence at the type's bound: plan a new key"}
	}
	return Next{Kind: "fix", SQL: "ALTER SEQUENCE " + name + " " + clause, Note: note + "it stops at " + what + ", short of what its type allows"}
}

// typeBounds are the ranges of the sequence types.
func typeBounds(t string) (lo, hi int64) {
	switch t {
	case "smallint":
		return -1 << 15, 1<<15 - 1
	case "integer":
		return -1 << 31, 1<<31 - 1
	}
	return -1 << 63, 1<<63 - 1
}
