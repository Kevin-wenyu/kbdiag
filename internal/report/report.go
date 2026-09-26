// Package report is the output contract (PRD §5): one Report per command,
// rendered as text or JSON, plus the verdict → exit code mapping.
package report

import (
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

type Report struct {
	Command  string           `json:"command"`
	Verdict  rule.Verdict     `json:"verdict"`
	Context  Context          `json:"context"`
	Data     map[string]Probe `json:"data"`
	Findings []Finding        `json:"findings"`
	Redacted []Redacted       `json:"redacted"`

	// text layouts that need more than Data; not part of JSON
	sessions *sessionsView
	locks    *locksView
	session  *sessionView
	txn      *txnView
	waits    *waitsView
	slots    *slotsView
}

type Context struct {
	Version     string `json:"version"`
	Role        string `json:"role"`
	Location    string `json:"location"`
	User        string `json:"user"`
	CollectedAt string `json:"collected_at"`
}

type Probe struct {
	Status    facts.Status `json:"status"`
	Reason    *string      `json:"reason"`
	Columns   []string     `json:"columns"`
	Rows      [][]any      `json:"rows"`
	Truncated int          `json:"truncated"`
}

type Finding struct {
	ID       string     `json:"id"`
	Level    rule.Level `json:"level"`
	Symptom  string     `json:"symptom"`
	Evidence []Evidence `json:"evidence"`
	Cause    *string    `json:"cause"`
	Next     []Next     `json:"next"`
}

type Evidence struct {
	ProbeID string         `json:"probe_id"`
	Fields  map[string]any `json:"fields"`
}

type Next struct {
	Kind    string `json:"kind"`
	Command string `json:"command,omitempty"`
	SQL     string `json:"sql,omitempty"`
	Note    string `json:"note"`
}

type Redacted struct {
	ProbeID      string `json:"probe_id"`
	Field        string `json:"field"`
	Reason       string `json:"reason"`
	RowsAffected int    `json:"rows_affected"`
}

func New(command string, c facts.Context, r rule.Result) *Report {
	rep := &Report{
		Command: command,
		Verdict: r.Verdict,
		Context: Context{
			Version:     c.Version,
			Role:        c.Role,
			Location:    c.Location,
			User:        c.User,
			CollectedAt: c.CollectedAt.Truncate(time.Second).Format(time.RFC3339),
		},
		Data:     map[string]Probe{},
		Findings: []Finding{},
		Redacted: []Redacted{},
	}
	for _, f := range r.Findings {
		out := Finding{ID: f.ID, Level: f.Level, Symptom: f.Symptom, Cause: f.Cause, Evidence: []Evidence{}, Next: []Next{}}
		for _, e := range f.Evidence {
			out.Evidence = append(out.Evidence, Evidence{ProbeID: e.ProbeID, Fields: e.Fields})
		}
		for _, n := range f.Next {
			out.Next = append(out.Next, Next(n))
		}
		rep.Findings = append(rep.Findings, out)
	}
	return rep
}

// AddProbe records one probe's table. Rows beyond limit are counted in
// truncated, not shown; limit <= 0 shows everything. A probe that is not ok
// never carries rows (PRD §5).
func (r *Report) AddProbe(id string, st facts.Status, reason string, cols []string, rows [][]any, limit int) {
	p := Probe{Status: st, Columns: cols, Rows: [][]any{}}
	if reason != "" {
		p.Reason = &reason
	}
	if st == facts.StatusOK {
		if limit > 0 && len(rows) > limit {
			p.Truncated = len(rows) - limit
			rows = rows[:limit]
		}
		p.Rows = append(p.Rows, rows...)
	}
	r.Data[id] = p
}

func (r *Report) AddRedacted(rs []facts.Redaction) {
	for _, x := range rs {
		r.Redacted = append(r.Redacted, Redacted(x))
	}
}

// ExitCode maps a verdict to the Nagios-style exit code (PRD §5).
func ExitCode(v rule.Verdict) int {
	switch v {
	case rule.VerdictOK:
		return 0
	case rule.VerdictWARN:
		return 1
	case rule.VerdictFAIL:
		return 2
	default:
		return 3
	}
}

const (
	ExitUsage       = 64
	ExitUnavailable = 69
)
