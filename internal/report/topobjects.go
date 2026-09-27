package report

import (
	"fmt"
	"io"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// writeTopObjects is the top-objects text layout (plan 2026-09-27 appendix
// B.8): the largest tables with what their size is made of, then the
// largest indexes.
func (r *Report) writeTopObjects(w io.Writer) error {
	in := " in " + escapeControl(r.Context.Database)
	if r.Context.Database == "" {
		in = ""
	}
	p := r.Data[facts.ObjectTablesID]
	if p.Status != facts.StatusOK {
		writeNotOKAs(w, "tables"+in, p.Status, reasonOf(p))
	} else {
		fmt.Fprintf(w, "\ntables%s: %d, largest first  (total = heap + indexes + TOAST)\n", in, len(p.Rows)+p.Truncated)
		var rows [][]string
		for _, row := range p.rows() {
			rows = append(rows, []string{fitWidth(escapeControl(cell(row["schemaname"])+"."+cell(row["relname"])), 2*maxName), relkind(cell(row["relkind"])),
				bytesCell(row["total_bytes"]), bytesCell(row["table_bytes"]), bytesCell(row["index_bytes"]), bytesCell(row["toast_bytes"]), cell(row["reltuples"])})
		}
		if len(rows) > 0 {
			if err := writeTable(w, "  ", []string{"table", "kind", "total", "heap", "indexes", "toast", "rows (est.)"}, rows); err != nil {
				return err
			}
		}
		writeTruncated(w, p.Truncated)
	}
	p = r.Data[facts.ObjectIndexesID]
	if p.Status != facts.StatusOK {
		writeNotOKAs(w, "indexes"+in, p.Status, reasonOf(p))
		return nil
	}
	fmt.Fprintf(w, "\nindexes%s: %d, largest first\n", in, len(p.Rows)+p.Truncated)
	var rows [][]string
	for _, row := range p.rows() {
		rows = append(rows, []string{fitWidth(escapeControl(cell(row["schemaname"])+"."+cell(row["relname"])), 2*maxName), name(row["table_name"]), bytesCell(row["bytes"])})
	}
	if len(rows) > 0 {
		if err := writeTable(w, "  ", []string{"index", "table", "size"}, rows); err != nil {
			return err
		}
	}
	writeTruncated(w, p.Truncated)
	return nil
}

// bytesCell is a size in readable units, "-" for NULL.
func bytesCell(v any) string {
	if b, ok := number(v); ok {
		return size(b)
	}
	return "-"
}
