package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// locksView is what the locks text needs beyond Data: every lock row, so
// the blockers section can say what each blocker holds.
type locksView struct {
	rows  []facts.Lock
	limit int
}

// SetLocks makes the text output the locks layout (plan
// 2026-09-26-polish-remaining, stage 2): who blocks the most, then every
// waiting session, longest wait first. limit trims only the waiting list.
func (r *Report) SetLocks(rows []facts.Lock, limit int) {
	r.locks = &locksView{rows: rows, limit: limit}
}

type lockObject struct{ locktype, relation string }

func objectOf(l facts.Lock) lockObject {
	return lockObject{l.Locktype, deref(l.Relation)}
}

// label names the object: the relation, or the lock type for the others
// (advisory, transactionid, ...), which carry no relation.
func (o lockObject) label() string {
	if o.relation != "" {
		return o.relation
	}
	return o.locktype
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// blockerName shows the prepared-transaction blocker (pid 0) as 2PC.
func blockerName(pid int32) string {
	if pid == facts.PreparedBlocker {
		return "2PC"
	}
	return fmt.Sprint(pid)
}

func (v *locksView) write(w io.Writer, p Probe) error {
	if p.Status != "ok" {
		writeNotOK(w, facts.LockListID, p)
		return nil
	}
	var waiting []facts.Lock
	for _, l := range v.rows {
		if !l.Granted && l.PID != nil {
			waiting = append(waiting, l)
		}
	}
	if len(waiting) > 0 {
		if err := v.writeBlockers(w, waiting); err != nil {
			return err
		}
	}
	return v.writeWaiting(w, waiting)
}

// writeBlockers counts, per direct blocker, the waiting sessions it blocks,
// and shows what it holds on the objects they want.
func (v *locksView) writeBlockers(w io.Writer, waiting []facts.Lock) error {
	blocks := map[int32]int{}
	wanted := map[int32]map[lockObject]bool{}
	for _, l := range waiting {
		for _, b := range l.BlockedBy {
			blocks[b]++
			if wanted[b] == nil {
				wanted[b] = map[lockObject]bool{}
			}
			wanted[b][objectOf(l)] = true
		}
	}
	pids := make([]int32, 0, len(blocks))
	for b := range blocks {
		pids = append(pids, b)
	}
	sort.Slice(pids, func(i, j int) bool {
		if blocks[pids[i]] != blocks[pids[j]] {
			return blocks[pids[i]] > blocks[pids[j]]
		}
		return pids[i] < pids[j]
	})
	fmt.Fprintf(w, "\nblockers: %d\n", len(pids))
	rows := make([][]string, len(pids))
	for i, b := range pids {
		rows[i] = []string{blockerName(b), fmt.Sprint(blocks[b]), v.holds(b, wanted[b])}
	}
	return writeTable(w, "  ", []string{"pid", "blocks", "holds"}, rows)
}

// holds lists the granted locks blocker b has on the wanted objects; a
// blocker that holds none there is queued ahead of the waiter. A prepared
// transaction's locks have no pid.
func (v *locksView) holds(b int32, wanted map[lockObject]bool) string {
	var held, queued []string
	seen := map[string]bool{}
	for _, l := range v.rows {
		mine := (b == facts.PreparedBlocker && l.PID == nil) || (l.PID != nil && *l.PID == b)
		if !mine || !wanted[objectOf(l)] {
			continue
		}
		s := fitWidth(escapeControl(objectOf(l).label()), maxName) + " " + l.Mode
		if seen[s] {
			continue
		}
		seen[s] = true
		if l.Granted {
			held = append(held, s)
		} else {
			queued = append(queued, s)
		}
	}
	switch {
	case len(held) > 0:
		return strings.Join(held, ", ")
	case len(queued) > 0:
		return "(queued ahead for " + strings.Join(queued, ", ") + ")"
	}
	return "-"
}

func (v *locksView) writeWaiting(w io.Writer, waiting []facts.Lock) error {
	fmt.Fprintf(w, "\nwaiting: %d\n", len(waiting))
	if len(waiting) == 0 {
		return nil
	}
	shown := append([]facts.Lock(nil), waiting...)
	sort.SliceStable(shown, func(i, j int) bool {
		a, b := shown[i].WaitS, shown[j].WaitS
		if (a == nil) != (b == nil) {
			return a != nil
		}
		if a != nil && *a != *b {
			return *a > *b
		}
		return *shown[i].PID < *shown[j].PID
	})
	more := 0
	if v.limit > 0 && len(shown) > v.limit {
		more = len(shown) - v.limit
		shown = shown[:v.limit]
	}
	rows := make([][]string, len(shown))
	for i, l := range shown {
		waited := "?" // masked, or untracked: the wait is unseen
		if l.WaitS != nil {
			waited = duration(*l.WaitS)
		}
		var by []string
		for _, b := range l.BlockedBy {
			by = append(by, blockerName(b))
		}
		blockedBy := "-"
		if len(by) > 0 {
			blockedBy = strings.Join(by, ", ")
		}
		rows[i] = []string{fmt.Sprint(*l.PID), fitWidth(escapeControl(objectOf(l).label()), maxName), l.Mode, waited, blockedBy}
	}
	if err := writeTable(w, "  ", []string{"pid", "object", "wants", "waited", "blocked by"}, rows); err != nil {
		return err
	}
	if more > 0 {
		fmt.Fprintf(w, "... %d more rows not shown (use --limit 0 to show all)\n", more)
	}
	return nil
}
