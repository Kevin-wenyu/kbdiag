package report

import (
	"fmt"
	"io"
	"strings"

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
		lockHint(w, p)
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
		for _, row := range p.rows() {
			if cell(row["relkind"]) == "p" {
				fmt.Fprintln(w, "  a partitioned table holds nothing itself: its partitions are listed on their own")
				break
			}
		}
	}
	p = r.Data[facts.ObjectIndexesID]
	if p.Status != facts.StatusOK {
		writeNotOKAs(w, "indexes"+in, p.Status, reasonOf(p))
		lockHint(w, p)
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

// lockHint points at locks when a size function waited out lock_timeout:
// some relation is held under an exclusive lock.
func lockHint(w io.Writer, p Probe) {
	if strings.Contains(reasonOf(p), "55P03") {
		fmt.Fprintln(w, "  a relation is locked exclusively (VACUUM FULL, TRUNCATE, ALTER TABLE ...): kbdiag locks shows who holds it")
	}
}
