// Package rule turns facts into findings. It is pure: it must not import
// conn or probe, so every judgment is testable without a database.
package rule

import (
	"fmt"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

type Level string

const (
	LevelOK   Level = "OK"
	LevelWARN Level = "WARN"
	LevelFAIL Level = "FAIL"
)

type Verdict string

const (
	VerdictOK      Verdict = "OK"
	VerdictWARN    Verdict = "WARN"
	VerdictFAIL    Verdict = "FAIL"
	VerdictUNKNOWN Verdict = "UNKNOWN"
)

type Evidence struct {
	ProbeID string
	Fields  map[string]any
}

type Next struct {
	Kind    string // verify | fix
	Command string
	SQL     string
	Note    string
}

type Finding struct {
	ID       string
	Level    Level
	Symptom  string
	Evidence []Evidence
	Cause    *string
	Next     []Next
}

type Result struct {
	Verdict  Verdict
	Findings []Finding
}

// Thresholds are the defaults from docs/queries.md; flags override them.
type Thresholds struct {
	IdleInTxnWarnS float64
}

var Defaults = Thresholds{IdleInTxnWarnS: 300}

// Sessions flags sessions that sat idle in a transaction too long.
func Sessions(a facts.SessionActivity, th Thresholds) Result {
	switch a.Status {
	case facts.StatusNotApplicable:
		return Result{Verdict: VerdictOK}
	case facts.StatusOK:
	default:
		return Result{Verdict: VerdictUNKNOWN}
	}
	var fs []Finding
	masked := false
	for _, s := range a.Rows {
		if s.Masked() || s.Untracked() {
			masked = true
			continue
		}
		if s.State == nil || s.StateAgeS == nil {
			continue
		}
		if *s.State != "idle in transaction" && *s.State != "idle in transaction (aborted)" {
			continue
		}
		if *s.StateAgeS < th.IdleInTxnWarnS {
			continue
		}
		fs = append(fs, Finding{
			ID:      "session.idle_in_txn",
			Level:   LevelWARN,
			Symptom: fmt.Sprintf("会话 %d 处于 %s 已 %.0f 秒", s.PID, *s.State, *s.StateAgeS),
			Evidence: []Evidence{{ProbeID: facts.SessionActivityID, Fields: map[string]any{
				"pid": s.PID, "state": *s.State, "state_age_s": *s.StateAgeS, "backend_xid": s.BackendXID,
			}}},
			Next: []Next{{Kind: "verify", Command: fmt.Sprintf("kbdiag session %d", s.PID), Note: "看它持有哪些锁、有没有挡住别人"}},
		})
	}
	return Result{Verdict: verdictOf(fs, masked), Findings: fs}
}

// verdictOf takes the highest finding level; when nothing is WARN or FAIL and
// part of the input could not be seen, the answer is UNKNOWN, never OK.
func verdictOf(fs []Finding, unknown bool) Verdict {
	v := VerdictOK
	for _, f := range fs {
		switch f.Level {
		case LevelFAIL:
			return VerdictFAIL
		case LevelWARN:
			v = VerdictWARN
		}
	}
	if v == VerdictOK && unknown {
		return VerdictUNKNOWN
	}
	return v
}
