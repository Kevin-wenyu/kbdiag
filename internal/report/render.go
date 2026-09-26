package report

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// maxCell caps long text (SQL) in the text table; --json has the full value.
const maxCell = 60

func (r *Report) WriteText(w io.Writer) error {
	c := r.Context
	fmt.Fprintf(w, "%s  %s  (%s, %s, %s@%s, %s)\n", r.Command, r.Verdict, c.Version, c.Role, c.User, c.Location, c.CollectedAt)
	// Symptoms and next steps quote server strings (relation names, gids):
	// they are escaped like every table cell.
	for _, f := range r.Findings {
		fmt.Fprintf(w, "\n[%s] %s  %s\n", f.Level, f.ID, escapeControl(f.Symptom))
		if f.Cause != nil {
			fmt.Fprintf(w, "  cause: %s\n", escapeControl(*f.Cause))
		}
		for _, n := range f.Next {
			action := n.Command
			if action == "" {
				action = n.SQL
			}
			fmt.Fprintf(w, "  %s: %s  # %s\n", n.Kind, escapeControl(action), escapeControl(n.Note))
		}
	}
	switch {
	case r.sessions != nil:
		if err := r.sessions.write(w, r.Data[facts.SessionActivityID]); err != nil {
			return err
		}
		writeRedacted(w, r.Redacted)
		return nil
	case r.slots != nil:
		if err := r.slots.write(w); err != nil {
			return err
		}
		writeRedacted(w, r.Redacted)
		return nil
	case r.waits != nil:
		if err := r.waits.write(w); err != nil {
			return err
		}
		writeRedacted(w, r.Redacted)
		return nil
	case r.txn != nil:
		if err := r.txn.write(w); err != nil {
			return err
		}
		writeRedacted(w, r.Redacted)
		return nil
	case r.session != nil:
		if err := r.session.write(w); err != nil {
			return err
		}
		writeRedacted(w, r.Redacted)
		return nil
	case r.locks != nil:
		if err := r.locks.write(w, r.Data[facts.LockListID]); err != nil {
			return err
		}
		writeRedacted(w, r.Redacted)
		return nil
	case r.Command == "status":
		if err := r.writeStatus(w); err != nil {
			return err
		}
	default:
		if err := r.writeTables(w); err != nil {
			return err
		}
	}
	for _, x := range r.Redacted {
		fmt.Fprintf(w, "\nredacted: %s.%s in %d rows (%s)", x.ProbeID, x.Field, x.RowsAffected, x.Reason)
	}
	if len(r.Redacted) > 0 {
		fmt.Fprintln(w)
	}
	return nil
}

// writeNotOK prints a probe that was not collected, with its reason.
func writeNotOK(w io.Writer, id string, p Probe) {
	reason := ""
	if p.Reason != nil {
		reason = *p.Reason
	}
	writeNotOKAs(w, id, p.Status, reason)
}

// writeTables prints each probe as a table, in probe_id order.
func (r *Report) writeTables(w io.Writer) error {
	ids := make([]string, 0, len(r.Data))
	for id := range r.Data {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := r.Data[id]
		if p.Status != "ok" {
			reason := ""
			if p.Reason != nil {
				reason = *p.Reason
			}
			fmt.Fprintf(w, "\n%s: %s  %s\n", id, p.Status, reason)
			continue
		}
		fmt.Fprintf(w, "\n%s: %d rows\n", id, len(p.Rows))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, strings.Join(p.Columns, "\t"))
		for _, row := range p.Rows {
			cells := make([]string, len(row))
			for i, v := range row {
				cells[i] = cell(v)
			}
			fmt.Fprintln(tw, strings.Join(cells, "\t"))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if p.Truncated > 0 {
			fmt.Fprintf(w, "... %d more rows not shown (use --limit 0 to show all)\n", p.Truncated)
		}
	}
	return nil
}

// cell formats one value for the text table: NULL as "-", whitespace
// collapsed, long text cut to maxCell runes.
func cell(v any) string {
	if v == nil {
		return "-"
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return "-"
		}
		v = rv.Elem().Interface()
	}
	s := escapeControl(strings.Join(strings.Fields(fmt.Sprint(v)), " "))
	if rs := []rune(s); len(rs) > maxCell {
		s = string(rs[:maxCell-3]) + "..."
	}
	return s
}

// escapeControl makes control characters visible, so a query text cannot
// drive the terminal (ESC sequences clearing the screen, recoloring, ...),
// and so do format characters (bidi overrides that reorder what is shown)
// and the Unicode line and paragraph separators.
func escapeControl(s string) string {
	if strings.IndexFunc(s, hostile) < 0 {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case !hostile(r):
			b.WriteRune(r)
		case r < 0x80:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			fmt.Fprintf(&b, `\u%04x`, r)
		}
	}
	return b.String()
}

func hostile(r rune) bool {
	return unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp)
}
