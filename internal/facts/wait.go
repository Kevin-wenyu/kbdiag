package facts

// WaitSummaryID is the probe_id of the wait-event summary.
const WaitSummaryID = "wait.summary"

// WaitColumns is the column contract of wait.summary (PRD §5.1).
var WaitColumns = []string{"wait_event_type", "wait_event", "state", "sessions", "pids"}

// Wait is one group of sessions sharing wait event and state.
type Wait struct {
	WaitEventType *string
	WaitEvent     *string
	State         *string
	Sessions      int
	PIDs          []int32
	Masked        int // sessions in this group KES hid from us
	// Background lists the group's background processes (not client
	// backends or parallel workers). Not in the JSON: it only tells the
	// text a background process running with no wait event from a busy
	// client (stage 9, the KES ksh writer).
	Background []int32
}

func (w Wait) Row() []any {
	return []any{w.WaitEventType, w.WaitEvent, w.State, w.Sessions, w.PIDs}
}

type WaitSummary struct {
	Status Status
	Reason string
	Rows   []Wait
}

var redactedWaitColumns = []string{"wait_event_type", "wait_event", "state"}

// Redacted counts sessions, not groups (PRD §5): masked ones for lack of
// privilege, and untracked ones (state 'disabled', see Session.Untracked).
func (s WaitSummary) Redacted() []Redaction {
	masked, untracked := 0, 0
	for _, w := range s.Rows {
		masked += w.Masked
		if w.State != nil && *w.State == "disabled" {
			untracked += w.Sessions - w.Masked
		}
	}
	var out []Redaction
	add := func(reason string, n int) {
		if n == 0 {
			return
		}
		for _, f := range redactedWaitColumns {
			out = append(out, Redaction{ProbeID: WaitSummaryID, Field: f, Reason: reason, RowsAffected: n})
		}
	}
	add(ReasonInsufficientPrivilege, masked)
	add(ReasonTrackActivitiesOff, untracked)
	return out
}
