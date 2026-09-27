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
	return left, r, true
}

// Seq fails a sequence that cannot hand out another value: nextval errors,
// so every insert that uses it fails now. The line is the sequence's own
// limit. One that cycles wraps instead. How close is too close has no
// objective line and is only shown.
func Seq(l facts.SeqList) Result {
	judge, unknown := collected(l.Status)
	if !judge {
		return Result{Verdict: verdictOf(nil, unknown)}
	}
	unknown = len(l.Redacted()) > 0
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

// seqFix widens the sequence: int and smallint become bigint (the column
// behind them first), a bigint gets its limit moved if it was set short of
// the type's. A name the text shows escaped is left out of the SQL.
func seqFix(s facts.Sequence) Next {
	name, note := qualified(s.Schemaname, s.Sequencename), ""
	if hasControl(s.Schemaname + s.Sequencename) {
		name, note = "<sequence>", "the name has control characters and is shown escaped: take it from kbdiag seq --json; "
	}
	if s.DataType != "bigint" {
		return Next{Kind: "fix", SQL: "ALTER SEQUENCE " + name + " AS bigint", Note: note + "the column it feeds must become bigint first (ALTER TABLE ... ALTER COLUMN ... TYPE bigint rewrites the table)"}
	}
	clause := "MAXVALUE <higher>"
	if s.IncrementBy < 0 {
		clause = "MINVALUE <lower>"
	}
	return Next{Kind: "fix", SQL: "ALTER SEQUENCE " + name + " " + clause, Note: note + "a bigint sequence at its limit: move the limit if it was set short of the type's, or plan a new key"}
}
