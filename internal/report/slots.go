package report

import (
	"fmt"
	"io"
	"sort"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// slotsView is the slots text layout (plan 2026-09-26-polish-remaining,
// stage 6): one row per slot, inactive ones first, then the ones keeping
// the most WAL.
type slotsView struct{ list facts.SlotList }

func (r *Report) SetSlots(l facts.SlotList) { r.slots = &slotsView{list: l} }

func (v *slotsView) write(w io.Writer) error {
	if v.list.Status != facts.StatusOK {
		writeNotOKAs(w, facts.SlotListID, v.list.Status, v.list.Reason)
		return nil
	}
	fmt.Fprintf(w, "\nslots: %d\n", len(v.list.Rows))
	if len(v.list.Rows) == 0 {
		return nil
	}
	rows := append([]facts.Slot(nil), v.list.Rows...)
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Active != b.Active {
			return !a.Active
		}
		if (a.RetainedWALBytes == nil) != (b.RetainedWALBytes == nil) {
			return a.RetainedWALBytes != nil
		}
		if a.RetainedWALBytes != nil && max(0, *a.RetainedWALBytes) != max(0, *b.RetainedWALBytes) {
			return *a.RetainedWALBytes > *b.RetainedWALBytes
		}
		return a.Name < b.Name
	})
	// catalog_xmin only matters for logical slots; the column appears when
	// some slot has one
	catalog := false
	for _, s := range rows {
		catalog = catalog || s.CatalogXmin != nil
	}
	header := []string{"name", "type", "active", "retained", "xmin"}
	if catalog {
		header = append(header, "catalog xmin")
	}
	header = append(header, "xmin age", "restart_lsn")
	out := make([][]string, len(rows))
	for i, s := range rows {
		active := "no"
		if s.Active {
			active = "yes"
			if s.ActivePID != nil {
				active = fmt.Sprintf("yes (pid %d)", *s.ActivePID)
			}
		}
		retained := "-"
		if s.RetainedWALBytes != nil {
			retained = size(max(0, float64(*s.RetainedWALBytes))) // see rule.Slots
		}
		row := []string{fitWidth(escapeControl(s.Name), maxName), cell(s.Type), active, retained, cell(s.Xmin)}
		if catalog {
			row = append(row, cell(s.CatalogXmin))
		}
		out[i] = append(row, cell(s.XminAge), cell(s.RestartLSN))
	}
	return writeTable(w, "  ", header, out)
}
