package rule

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func TestParseSyncNames(t *testing.T) {
	cases := map[string]string{
		"":                      "0",
		"ANY 1( node2)":         "1 [node2]",
		"any 2 (a, \"B c\", d)": "2 [a B c d]",
		"FIRST 1 (s1, s2)":      "1 [s1 s2]",
		"2 (s1, s2, s3)":        "2 [s1 s2 s3]",
		"s1, s2":                "1 [s1 s2]",
		"*":                     "1 [*]",
		"s1":                    "1 [s1]",
		"ANY x (a)":             "?",
		"FIRST 1 (a":            "?",
		`FIRST 1 ("app,1", b)`:  "1 [app,1 b]",
		"anyhost, s2":           "1 [anyhost s2]",
		`"a(b)"`:                "1 [a(b)]",
		"a,,b":                  "?",
		`"unterminated`:         "?",
		"FIRST x":               "?",
	}
	for in, want := range cases {
		n, names, ok := parseSyncNames(in)
		got := "?"
		if ok {
			got = strings.TrimSpace(strings.Join(append([]string{strconv.Itoa(n)}, bracket(names)...), " "))
		}
		if got != want {
			t.Errorf("parseSyncNames(%q) = %q, want %q", in, got, want)
		}
	}
}

func bracket(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return []string{"[" + strings.Join(s, " ") + "]"}
}

func replica(name, state, sync string) facts.Replica {
	return facts.Replica{ApplicationName: &name, State: &state, SyncState: &sync}
}

func TestRepl(t *testing.T) {
	names := func(s, commit string) facts.ReplSync {
		return facts.ReplSync{Status: facts.StatusOK, Rows: []facts.SyncSetting{{StandbyNames: &s, Commit: commit}}}
	}
	down := func(rs ...facts.Replica) facts.ReplDownstreams {
		return facts.ReplDownstreams{Status: facts.StatusOK, Rows: rs}
	}
	na := facts.InstUpstream{Status: facts.StatusNotApplicable}
	naReplay := facts.ReplReplay{Status: facts.StatusNotApplicable}
	cases := []struct {
		name    string
		d       facts.ReplDownstreams
		s       facts.ReplSync
		u       facts.InstUpstream
		r       facts.ReplReplay
		verdict Verdict
		ids     string
	}{
		{"lab primary", down(replica("node2", "streaming", "quorum")), names("ANY 1( node2)", "remote_apply"), na, naReplay, VerdictOK, ""},
		{"standby paused: none streaming (stage 0)", down(), names("ANY 1( node2)", "remote_apply"), na, naReplay, VerdictWARN, "repl.sync_short"},
		{"catching up does not count (the server says potential)", down(replica("node2", "catchup", "potential")), names("ANY 1( node2)", "on"), na, naReplay, VerdictWARN, "repl.sync_short"},
		{"the server's choice is taken as is (stopping counts)", down(replica("NODE2", "stopping", "sync")), names("node2", "on"), na, naReplay, VerdictOK, ""},
		{"a standby not in the list does not count", down(replica("other", "streaming", "async")), names("FIRST 1 (node2)", "on"), na, naReplay, VerdictWARN, "repl.sync_short"},
		{"priority mode caps sync at the number asked for", down(replica("a", "streaming", "sync"), replica("b", "streaming", "sync"), replica("c", "streaming", "potential")), names("FIRST 2 (a, b, c)", "on"), na, naReplay, VerdictOK, ""},
		{"quorum short", down(replica("a", "streaming", "quorum"), replica("b", "catchup", "potential")), names("ANY 2 (a, b)", "remote_apply"), na, naReplay, VerdictWARN, "repl.sync_short"},
		{"no sync standbys asked for", down(), names("", "on"), na, naReplay, VerdictOK, ""},
		{"synchronous_commit local: commits never wait", down(), names("node2", "local"), na, naReplay, VerdictOK, ""},
		{"unparsable names are not judged", down(), names("ANY x (a)", "on"), na, naReplay, VerdictUNKNOWN, ""},
		{"downstreams not collected", facts.ReplDownstreams{Status: facts.StatusError}, names("node2", "on"), na, naReplay, VerdictUNKNOWN, ""},
		{"standby receiving, replay running", facts.ReplDownstreams{Status: facts.StatusOK}, facts.ReplSync{Status: facts.StatusNotApplicable},
			facts.InstUpstream{Status: facts.StatusOK, Rows: []facts.Upstream{{Status: sp("streaming")}}}, facts.ReplReplay{Status: facts.StatusOK, Rows: []facts.Replay{{}}}, VerdictOK, ""},
		{"standby replay paused", facts.ReplDownstreams{Status: facts.StatusOK}, facts.ReplSync{Status: facts.StatusNotApplicable},
			facts.InstUpstream{Status: facts.StatusOK, Rows: []facts.Upstream{{Status: sp("streaming")}}}, facts.ReplReplay{Status: facts.StatusOK, Rows: []facts.Replay{{ReplayPaused: true}}}, VerdictWARN, "repl.replay_paused"},
		{"standby without a receiver", facts.ReplDownstreams{Status: facts.StatusOK}, facts.ReplSync{Status: facts.StatusNotApplicable},
			facts.InstUpstream{Status: facts.StatusOK}, facts.ReplReplay{Status: facts.StatusOK, Rows: []facts.Replay{{}}}, VerdictWARN, "inst.upstream"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Repl(c.d, c.s, c.u, c.r)
			var ids []string
			for _, f := range r.Findings {
				ids = append(ids, f.ID)
				if f.Level != LevelWARN {
					t.Errorf("level %s", f.Level)
				}
			}
			if r.Verdict != c.verdict || strings.Join(ids, ",") != c.ids {
				t.Errorf("verdict=%s ids=%v", r.Verdict, ids)
			}
		})
	}
}
