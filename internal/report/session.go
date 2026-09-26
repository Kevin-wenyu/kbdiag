package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// sessionView is what the session <pid> text needs: the session's row and
// every lock row that involves it, as facts.
type sessionView struct {
	pid      int32
	activity facts.SessionActivity
	locks    facts.LockList
	found    bool
}

// SetSession makes the text output the session layout (plan
// 2026-09-26-polish-remaining, stage 3): who it is and what it does, its
// full SQL, what it waits for, whom it blocks, what it holds.
func (r *Report) SetSession(pid int32, a facts.SessionActivity, l facts.LockList, found bool) {
	r.session = &sessionView{pid: pid, activity: a, locks: l, found: found}
}

func (v *sessionView) write(w io.Writer) error {
	if !v.found {
		fmt.Fprintf(w, "\nsession %d: not found (it may have ended)\n", v.pid)
		return nil
	}
	if v.activity.Status != facts.StatusOK {
		p := Probe{Status: v.activity.Status}
		if v.activity.Reason != "" {
			p.Reason = &v.activity.Reason
		}
		writeNotOK(w, facts.SessionActivityID, p)
	} else {
		for _, s := range v.activity.Rows {
			v.writeActivity(w, s)
		}
	}
	if v.locks.Status != facts.StatusOK {
		p := Probe{Status: v.locks.Status}
		if v.locks.Reason != "" {
			p.Reason = &v.locks.Reason
		}
		writeNotOK(w, facts.LockListID, p)
		return nil
	}
	if err := v.writeWaitingFor(w); err != nil {
		return err
	}
	if err := v.writeBlocking(w); err != nil {
		return err
	}
	return v.writeHolds(w)
}

func (v *sessionView) writeActivity(w io.Writer, s facts.Session) {
	hidden := func(x any) string {
		if s.Masked() {
			return "?"
		}
		return cell(x)
	}
	age := func(x *float64) string {
		switch {
		case s.Masked() || s.Untracked():
			return "?"
		case x == nil:
			return "-"
		}
		return duration(*x)
	}
	state := hidden(s.State)
	if !s.Masked() && s.State != nil && s.StateAgeS != nil {
		state += "  (for " + duration(*s.StateAgeS) + ")"
	}
	wait := "-"
	if s.Masked() {
		wait = "?"
	} else if s.State != nil && *s.State == "active" && s.WaitEventType != nil {
		wait = cell(s.WaitEventType) + ":" + cell(s.WaitEvent)
	}
	fmt.Fprintf(w, "\nsession %d\n", s.PID)
	writeKV(w, 0, [][2]string{
		{"user", name(s.Usename)},
		{"database", name(s.Datname)},
		{"application", name(s.ApplicationName)},
		{"client", client(s)},
		{"type", hidden(s.BackendType)},
		{"state", state},
		{"xact", age(s.XactAgeS)},
		{"query", age(s.QueryAgeS)},
		{"wait", wait},
		{"xid / xmin", cell(s.BackendXID) + " / " + cell(s.BackendXmin)},
	})

	// The whole statement: this is the command for reading one session's SQL.
	title := "sql"
	if !s.Masked() && !s.Untracked() && s.State != nil && *s.State != "active" {
		title = "last sql" // idle ones show the statement that already ended
	}
	fmt.Fprintf(w, "\n%s\n", title)
	switch {
	case s.Masked():
		fmt.Fprintln(w, "  ?")
	case s.Query == nil || strings.TrimSpace(*s.Query) == "":
		fmt.Fprintln(w, "  -")
	default:
		for _, line := range sqlLines(*s.Query) {
			fmt.Fprintln(w, "  "+line)
		}
	}
}

// sqlLines keeps the statement's own line breaks, expands tabs and makes
// every other control character visible.
func sqlLines(q string) []string {
	q = strings.ReplaceAll(strings.ReplaceAll(q, "\r\n", "\n"), "\t", "    ")
	lines := strings.Split(strings.TrimRight(q, "\n"), "\n")
	for i, l := range lines {
		lines[i] = escapeControl(strings.TrimRight(l, " "))
	}
	return lines
}

func (v *sessionView) mine(l facts.Lock) bool { return l.PID != nil && *l.PID == v.pid }

// writeWaitingFor lists the locks this session waits for.
func (v *sessionView) writeWaitingFor(w io.Writer) error {
	var rows [][]string
	for _, l := range v.locks.Rows {
		if !v.mine(l) || l.Granted {
			continue
		}
		var by []string
		for _, b := range l.Blockers() {
			by = append(by, blockerName(b))
		}
		blockedBy := "-"
		if len(by) > 0 {
			blockedBy = strings.Join(by, ", ")
		}
		rows = append(rows, []string{objectName(l), l.Mode, waited(l), blockedBy})
	}
	fmt.Fprintf(w, "\nwaiting for: %d\n", len(rows))
	if len(rows) == 0 {
		return nil
	}
	return writeTable(w, "  ", []string{"object", "wants", "waited", "blocked by"}, rows)
}

// writeBlocking lists the sessions waiting behind this one.
func (v *sessionView) writeBlocking(w io.Writer) error {
	var rows [][]string
	for _, l := range v.locks.Rows {
		if l.Granted || l.PID == nil || v.mine(l) {
			continue
		}
		for _, b := range l.Blockers() {
			if b == v.pid {
				rows = append(rows, []string{fmt.Sprint(*l.PID), objectName(l), l.Mode, waited(l)})
				break
			}
		}
	}
	fmt.Fprintf(w, "\nblocking: %d\n", len(rows))
	if len(rows) == 0 {
		return nil
	}
	return writeTable(w, "  ", []string{"pid", "object", "wants", "waited"}, rows)
}

// writeHolds lists the locks this session holds. Every transaction holds
// its own virtualxid, and a writing one its transactionid; those are left
// out unless someone waits for them (the xid is shown above).
func (v *sessionView) writeHolds(w io.Writer) error {
	contested := map[lockObject]bool{}
	for _, l := range v.locks.Rows {
		if !l.Granted && !v.mine(l) {
			contested[objectOf(l)] = true
		}
	}
	var rows [][]string
	for _, l := range v.locks.Rows {
		if !v.mine(l) || !l.Granted {
			continue
		}
		if (l.Locktype == "virtualxid" || l.Locktype == "transactionid") && !contested[objectOf(l)] {
			continue
		}
		rows = append(rows, []string{objectName(l), l.Mode})
	}
	fmt.Fprintf(w, "\nholds: %d\n", len(rows))
	if len(rows) == 0 {
		return nil
	}
	return writeTable(w, "  ", []string{"object", "mode"}, rows)
}

func objectName(l facts.Lock) string { return fitWidth(escapeControl(objectOf(l).label()), maxName) }

func waited(l facts.Lock) string {
	if l.WaitS == nil {
		return "?" // masked, or untracked: the wait is unseen
	}
	return duration(*l.WaitS)
}
