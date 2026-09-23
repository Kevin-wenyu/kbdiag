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

// Status flags connections nearing the limit ordinary users may open:
// max_connections less the slots reserved for superusers. A database size we
// may not read is redacted but does not make the verdict UNKNOWN: nothing is
// judged on it.
func Status(i facts.InstInfo, d facts.InstDatabases, n facts.InstDownstreams, th Thresholds) Result {
	judge, unknown := collected(i.Status)
	for _, st := range []facts.Status{d.Status, n.Status} {
		_, u := collected(st)
		unknown = unknown || u
	}
	var fs []Finding
	if judge {
		for _, x := range i.Rows {
			if f, ok := connections(x, th); ok {
				fs = append(fs, f)
			}
		}
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}

func connections(x facts.Info, th Thresholds) (Finding, bool) {
	usable := x.MaxConnections - x.SuperuserReserved
	if usable <= 0 {
		return Finding{}, false
	}
	pct := float64(x.Connections) * 100 / float64(usable)
	if pct < th.ConnWarnPct {
		return Finding{}, false
	}
	level := LevelWARN
	if pct >= th.ConnFailPct {
		level = LevelFAIL
	}
	return Finding{
		ID:    "inst.connections",
		Level: level,
		Symptom: fmt.Sprintf("连接已用 %d 个，普通用户可用 %d 个（max_connections %d 减去超级用户保留 %d），占 %.0f%%",
			x.Connections, usable, x.MaxConnections, x.SuperuserReserved, pct),
		Evidence: []Evidence{{ProbeID: facts.InstInfoID, Fields: map[string]any{
			"connections": x.Connections, "max_connections": x.MaxConnections, "superuser_reserved_connections": x.SuperuserReserved,
		}}},
		Next: []Next{{Kind: "verify", Command: "kbdiag sessions --limit 0", Note: "看连接是谁占的：按 usename、application_name、client_addr 看有没有扎堆"}},
	}, true
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
			Next: []Next{{Kind: "verify", Command: "kbdiag sessions", Note: "在备库上运行：连不上说明备库实例挂了；列表里没有 walreceiver 进程说明它没在接收 WAL"}},
		})
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}
