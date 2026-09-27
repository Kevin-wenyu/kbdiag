package rule

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

// Repl judges three things, all WARN: a standby that is not receiving WAL
// (inst.upstream, the status rule), a standby whose replay is paused, and a
// primary with fewer synchronous standbys streaming than
// synchronous_standby_names asks for. Lag is shown, never judged: there is
// no server-side line for it. A stuck walreceiver (last_msg past
// wal_receiver_timeout) is part of inst.upstream.
func Repl(d facts.ReplDownstreams, s facts.ReplSync, u facts.InstUpstream, r facts.ReplReplay) Result {
	var fs []Finding
	unknown := false
	if f, ok, x := upstream(u); ok {
		fs = append(fs, f)
	} else {
		unknown = unknown || x
	}
	judge, x := collected(r.Status)
	unknown = unknown || x
	if judge {
		for _, p := range r.Rows {
			if p.ReplayPaused {
				fs = append(fs, Finding{
					ID:       "repl.replay_paused",
					Level:    LevelWARN,
					Symptom:  "WAL replay is paused on this standby (sys_wal_replay_pause()): queries still run, but it falls further behind, and a failover would first have to replay everything received",
					Evidence: []Evidence{{ProbeID: facts.ReplReplayID, Fields: map[string]any{"replay_paused": true, "receive_lsn": p.ReceiveLSN, "replay_lsn": p.ReplayLSN, "replay_gap_bytes": p.ReplayGapBytes}}},
					Next:     []Next{{Kind: "fix", SQL: "SELECT sys_wal_replay_resume()", Note: "unless replay was paused on purpose"}},
				})
			}
		}
	}
	f, ok, x := syncShort(d, s)
	unknown = unknown || x
	if ok {
		fs = append(fs, f)
	}
	return Result{Verdict: verdictOf(fs, unknown), Findings: fs}
}

// syncShort compares the number synchronous_standby_names asks for with
// the walsenders the server counts as synchronous. Commits wait for
// standbys only at synchronous_commit on, remote_write or remote_apply;
// that value is this connection's (a role or database may set another).
// OK does not mean commits flow: a candidate that stops confirming (replay
// paused under remote_apply) still counts; its lag is only shown. What the server does when too few are
// connected is not settled for KES: stage 0 saw commits go through with
// none (plan §3), so the symptom does not claim they hang.
func syncShort(d facts.ReplDownstreams, s facts.ReplSync) (f Finding, found, unknown bool) {
	judge, unknown := collected(s.Status)
	if !judge || len(s.Rows) == 0 {
		return Finding{}, false, unknown
	}
	set := s.Rows[0]
	names := ""
	if set.StandbyNames != nil {
		names = *set.StandbyNames
	}
	switch set.Commit {
	case "on", "remote_write", "remote_apply":
	default:
		return Finding{}, false, false
	}
	need, list, ok := parseSyncNames(names)
	if !ok {
		return Finding{}, false, true
	}
	if need == 0 {
		return Finding{}, false, false
	}
	judge, unknown = collected(d.Status)
	if !judge {
		return Finding{}, false, unknown
	}
	_ = list
	streaming := SyncStreaming(d)
	if len(streaming) >= need {
		return Finding{}, false, false
	}
	return Finding{
		ID:    "repl.sync_short",
		Level: LevelWARN,
		Symptom: fmt.Sprintf("synchronous_standby_names '%s' needs %d synchronous %s, but the server counts %d as synchronous candidates: commits either wait or the server no longer waits for a standby, so the synchronous copy is not guaranteed",
			names, need, map[bool]string{true: "standby", false: "standbys"}[need == 1], len(streaming)),
		Evidence: []Evidence{{ProbeID: facts.ReplSyncID, Fields: map[string]any{
			"synchronous_standby_names": names, "synchronous_commit": set.Commit, "required": need, "candidates": streaming,
		}}},
		Next: []Next{
			{Kind: "verify", Command: "kbdiag slots", Note: "whether the missing standby's slot is inactive"},
			{Kind: "verify", Command: "kbdiag status", Note: "run on the missing standby: is it receiving WAL"},
		},
	}, true, false
}

