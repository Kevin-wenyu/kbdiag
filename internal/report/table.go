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
