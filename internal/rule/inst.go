package rule

import (
	"fmt"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/units"
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

// Status judges two things: ordinary users can no longer connect (FAIL), and
// a standby is not receiving WAL (WARN). Everything else it shows is not
// judged: a database size or downstream state we may not read is redacted
// but does not make the verdict UNKNOWN, and inst.disk is not an input.
func Status(i facts.InstInfo, d facts.InstDatabases, n facts.InstDownstreams, u facts.InstUpstream) Result {
	judge, unknown := collected(i.Status)
	for _, st := range []facts.Status{d.Status, n.Status} {
		_, x := collected(st)
		unknown = unknown || x
	}
	var fs []Finding
	if judge {
		for _, x := range i.Rows {
			if f, ok := connections(x); ok {
				fs = append(fs, f)
			}
		}
	}
	f, ok, x := upstream(u)
	if ok {
		fs = append(fs, f)
	}
	return Result{Verdict: verdictOf(fs, unknown || x), Findings: fs}
}

// connections fails once every slot ordinary users may open is taken. Short
// of that there is no objective line: how full is too full depends on the
// application, so nothing is reported.
func connections(x facts.Info) (Finding, bool) {
	usable := x.Usable()
	if usable <= 0 || x.Connections < usable {
		return Finding{}, false
	}
	return Finding{
		ID:    "inst.connections",
		Level: LevelFAIL,
		Symptom: fmt.Sprintf("%d connections in use, reaching the %d ordinary users may open (max_connections %d minus %d reserved for superusers): ordinary users can no longer connect",
			x.Connections, usable, x.MaxConnections, x.SuperuserReserved),
		Evidence: []Evidence{{ProbeID: facts.InstInfoID, Fields: map[string]any{
			"connections": x.Connections, "max_connections": x.MaxConnections, "superuser_reserved_connections": x.SuperuserReserved,
		}}},
		Next: []Next{{Kind: "verify", Command: "kbdiag sessions", Note: "who holds the connections: the summary at the top counts them by user, database, application and client"}},
	}, true
}

// upstream warns when a standby receives no WAL: today's queries still run,
// but if the primary fails now there is no up-to-date standby to take over.
// last_msg is not judged: a quiet primary sends one every
// wal_receiver_status_interval (8s ago was normal in the lab).
func upstream(u facts.InstUpstream) (f Finding, found, unknown bool) {
	judge, unknown := collected(u.Status)
	if !judge {
		return Finding{}, false, unknown
	}
	next := []Next{{Kind: "verify", Command: "kbdiag slots", Note: "run on the primary: is this standby's slot inactive?"}}
	if len(u.Rows) == 0 {
		return Finding{
			ID:       "inst.upstream",
			Level:    LevelWARN,
			Symptom:  "the standby has no WAL receiver and is not receiving WAL from the primary: if the primary fails now, no standby can take over",
			Evidence: []Evidence{{ProbeID: facts.InstUpstreamID, Fields: map[string]any{"status": nil}}},
			Next:     next,
		}, true, false
	}
	for _, x := range u.Rows {
		if x.Status == nil {
			return Finding{}, false, true
		}
		if *x.Status == "streaming" {
			continue
		}
		return Finding{
			ID:      "inst.upstream",
			Level:   LevelWARN,
			Symptom: fmt.Sprintf("the standby's WAL receiver is %s, not streaming, so it is not receiving WAL from the primary: if the primary fails now, no standby can take over", *x.Status),
			Evidence: []Evidence{{ProbeID: facts.InstUpstreamID, Fields: map[string]any{
				"status": *x.Status, "sender_host": x.SenderHost, "sender_port": x.SenderPort, "slot_name": x.SlotName,
			}}},
			Next: next,
		}, true, false
	}
	return Finding{}, false, false
}

// Slots flags inactive replication slots: nothing consumes them, yet they
// keep WAL and, with an xmin, hold back the vacuum horizon. WARN: the harm
// (a full disk, bloat, no up-to-date standby) comes later, not now.
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
		symptom := fmt.Sprintf("replication slot %s is inactive, ", s.Name)
		if s.RetainedWALBytes == nil {
			symptom += "retaining no WAL"
		} else {
			// a standby's replay can pass restart_lsn for a moment
			symptom += "retaining " + units.Bytes(max(0, float64(*s.RetainedWALBytes))) + " WAL"
		}
		if s.Xmin != nil {
			symptom += fmt.Sprintf(", xmin %d holding back the vacuum horizon", *s.Xmin)
		}
		if s.CatalogXmin != nil { // logical slots: holds back vacuum of system catalogs
			symptom += fmt.Sprintf(", catalog_xmin %d holding back the system catalogs' horizon", *s.CatalogXmin)
		}
		fs = append(fs, Finding{
			ID:      "slot.inactive",
			Level:   LevelWARN,
			Symptom: symptom,
			Evidence: []Evidence{{ProbeID: facts.SlotListID, Fields: map[string]any{
				"slot_name": s.Name, "active": s.Active, "xmin": s.Xmin, "catalog_xmin": s.CatalogXmin, "retained_wal_bytes": s.RetainedWALBytes,
			}}},
			Next: []Next{{Kind: "verify", Command: "kbdiag status", Note: "run on this slot's downstream node (usually a standby): if it cannot connect, the node is down; inst.upstream with no receiver or not streaming means it is not receiving WAL; if it shows streaming, run it again after 10-20s: a last_msg that keeps growing means the receiver is stuck"}},
		})
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}
