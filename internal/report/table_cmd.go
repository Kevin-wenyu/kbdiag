package report

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// tableView is the table text layout (plan 2026-09-27 appendix B.9): what
// the table is and how big, then vacuum and analyze, then how it is read
// and written, then its indexes.
type tableView struct {
	i     facts.TableInfos
	z     facts.TableSizes
	s     facts.TableStats
	x     facts.TableIndexes
	l     facts.FreezeLimits
	v     facts.VacuumSettings
	found bool
}

func (r *Report) SetTable(i facts.TableInfos, z facts.TableSizes, s facts.TableStats, x facts.TableIndexes, l facts.FreezeLimits, v facts.VacuumSettings, found bool) {
	t := &tableView{i: i, z: z, s: s, x: x, l: l, v: v, found: found}
	r.layout = func(r *Report, w io.Writer) error { return t.write(w) }
}

func (t *tableView) write(w io.Writer) error {
	if t.i.Status != facts.StatusOK {
		writeNotOKAs(w, "table", t.i.Status, t.i.Reason)
		return nil
	}
	if !t.found {
		if len(t.i.Rows) == 1 {
			x := t.i.Rows[0]
			k := relkind(x.Relkind)
			article := "a"
			if strings.ContainsRune("aeiou", rune(k[0])) {
				article = "an"
			}
			fmt.Fprintf(w, "\ntable: %s is %s %s, not a table\n", escapeControl(x.Schemaname+"."+x.Relname), article, k)
		} else {
			fmt.Fprintln(w, "\ntable: not found")
		}
		return nil
	}
	x := t.i.Rows[0]
	kind := relkind(x.Relkind)
	if p := map[string]string{"u": "unlogged", "t": "temporary"}[x.Relpersistence]; p != "" {
		kind += ", " + p
	}
	fmt.Fprintf(w, "\n%s  (%s)\n", escapeControl(x.Schemaname+"."+x.Relname), kind)
	sz := "?  (table.size: " + string(t.z.Status) + ")"
	if t.z.Status == facts.StatusOK && len(t.z.Rows) > 0 {
		z := t.z.Rows[0]
		toast := "no TOAST"
		if z.ToastBytes != nil {
			toast = "TOAST " + size(float64(*z.ToastBytes))
		}
		sz = fmt.Sprintf("%s  (heap %s, indexes %s, %s)", size(float64(z.TotalBytes)), size(float64(z.TableBytes)), size(float64(z.IndexBytes)), toast)
		if x.Relkind == "p" {
			sz += "; a partitioned table holds nothing itself: its partitions are tables of their own"
		}
	}
	rows := strconv.FormatFloat(float64(x.Reltuples), 'f', -1, 32) + " estimated"
	var st *facts.TableStat
	if t.s.Status == facts.StatusOK && len(t.s.Rows) > 0 {
		st = &t.s.Rows[0]
		rows += fmt.Sprintf("; %d live, %d dead", st.NLiveTup, st.NDeadTup)
	}
	xid, mxid := "-", "-"
	if x.XIDAge != nil {
		xid = fmt.Sprint(*x.XIDAge)
		if t.l.Status == facts.StatusOK && len(t.l.Rows) > 0 {
			xid += fmt.Sprintf("  (autovacuum_freeze_max_age %d)", t.l.Rows[0].FreezeMaxAge)
		}
	}
	if x.MXIDAge != nil {
		mxid = fmt.Sprint(*x.MXIDAge)
	}
	opts := "-"
	if len(x.Reloptions) > 0 {
		opts = escapeControl(strings.Join(x.Reloptions, ", "))
	}
	writeKV(w, 0, [][2]string{
		{"size", sz},
		{"rows", rows}, {"xid age", xid}, {"mxid age", mxid}, {"reloptions", opts},
	})
	if t.z.Status != facts.StatusOK && strings.Contains(t.z.Reason, "55P03") {
		fmt.Fprintln(w, "  the table is locked exclusively (VACUUM FULL, TRUNCATE, ALTER TABLE ...): kbdiag locks shows who holds it")
	}
	switch {
	case st != nil:
		t.writeStats(w, x, *st)
	case t.s.Status == facts.StatusOK:
		fmt.Fprintln(w, "\nstatistics: none  (sys_stat_user_tables has no row for system catalogs, partitioned tables and TOAST tables)")
	default:
		writeNotOKAs(w, "statistics", t.s.Status, t.s.Reason)
	}
	return t.writeIndexes(w)
}

