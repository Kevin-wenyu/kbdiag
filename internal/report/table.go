package report

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/text/width"
)

// writeTable prints an aligned table: every line starts with indent, columns
// are two spaces apart, and no line ends in spaces. Widths are
// terminal columns, so Chinese text stays aligned (tabwriter counts runes).
func writeTable(w io.Writer, indent string, header []string, rows [][]string) error {
	widths := make([]int, len(header))
	for _, r := range append([][]string{header}, rows...) {
		for i, c := range r {
			widths[i] = max(widths[i], displayWidth(c))
		}
	}
	for _, r := range append([][]string{header}, rows...) {
		var b strings.Builder
		b.WriteString(indent)
		for i, c := range r {
			b.WriteString(c)
			if i < len(r)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-displayWidth(c)+2))
			}
		}
		if _, err := fmt.Fprintln(w, strings.TrimRight(b.String(), " ")); err != nil {
			return err
		}
	}
	return nil
}

// maxName caps names (user, database, application) in tables, in terminal
// columns: one long application name must not push every other column off
// the screen. --json has the full value.
const maxName = 30

// name is cell cut to maxName terminal columns.
func name(v any) string { return fitWidth(cell(v), maxName) }

// fitWidth cuts s to at most w terminal columns, marking the cut with "...".
func fitWidth(s string, w int) string {
	if displayWidth(s) <= w {
		return s
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		rw := displayWidth(string(r))
		if n+rw > w-3 {
			break
		}
		b.WriteRune(r)
		n += rw
	}
	return b.String() + "..."
}

// displayWidth counts terminal columns: East Asian wide and fullwidth
// characters take two.
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			n += 2
		default:
			n++
		}
	}
	return n
}
