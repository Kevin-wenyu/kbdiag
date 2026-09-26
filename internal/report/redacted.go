package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// fieldLabel folds a probe's redacted columns into the words a reader
// thinks in; columns not listed keep their own name.
type fieldLabel struct {
	label  string
	fields []string
}

var redactedLabels = map[string][]fieldLabel{
	facts.SessionActivityID: {
		{"state", []string{"state"}},
		{"backend_type", []string{"backend_type"}},
		{"client_addr", []string{"client_addr"}},
		{"ages", []string{"xact_age_s", "query_age_s", "state_age_s"}},
		{"wait", []string{"wait_event_type", "wait_event"}},
		{"query", []string{"query"}},
	},
	facts.LockListID:    {{"wait", []string{"wait_s"}}},
	facts.WaitSummaryID: {{"wait", []string{"wait_event_type", "wait_event"}}, {"state", []string{"state"}}},
}

// writeRedacted prints one line per probe and reason instead of one per
// column: "redacted: 12 rows of session.activity hide state, ... (reason)".
// JSON keeps one entry per column.
func writeRedacted(w io.Writer, rs []Redacted) {
	type key struct{ probe, reason string }
	type group struct {
		rows   int
		fields []string
	}
	var keys []key
	groups := map[key]*group{}
	for _, x := range rs {
		k := key{x.ProbeID, x.Reason}
		g, ok := groups[k]
		if !ok {
			g = &group{}
			groups[k] = g
			keys = append(keys, k)
		}
		g.rows = max(g.rows, x.RowsAffected)
		g.fields = append(g.fields, x.Field)
	}
	for _, k := range keys {
		g := groups[k]
		fmt.Fprintf(w, "redacted: %s of %s %s %s (%s%s)\n", plural(g.rows, "row", "rows"), k.probe,
			map[bool]string{true: "hides", false: "hide"}[g.rows == 1], strings.Join(labels(k.probe, g.fields), ", "), k.reason,
			map[bool]string{true: "; grant sys_monitor", false: ""}[k.reason == facts.ReasonInsufficientPrivilege])
	}
}

// labels maps fields to their labels, in the probe's label order, then any
// field without a label in the order given.
func labels(probe string, fields []string) []string {
	has := map[string]bool{}
	for _, f := range fields {
		has[f] = true
	}
	var out []string
	for _, l := range redactedLabels[probe] {
		for _, f := range l.fields {
			if has[f] {
				out = append(out, l.label)
				break
			}
		}
		for _, f := range l.fields {
			delete(has, f)
		}
	}
	for _, f := range fields {
		if has[f] {
			out = append(out, f)
			delete(has, f)
		}
	}
	return out
}
