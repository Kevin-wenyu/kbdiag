package report

import (
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// writeFreeze is the freeze text layout (plan 2026-09-27 appendix B.2): the
// lines the ages are measured against, every database oldest first, then
// the oldest tables of the current one.
func (r *Report) writeFreeze(w io.Writer) error {
	maxAge := 0.0
	if p := r.Data[facts.FreezeLimitsID]; p.Status != facts.StatusOK {
		writeNotOKAs(w, "limits", p.Status, reasonOf(p))
	} else {
		for _, row := range p.rows() {
			maxAge, _ = number(row["autovacuum_freeze_max_age"])
			fmt.Fprintln(w, "\nlimits")
			writeKV(w, 0, [][2]string{
				{"autovacuum_freeze_max_age", cell(row["autovacuum_freeze_max_age"])},
				{"autovacuum_multixact_freeze_max_age", cell(row["autovacuum_multixact_freeze_max_age"])},
				{"xid stop limit", fmt.Sprintf("%d  (new transaction IDs are refused)", facts.XIDStopAge)},
			})
		}
	}
	if p := r.Data[facts.FreezeDatabasesID]; p.Status != facts.StatusOK {
		writeNotOKAs(w, "databases", p.Status, reasonOf(p))
	} else {
		fmt.Fprintf(w, "\ndatabases: %d\n", len(p.Rows))
		dbs := p.rows()
		sort.SliceStable(dbs, func(i, j int) bool {
			a, _ := number(dbs[i]["xid_age"])
			b, _ := number(dbs[j]["xid_age"])
			if a != b {
				return a > b
			}
			return cell(dbs[i]["datname"]) < cell(dbs[j]["datname"])
		})
		var rows [][]string
		for _, row := range dbs {
			age, _ := number(row["xid_age"])
			of := "-"
			if maxAge > 0 {
				of = fmt.Sprintf("%.0f%%", math.Floor(age*100/maxAge))
			}
			rows = append(rows, []string{name(row["datname"]), cell(row["xid_age"]), of, cell(row["mxid_age"]),
				fmt.Sprint(max(0, int64(facts.XIDStopAge)-int64(age)))})
		}
		if len(rows) > 0 {
			if err := writeTable(w, "  ", []string{"name", "xid age", "of freeze max", "mxid age", "xids left"}, rows); err != nil {
				return err
			}
		}
	}
	p := r.Data[facts.FreezeTablesID]
	title := "tables in " + escapeControl(r.Context.Database)
	if r.Context.Database == "" {
		title = "tables"
	}
	if p.Status != facts.StatusOK {
		writeNotOKAs(w, title, p.Status, reasonOf(p))
		return nil
	}
	fmt.Fprintf(w, "\n%s: %d, oldest first\n", title, len(p.Rows)+p.Truncated)
	var rows [][]string
	for _, row := range p.rows() {
		v, _ := number(row["heap_bytes_est"])
		sz := size(v)
		rows = append(rows, []string{fitWidth(cell(row["relation"]), 2*maxName), relkind(cell(row["relkind"])), cell(row["xid_age"]), cell(row["mxid_age"]), sz})
	}
	if len(rows) > 0 {
		if err := writeTable(w, "  ", []string{"relation", "kind", "xid age", "mxid age", "heap (est.)"}, rows); err != nil {
			return err
		}
	}
	writeTruncated(w, p.Truncated)
	return nil
}

// relkind names sys_class.relkind for a reader.
func relkind(k string) string {
	if n, ok := map[string]string{"r": "table", "p": "partitioned", "m": "matview", "t": "toast", "i": "index", "I": "partitioned index", "v": "view", "S": "sequence", "f": "foreign"}[k]; ok {
		return n
	}
	return k
}

func reasonOf(p Probe) string {
	if p.Reason == nil {
		return ""
	}
	return *p.Reason
}

func writeTruncated(w io.Writer, n int) {
	if n > 0 {
		fmt.Fprintf(w, "  ... %d more not shown (--limit 0 shows all)\n", n)
	}
}
