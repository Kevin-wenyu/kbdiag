package rule

import (
	"fmt"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// Waits only reports; it has no threshold in v0.1. Hidden sessions keep the
// verdict from reading OK.
func Waits(w facts.WaitSummary) Result {
	judge, unknown := collected(w.Status)
	if judge && len(w.Redacted()) > 0 {
		unknown = true
	}
	return Result{Verdict: verdictOf(nil, unknown)}
}

// Status only reports. A database size we may not read is redacted but does
// not make the verdict UNKNOWN: nothing is judged on it.
func Status(i facts.InstInfo, d facts.InstDatabases, n facts.InstDownstreams) Result {
	unknown := false
	for _, st := range []facts.Status{i.Status, d.Status, n.Status} {
		_, u := collected(st)
		unknown = unknown || u
	}
	return Result{Verdict: verdictOf(nil, unknown)}
}

// Slots flags inactive replication slots: nothing consumes them, yet they
// keep WAL and, with an xmin, hold back the vacuum horizon.
func Slots(l facts.SlotList) Result {
	judge, unknown := collected(l.Status)
	if !judge {
		return Result{Verdict: verdictOf(nil, unknown)}
	}
	var fs []Finding
	for _, s := range l.Rows {
		if s.Active {
			continue
		}
		symptom := fmt.Sprintf("复制槽 %s 未激活，", s.Name)
		if s.RetainedWALBytes == nil {
			symptom += "未保留 WAL"
		} else {
			symptom += fmt.Sprintf("保留 %.0f MB WAL", float64(*s.RetainedWALBytes)/(1<<20))
		}
		if s.Xmin != nil {
			symptom += fmt.Sprintf("，xmin %d 压着视界", *s.Xmin)
		}
		fs = append(fs, Finding{
			ID:      "slot.inactive",
			Level:   LevelFAIL,
			Symptom: symptom,
			Evidence: []Evidence{{ProbeID: facts.SlotListID, Fields: map[string]any{
				"slot_name": s.Name, "active": s.Active, "xmin": s.Xmin, "retained_wal_bytes": s.RetainedWALBytes,
			}}},
			Next: []Next{{Kind: "verify", Command: "kbdiag status", Note: "在备库上运行，确认它是否在线、是否在接收 WAL"}},
		})
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}
