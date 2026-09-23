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
	for _, f := range r.Findings {
		fmt.Fprintf(w, "\n[%s] %s  %s\n", f.Level, f.ID, f.Symptom)
		if f.Cause != nil {
			fmt.Fprintf(w, "  cause: %s\n", *f.Cause)
		}
		for _, n := range f.Next {
			action := n.Command
			if action == "" {
				action = n.SQL
			}
			fmt.Fprintf(w, "  %s: %s  # %s\n", n.Kind, action, n.Note)
		}
	}
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
	for _, x := range r.Redacted {
		fmt.Fprintf(w, "\nredacted: %s.%s in %d rows (%s)", x.ProbeID, x.Field, x.RowsAffected, x.Reason)
	}
	if len(r.Redacted) > 0 {
		fmt.Fprintln(w)
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
// drive the terminal (ESC sequences clearing the screen, recoloring, ...).
func escapeControl(s string) string {
	if strings.IndexFunc(s, unicode.IsControl) < 0 {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case !unicode.IsControl(r):
			b.WriteRune(r)
		case r < 0x80:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			fmt.Fprintf(&b, `\u%04x`, r)
		}
	}
	return b.String()
}
