package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// txnView is what the txn text needs beyond Data: every session, so the
// oldest xid covers rows --limit leaves out.
type txnView struct {
	activity facts.SessionActivity
	prepared facts.TxnPrepared
	limit    int
}

// SetTxn makes the text output the txn layout (plan
// 2026-09-26-polish-remaining, stage 4): the oldest xid holding back the
// vacuum horizon and who holds it, the open transactions, the prepared ones.
func (r *Report) SetTxn(a facts.SessionActivity, p facts.TxnPrepared, limit int) {
	r.txn = &txnView{activity: a, prepared: p, limit: limit}
}

// olderXID compares transaction ids modulo 2^32, as the server does for
// normal xids. Live xids are within 2^31 of each other, so a linear scan
// finds the oldest; the special xids 0-2 never appear (NULL instead).
func olderXID(a, b uint32) bool { return int32(a-b) < 0 }

func (v *txnView) write(w io.Writer) error {
	v.writeOldest(w)
	if err := v.writeOpen(w); err != nil {
		return err
	}
	return v.writePrepared(w)
}

// writeOldest names the oldest xid among sessions' xid and xmin and the
// prepared transactions, and everything that holds exactly it.
func (v *txnView) writeOldest(w io.Writer) {
	type holder struct {
		xid   uint32
		label string
	}
	var hs []holder
	if v.prepared.Status == facts.StatusOK {
		for _, p := range v.prepared.Rows {
			hs = append(hs, holder{p.Transaction, "2PC " + fitWidth(escapeControl(p.GID), maxName)})
		}
	}
	if v.activity.Status == facts.StatusOK {
		rows := append([]facts.Session(nil), v.activity.Rows...)
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].PID < rows[j].PID })
		for _, s := range rows {
			if s.BackendXID != nil {
				hs = append(hs, holder{*s.BackendXID, fmt.Sprintf("%d xid", s.PID)})
			}
			if s.BackendXmin != nil {
				hs = append(hs, holder{*s.BackendXmin, fmt.Sprintf("%d xmin", s.PID)})
			}
		}
	}
	// A source we could not read may hold an older xid: say so rather than
	// name a false oldest (not_applicable prepared on a standby is fine).
	var missing []string
	if v.activity.Status != facts.StatusOK {
		missing = append(missing, facts.SessionActivityID)
	}
	if v.prepared.Status != facts.StatusOK && v.prepared.Status != facts.StatusNotApplicable {
		missing = append(missing, facts.TxnPreparedID)
	}
	caveat := ""
	if len(missing) > 0 {
		caveat = "; " + strings.Join(missing, ", ") + " not collected, an older one may exist"
	}
	if len(hs) == 0 {
		if len(missing) > 0 {
			fmt.Fprintf(w, "\noldest xid: unknown  (%s not collected)\n", strings.Join(missing, ", "))
		}
		return
	}
	oldest := hs[0].xid
	for _, h := range hs[1:] {
		if olderXID(h.xid, oldest) {
			oldest = h.xid
		}
	}
	var labels []string
	for _, h := range hs {
		if h.xid == oldest {
			labels = append(labels, h.label)
		}
	}
	fmt.Fprintf(w, "\noldest xid: %d  (%s%s)\n", oldest, strings.Join(labels, ", "), caveat)
}

// writeOpen lists the sessions inside a transaction: an xact start, an xid
// or an xmin. A hidden session shows only its xid and xmin, which KES does
// not mask; one without either is only counted.
func (v *txnView) writeOpen(w io.Writer) error {
	if v.activity.Status != facts.StatusOK {
		writeNotOKAs(w, "open transactions", v.activity.Status, v.activity.Reason)
		return nil
	}
	var shown []facts.Session
	hidden := 0
	for _, s := range v.activity.Rows {
		ids := s.BackendXID != nil || s.BackendXmin != nil
		switch {
		case s.Masked() || s.Untracked():
			if ids {
				shown = append(shown, s)
			} else {
				hidden++
			}
		case ids || s.XactAgeS != nil:
			shown = append(shown, s)
		}
	}
	// hidden are sessions we cannot see at all, mostly idle: not transactions
	fmt.Fprintf(w, "\nopen transactions: %d", len(shown))
	if hidden > 0 {
		fmt.Fprintf(w, ", %s hidden", plural(hidden, "session", "sessions"))
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
	rows := make([][]string, len(shown))
	for i, s := range shown {
		state, xact, sql := cell(s.State), "-", cell(s.Query)
		switch {
		case s.Masked():
			state, xact, sql = "?", "?", "?"
		case s.Untracked():
			xact, sql = "?", "?" // its xact age is stale
		default:
			if s.XactAgeS != nil {
				xact = duration(*s.XactAgeS)
			}
			if sql == "" {
				sql = "-"
			}
		}
		rows[i] = []string{fmt.Sprint(s.PID), name(s.Usename), name(s.Datname), name(s.ApplicationName),
			state, xact, cell(s.BackendXID), cell(s.BackendXmin), sql}
	}
	if err := writeTable(w, "  ", []string{"pid", "user", "database", "application", "state", "xact", "xid", "xmin", "sql"}, rows); err != nil {
		return err
	}
	if more > 0 {
		fmt.Fprintf(w, "... %d more rows not shown (use --limit 0 to show all)\n", more)
	}
	return nil
}

func (v *txnView) writePrepared(w io.Writer) error {
	if v.prepared.Status != facts.StatusOK {
		writeNotOKAs(w, "prepared", v.prepared.Status, v.prepared.Reason)
		return nil
	}
	fmt.Fprintf(w, "\nprepared: %d\n", len(v.prepared.Rows))
	if len(v.prepared.Rows) == 0 {
		return nil
	}
	rows := make([][]string, len(v.prepared.Rows))
	for i, p := range v.prepared.Rows {
		rows[i] = []string{fitWidth(escapeControl(p.GID), maxName), name(p.Owner), name(p.Database), duration(p.AgeS), fmt.Sprint(p.Transaction)}
	}
	return writeTable(w, "  ", []string{"gid", "owner", "database", "age", "xid"}, rows)
}

// writeNotOKAs prints a section that was not collected under its title.
func writeNotOKAs(w io.Writer, title string, st facts.Status, reason string) {
	fmt.Fprintf(w, "\n%s: %s", title, st)
	if reason != "" {
		fmt.Fprintf(w, "  (%s)", escapeControl(reason))
	}
	fmt.Fprintln(w)
}