func (t *tableView) writeStats(w io.Writer, x facts.TableInfo, s facts.TableStat) {
	dead := fmt.Sprint(s.NDeadTup)
	if t.v.Status == facts.StatusOK && len(t.v.Rows) > 0 {
		th, on := rule.VacuumThreshold(facts.VacuumTable{Reltuples: x.Reltuples, Reloptions: x.Reloptions, NDeadTup: s.NDeadTup}, t.v.Rows[0])
		dead += ", autovacuum threshold " + strconv.FormatFloat(float64(th), 'f', -1, 32)
		if float32(s.NDeadTup) > th {
			dead += ": due"
		}
		switch {
		case t.v.Rows[0].Autovacuum != "on" || t.v.Rows[0].TrackCounts != "on":
			dead += " (autovacuum is off for the whole server)"
		case !on:
			dead += " (autovacuum is off for this table)"
		}
	}
	times := func(n int64) string {
		return map[bool]string{true: "1 time", false: fmt.Sprintf("%d times", n)}[n == 1]
	}
	fmt.Fprintln(w, "\nvacuum and analyze")
	writeKV(w, 0, [][2]string{
		{"dead tuples", dead},
		{"last vacuum", ago(s.LastVacuumAgeS) + "  (" + times(s.VacuumCount) + ")"},
		{"last autovacuum", ago(s.LastAutovacuumAgeS) + "  (" + times(s.AutovacuumCount) + ")"},
		{"last analyze", ago(s.LastAnalyzeAgeS) + "  (" + times(s.AnalyzeCount) + ")"},
		{"last autoanalyze", ago(s.LastAutoanalyzeAgeS) + "  (" + times(s.AutoanalyzeN) + ")"},
		{"modified since analyze", fmt.Sprint(s.NModSinceAnalyze)},
	})
	idx := "-"
	if s.IdxScan != nil {
		idx = fmt.Sprintf("%d  (%s rows fetched)", *s.IdxScan, cell(s.IdxTupFetch))
	}
	fmt.Fprintln(w, "\naccess since the statistics reset")
	writeKV(w, 0, [][2]string{
		{"seq scans", fmt.Sprintf("%d  (%d rows read)", s.SeqScan, s.SeqTupRead)},
		{"index scans", idx},
		{"rows written", fmt.Sprintf("%d inserted, %d updated (%d HOT), %d deleted", s.NTupIns, s.NTupUpd, s.NTupHotUpd, s.NTupDel)},
		{"heap blocks", blocks(s.HeapBlksRead, s.HeapBlksHit)},
		{"index blocks", blocks(s.IdxBlksRead, s.IdxBlksHit)},
	})
}

// blocks is "read, hit (hit %)"; the ratio keeps one decimal, rounded
// down, so a few reads never show as 100%.
func blocks(read, hit *int64) string {
	if read == nil || hit == nil {
		return "-"
	}
	s := fmt.Sprintf("%d read, %d hit", *read, *hit)
	if *read+*hit > 0 {
		s += fmt.Sprintf(" (%.1f%% hit)", math.Floor(float64(*hit)*1000/float64(*read+*hit))/10)
	}
	return s
}

func (t *tableView) writeIndexes(w io.Writer) error {
	if t.x.Status != facts.StatusOK {
		writeNotOKAs(w, "indexes", t.x.Status, t.x.Reason)
		return nil
	}
	fmt.Fprintf(w, "\nindexes: %d\n", len(t.x.Rows))
	if len(t.x.Rows) == 0 {
		return nil
	}
	var rows [][]string
	for _, i := range t.x.Rows {
		var kind []string
		switch {
		case i.IsPrimary:
			kind = append(kind, "primary key")
		case i.IsUnique:
			kind = append(kind, "unique")
		}
		if !i.IsValid {
			kind = append(kind, "INVALID")
		}
		k := "-"
		if len(kind) > 0 {
			k = strings.Join(kind, ", ")
		}
		rows = append(rows, []string{escapeControl(i.Name), sizeOf(i.Bytes), cell(i.IdxScan), k, fitWidth(escapeControl(i.Definition), 120)})
	}
	return writeTable(w, "  ", []string{"name", "size", "scans", "kind", "definition"}, rows)
}
