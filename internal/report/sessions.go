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
			groups[[4]string{name(s.Usename), name(s.Datname), name(s.ApplicationName), client(s)}]++
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
		// an untracked row keeps stale ages: they are not current
		age := func(v *float64) string {
			switch {
			case s.Masked() || s.Untracked():
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
			wait = cell(s.WaitEventType) + ":" + cell(s.WaitEvent)
		}
		sql := masked(s.Query)
		if sql == "" {
			sql = "-"
		}
		row := []string{fmt.Sprint(s.PID), name(s.Usename), name(s.Datname), name(s.ApplicationName), client(s)}
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
// when KES hides it, "-" for background processes, which have none.
func client(s facts.Session) string {
	switch {
	case s.Masked():
		return "?"
	case s.ClientAddr == nil:
		if k := kindOf(s); k == kindClient || k == kindWalsender {
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