// SyncStreaming lists the walsenders the server itself counts as
// synchronous candidates: sync_state sync or quorum. The server picks them
// (streaming or stopping, a valid flush position, a name in the list,
// compared case-insensitively), so kbdiag does not redo that choice; in
// priority mode sync is capped at the required number, which is still
// enough to compare with it.
func SyncStreaming(d facts.ReplDownstreams) []string {
	out := []string{}
	for _, r := range d.Rows {
		if r.SyncState == nil || (*r.SyncState != "sync" && *r.SyncState != "quorum") {
			continue
		}
		name := "-"
		if r.ApplicationName != nil {
			name = *r.ApplicationName
		}
		out = append(out, name)
	}
	return out
}

// parseSyncNames reads synchronous_standby_names: "", "a, b" (one of them,
// first by priority), "N (a, b)", "FIRST N (...)", "ANY N (...)". Names may
// be double-quoted, with commas or parentheses inside. ok is false for
// anything else, which is then not judged.
func parseSyncNames(s string) (need int, names []string, ok bool) {
	toks, ok := syncTokens(s)
	if !ok {
		return 0, nil, false
	}
	if len(toks) == 0 {
		return 0, nil, true
	}
	need, i := 1, 0
	if len(toks) > 1 && !toks[0].quoted && (strings.EqualFold(toks[0].s, "first") || strings.EqualFold(toks[0].s, "any")) {
		i = 1
	}
	if n, err := strconv.Atoi(toks[i].s); err == nil && !toks[i].quoted && i+1 < len(toks) && toks[i+1].s == "(" && !toks[i+1].quoted {
		if n < 0 || toks[len(toks)-1].s != ")" || toks[len(toks)-1].quoted {
			return 0, nil, false
		}
		need, toks = n, toks[i+2:len(toks)-1]
	} else if i == 1 {
		return 0, nil, false // FIRST/ANY without "N (" is not the grammar
	}
	// the rest is name {, name}
	for j, t := range toks {
		switch {
		case j%2 == 0 && (t.quoted || t.s != "," && t.s != "(" && t.s != ")"):
			names = append(names, t.s)
		case j%2 == 1 && t.s == "," && !t.quoted:
		default:
			return 0, nil, false
		}
	}
	if len(names) == 0 || len(toks)%2 == 0 {
		return 0, nil, false
	}
	return need, names, true
}

type syncToken struct {
	s      string
	quoted bool
}

// syncTokens splits the setting into words, quoted names, commas and
// parentheses.
func syncTokens(s string) ([]syncToken, bool) {
	var out []syncToken
	rs := []rune(s)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case r == ' ' || r == '\t' || r == '\n':
			i++
		case r == ',' || r == '(' || r == ')':
			out = append(out, syncToken{s: string(r)})
			i++
		case r == '"':
			var b strings.Builder
			i++
			for {
				if i >= len(rs) {
					return nil, false
				}
				if rs[i] == '"' {
					if i+1 < len(rs) && rs[i+1] == '"' {
						b.WriteRune('"')
						i += 2
						continue
					}
					i++
					break
				}
				b.WriteRune(rs[i])
				i++
			}
			out = append(out, syncToken{s: b.String(), quoted: true})
		default:
			j := i
			for j < len(rs) && !strings.ContainsRune(" \t\n,()\"", rs[j]) {
				j++
			}
			out = append(out, syncToken{s: string(rs[i:j])})
			i = j
		}
	}
	return out, true
}

// SyncNames describes synchronous_standby_names for a reader:
// "any 1 of node2", "the first 1 of s1, s2 by priority"; ok false when it
// does not parse, need 0 when nothing is asked for.
func SyncNames(s string) (need int, names []string, desc string, ok bool) {
	need, names, ok = parseSyncNames(s)
	if !ok || need == 0 {
		return need, names, "", ok
	}
	list := strings.Join(names, ", ")
	if toks, _ := syncTokens(s); len(toks) > 2 && !toks[0].quoted && strings.EqualFold(toks[0].s, "any") {
		return need, names, fmt.Sprintf("any %d of %s must confirm", need, list), true
	}
	return need, names, fmt.Sprintf("the first %d of %s by priority must confirm", need, list), true
}
