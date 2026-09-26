package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// waitsView is the waits text layout (plan 2026-09-26-polish-remaining,
// stage 5): what the sessions that are doing something wait on, biggest
// pile first; idle sessions and background processes only counted.
type waitsView struct{ summary facts.WaitSummary }

func (r *Report) SetWaits(w facts.WaitSummary) { r.waits = &waitsView{summary: w} }

// maxPIDs caps the pid list of one group; --json has them all.
const maxPIDs = 10

func (v *waitsView) write(w io.Writer) error {
	if v.summary.Status != facts.StatusOK {
		writeNotOKAs(w, facts.WaitSummaryID, v.summary.Status, v.summary.Reason)
		return nil
	}
	var shown []facts.Wait
	busy, idle, background, hidden, untracked := 0, 0, 0, 0, 0
	for _, g := range v.summary.Rows {
		hidden += g.Masked
		rest := g.Sessions - g.Masked
		switch {
		case rest == 0:
		// Activity: a process idling in its main loop (walsender with
		// nothing to send, checkpointer between checkpoints, KSH workers),
		// whatever its state says
		case g.WaitEventType != nil && *g.WaitEventType == "Activity":
			background += rest
		case g.State != nil && *g.State == "idle":
			idle += rest
		// a background process (no state) not waiting on anything
		case g.State == nil && g.WaitEventType == nil:
			background += rest
		case g.State != nil && *g.State == "disabled":
			untracked += rest // what it does is not reported
		default:
			// includes background processes stuck on a real wait (a
			// checkpointer on IO, the startup process on a buffer pin)
			busy += rest
			shown = append(shown, g)
		}
	}
	sort.SliceStable(shown, func(i, j int) bool {
		a, b := shown[i], shown[j]
		if a.Sessions != b.Sessions {
			return a.Sessions > b.Sessions
		}
		if aa, ba := isActive(a), isActive(b); aa != ba {
			return aa
		}
		if waitLabel(a) != waitLabel(b) {
			return waitLabel(a) < waitLabel(b)
		}
		return cell(a.State) < cell(b.State)
	})
	fmt.Fprintf(w, "\nnot idle: %d\n", busy)
	if len(shown) > 0 {
		rows := make([][]string, len(shown))
		for i, g := range shown {
			state := cell(g.State)
			if g.State == nil {
				state = "(background)"
			}
			rows[i] = []string{waitLabel(g), state, fmt.Sprint(g.Sessions - g.Masked), pidList(g.PIDs)}
		}
		if err := writeTable(w, "  ", []string{"wait", "state", "sessions", "pids"}, rows); err != nil {
			return err
		}
	}
	var parts []string
	if idle > 0 {
		parts = append(parts, fmt.Sprintf("%d idle", idle))
	}
	if background > 0 {
		parts = append(parts, fmt.Sprintf("%d background", background))
	}
	// we cannot tell whether these are idle: not "not idle" either
	if hidden > 0 {
		parts = append(parts, fmt.Sprintf("%d hidden (state unknown)", hidden))
	}
	if untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked (state unknown)", untracked))
	}
	if len(parts) > 0 {
		fmt.Fprintf(w, "\nnot shown: %s\n", strings.Join(parts, ", "))
	}
	return nil
}

func isActive(g facts.Wait) bool { return g.State != nil && *g.State == "active" }

// waitLabel is "type:event"; a session executing (active, or in a
// fastpath function call) without a wait event is running.
func waitLabel(g facts.Wait) string {
	switch {
	case g.WaitEventType != nil:
		return cell(g.WaitEventType) + ":" + cell(g.WaitEvent)
	case isActive(g) || (g.State != nil && *g.State == "fastpath function call"):
		return "(running)"
	}
	return "-"
}

func pidList(pids []int32) string {
	var s []string
	for i, p := range pids {
		if i == maxPIDs {
			s = append(s, fmt.Sprintf("... (+%d)", len(pids)-maxPIDs))
			break
		}
		s = append(s, fmt.Sprint(p))
	}
	return strings.Join(s, " ")
}
