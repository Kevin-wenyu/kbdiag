package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// sessionsView is what the sessions text needs beyond Data: every row, not
// only the ones --limit leaves in the JSON, so the summary counts them all.
type sessionsView struct {
	rows  []facts.Session
	all   bool
	limit int
}

// SetSessions makes the text output the sessions layout (plan
// 2026-09-26-polish-remaining appendix A): who holds the connections, then
// the sessions doing something. all lists every session instead; limit
// trims only that list.
func (r *Report) SetSessions(rows []facts.Session, all bool, limit int) {
	r.sessions = &sessionsView{rows: rows, all: all, limit: limit}
}

type sessionKind int

const (
	kindClient sessionKind = iota
	kindWalsender
	kindBackground
	kindHidden // masked: KES does not tell us what it is
)

func kindOf(s facts.Session) sessionKind {
	switch {
	case s.Masked() || s.BackendType == nil:
		return kindHidden
	case *s.BackendType == "client backend":
		return kindClient
	case *s.BackendType == "walsender":
		return kindWalsender
	}
	return kindBackground
}

func (v *sessionsView) write(w io.Writer, p Probe) error {
	if p.Status != "ok" {
		writeNotOK(w, facts.SessionActivityID, p)
		return nil
	}
	if err := v.writeSummary(w); err != nil {
		return err
	}
	return v.writeList(w)
}

// writeSummary counts sessions by kind, and groups client and hidden ones by
// user, database, application and client, most first.
func (v *sessionsView) writeSummary(w io.Writer) error {
	var n [4]int
	groups := map[[4]string]int{}
	for _, s := range v.rows {
		k := kindOf(s)
		n[k]++
		if k == kindClient || k == kindHidden {
			groups[[4]string{cell(s.Usename), cell(s.Datname), cell(s.ApplicationName), client(s)}]++
		}
	}
	var parts []string
	if n[kindClient] > 0 || n[kindHidden] == 0 {
		parts = append(parts, plural(n[kindClient], "client session", "client sessions")+" (not counting kbdiag)")
	}
	if n[kindWalsender] > 0 {
		parts = append(parts, plural(n[kindWalsender], "walsender", "walsenders"))
	}
	if n[kindBackground] > 0 {
		parts = append(parts, fmt.Sprintf("%d background", n[kindBackground]))
	}
	if n[kindHidden] > 0 {
		parts = append(parts, fmt.Sprintf("%d hidden", n[kindHidden]))
	}
	fmt.Fprintf(w, "\nconnected: %s\n", strings.Join(parts, ", "))
	if len(groups) == 0 {
		return nil
	}
	keys := make([][4]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if groups[keys[i]] != groups[keys[j]] {
			return groups[keys[i]] > groups[keys[j]]
		}
		for x := range keys[i] {
			if keys[i][x] != keys[j][x] {
				return keys[i][x] < keys[j][x]
			}
		}
		return false
	})
	rows := make([][]string, len(keys))
	for i, k := range keys {
		rows[i] = []string{fmt.Sprint(groups[k]), k[0], k[1], k[2], k[3]}
	}
	return writeTable(w, "  ", []string{"count", "user", "database", "application", "client"}, rows)
}

// writeList lists the client sessions that are not idle, longest
// transaction first (the probe's order); with --all, every session.
func (v *sessionsView) writeList(w io.Writer) error {
	var shown []facts.Session
	hidden := 0
	for _, s := range v.rows {
		switch k := kindOf(s); {
		case v.all:
			shown = append(shown, s)
		case k == kindHidden:
			hidden++
		case k == kindClient && (s.State == nil || *s.State != "idle"):
			shown = append(shown, s)
		}
	}
	title := "not idle"
	if v.all {
		title = "all"
	}
	fmt.Fprintf(w, "\n%s: %d", title, len(shown))
	if hidden > 0 {
		fmt.Fprintf(w, " visible, %d hidden", hidden)
	}
	fmt.Fprintln(w)
	if len(shown) == 0 {
		return nil
	}
	more := 0
	if v.limit > 0 && len(shown) > v.limit {
		more = len(shown) - v.limit
		shown = shown[:v.limit]
	}
	header := []string{"pid", "user", "database", "application", "client", "state", "xact", "query", "wait", "sql"}
	if v.all {
		header = []string{"pid", "user", "database", "application", "client", "type", "state", "xact", "query", "wait", "sql"}
	}
	rows := make([][]string, len(shown))
	for i, s := range shown {
		masked := func(v any) string {
			if s.Masked() {
				return "?"
			}
			return cell(v)
		}
		age := func(v *float64) string {
			switch {
			case s.Masked():
				return "?"
			case v == nil:
				return "-"
			}
			return duration(*v)
		}
		wait := "-"
		if s.Masked() {
			wait = "?"
		} else if s.State != nil && *s.State == "active" && s.WaitEventType != nil {
			wait = *s.WaitEventType + ":" + cell(s.WaitEvent)
		}
		sql := masked(s.Query)
		if sql == "" {
			sql = "-"
		}
		row := []string{fmt.Sprint(s.PID), cell(s.Usename), cell(s.Datname), cell(s.ApplicationName), client(s)}
		if v.all {
			row = append(row, masked(s.BackendType))
		}
		rows[i] = append(row, masked(s.State), age(s.XactAgeS), age(s.QueryAgeS), wait, sql)
	}
	if err := writeTable(w, "  ", header, rows); err != nil {
		return err
	}
	if more > 0 {
		fmt.Fprintf(w, "... %d more rows not shown (use --limit 0 to show all)\n", more)
	}
	return nil
}

// client is where the session comes from: "local" for a Unix socket, "?"
// when KES hides it.
func client(s facts.Session) string {
	switch {
	case s.Masked():
		return "?"
	case s.ClientAddr == nil:
		if kindOf(s) == kindClient {
			return "local"
		}
		return "-"
	}
	return cell(s.ClientAddr)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// sessionFieldLabels folds the redacted session.activity columns into the
// words a reader thinks in, in the order the draft uses.
var sessionFieldLabels = []struct {
	label  string
	fields []string
}{
	{"state", []string{"state"}},
	{"backend_type", []string{"backend_type"}},
	{"client_addr", []string{"client_addr"}},
	{"ages", []string{"xact_age_s", "query_age_s", "state_age_s"}},
	{"wait", []string{"wait_event_type", "wait_event"}},
	{"query", []string{"query"}},
}

// writeSessionsRedacted prints one line per reason instead of one per column.
func writeSessionsRedacted(w io.Writer, rs []Redacted) {
	type group struct {
		rows   int
		fields map[string]bool
	}
	var reasons []string
	groups := map[string]*group{}
	for _, x := range rs {
		g, ok := groups[x.Reason]
		if !ok {
			g = &group{fields: map[string]bool{}}
			groups[x.Reason] = g
			reasons = append(reasons, x.Reason)
		}
		g.rows = max(g.rows, x.RowsAffected)
		g.fields[x.Field] = true
	}
	for _, reason := range reasons {
		g := groups[reason]
		var labels []string
		for _, l := range sessionFieldLabels {
			for _, f := range l.fields {
				if g.fields[f] {
					labels = append(labels, l.label)
					break
				}
			}
		}
		hint := ""
		if reason == facts.ReasonInsufficientPrivilege {
			hint = "; grant sys_monitor"
		}
		fmt.Fprintf(w, "redacted: %s of %s hide %s (%s%s)\n", plural(g.rows, "row", "rows"), facts.SessionActivityID,
			strings.Join(labels, ", "), reason, hint)
	}
}
