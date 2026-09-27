package scenario

import (
	"testing"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// replFacts rebuilds repl from the stage 0 captures of one node, clean or
// with the standby's walreceiver paused. The LSN differences are what
// sys_wal_lsn_diff gives for the captured LSNs (all equal on the lab), the
// ages come from each capture's now().
func replFacts(t *testing.T, node, scene string) (facts.Context, facts.ReplDownstreams, facts.ReplSync, facts.InstUpstream, facts.ReplReplay) {
	t.Helper()
	lsn := loadKsql(t, "repl_"+node+"_sys_lsn"+scene).rows[0]
	now := *kTime(t, lsn["now"])
	c := v02Context("primary", "system", "local")
	if kBool(lsn["in_recovery"]) {
		c.Role = "standby"
	}
	d := facts.ReplDownstreams{Status: facts.StatusOK}
	stat := loadKsql(t, "repl_"+node+"_sys_stat"+scene)
	for _, m := range stat.rows {
		cur := lsn["current_lsn"]
		if cur == nil {
			cur = lsn["replay_lsn"]
		}
		zero := func(l *string) *int64 { // the lab's LSNs are equal; anything else would need sys_wal_lsn_diff
			if *l != *cur {
				t.Fatalf("capture LSN %s differs from %s", *l, *cur)
			}
			return i64(0)
		}
		reply := kTime(t, m["reply_time"])
		sNow := kTime(t, m["now"])
		age := sNow.Sub(*reply).Seconds()
		prio := int32(*kI64(t, m["sync_priority"]))
		d.Rows = append(d.Rows, facts.Replica{PID: int32(*kI64(t, m["pid"])), ApplicationName: m["application_name"], ClientAddr: m["client_addr"], State: m["state"],
			SyncState: m["sync_state"], SyncPriority: &prio, SentLSN: m["sent_lsn"], WriteLSN: m["write_lsn"], FlushLSN: m["flush_lsn"], ReplayLSN: m["replay_lsn"],
			SentLagBytes: zero(m["sent_lsn"]), FlushLagBytes: zero(m["flush_lsn"]), ReplayLagBytes: zero(m["replay_lsn"]), ReplyAgeS: &age})
		c.CollectedAt = sNow.Truncate(time.Second)
	}
	if len(stat.rows) == 0 {
		c.CollectedAt = now.Truncate(time.Second)
	}
	set := map[string]*string{}
	for _, m := range loadKsql(t, "repl_"+node+"_sys_settings").rows {
		set[*m["name"]] = m["setting"]
	}
	s := facts.ReplSync{Status: facts.StatusNotApplicable, Reason: "standby"}
	u := facts.InstUpstream{Status: facts.StatusNotApplicable, Reason: "primary"}
	r := facts.ReplReplay{Status: facts.StatusNotApplicable, Reason: "primary"}
	if c.Role == "primary" {
		s = facts.ReplSync{Status: facts.StatusOK, Rows: []facts.SyncSetting{{StandbyNames: set["synchronous_standby_names"], Commit: *set["synchronous_commit"]}}}
	} else {
		u = facts.InstUpstream{Status: facts.StatusOK}
		for _, m := range loadKsql(t, "repl_"+node+"_sys_receiver"+scene).rows {
			port := int32(*kI64(t, m["sender_port"]))
			age := kTime(t, m["now"]).Sub(*kTime(t, m["last_msg_receipt_time"])).Seconds()
			u.Rows = append(u.Rows, facts.Upstream{Status: m["status"], SenderHost: m["sender_host"], SenderPort: &port, SlotName: m["slot_name"], LastMsgAgeS: &age})
		}
		gap := int64(0)
		if *lsn["receive_lsn"] != *lsn["replay_lsn"] {
			t.Fatal("capture LSNs differ")
		}
		replayAge := now.Sub(*kTime(t, lsn["replay_ts"])).Seconds()
		r = facts.ReplReplay{Status: facts.StatusOK, Rows: []facts.Replay{{ReceiveLSN: lsn["receive_lsn"], ReplayLSN: lsn["replay_lsn"], ReplayGapBytes: &gap,
			LastReplayAgeS: &replayAge, ReplayPaused: kBool(lsn["paused"])}}}
		c.CollectedAt = now.Truncate(time.Second)
	}
	return c, d, s, u, r
}

func TestReplText(t *testing.T) {
	for _, x := range []struct {
		golden, node, scene string
		verdict             rule.Verdict
	}{
		{"repl_primary", "node1", "", rule.VerdictOK},
		{"repl_primary_paused", "node1", "_paused", rule.VerdictWARN},
		{"repl_standby", "node2", "", rule.VerdictOK},
	} {
		t.Run(x.golden, func(t *testing.T) {
			rep := Repl(replFacts(t, x.node, x.scene))
			assertGolden(t, x.golden, rep)
			if rep.Verdict != x.verdict {
				t.Errorf("verdict = %s", rep.Verdict)
			}
		})
	}
}

// A primary with two standbys behind by MB and GB (one catching up, one
// outside the list), FIRST 2 of three names; unparsable names; a standby
// with replay paused, 3 GB received but not replayed, a cascade downstream.
func TestReplTextEdges(t *testing.T) {
	c := v02Context("primary", "system", "local")
	names := `FIRST 2 (node2, "Node 3", node4)`
	d := facts.ReplDownstreams{Status: facts.StatusOK, Rows: []facts.Replica{
		{PID: 1, ApplicationName: str("node2"), ClientAddr: str("10.0.0.2"), State: str("streaming"), SyncState: str("sync"),
			SentLagBytes: i64(0), FlushLagBytes: i64(5 << 20), ReplayLagBytes: i64(40 << 20), ReplayLagS: f64(12.4), ReplyAgeS: f64(0.4)},
		{PID: 2, ApplicationName: str("Node 3"), State: str("catchup"), SyncState: str("potential"),
			SentLagBytes: i64(3 << 30), FlushLagBytes: i64(3 << 30), ReplayLagBytes: i64(3 << 30), ReplyAgeS: f64(3700)},
		{PID: 3, ApplicationName: str("backup\x1b[2J"), State: str("streaming"), SyncState: str("async"), SentLagBytes: i64(-5)},
	}}
	s := facts.ReplSync{Status: facts.StatusOK, Rows: []facts.SyncSetting{{StandbyNames: &names, Commit: "on"}}}
	na := facts.InstUpstream{Status: facts.StatusNotApplicable, Reason: "primary"}
	nr := facts.ReplReplay{Status: facts.StatusNotApplicable, Reason: "primary"}
	rep := Repl(c, d, s, na, nr)
	assertGolden(t, "repl_edges_primary", rep)
	if rep.Verdict != rule.VerdictWARN || len(rep.Findings) != 1 {
		t.Errorf("verdict=%s findings=%+v", rep.Verdict, rep.Findings)
	}
	bad := "ANY two (a)"
	s.Rows[0].StandbyNames = &bad
	if rep := Repl(c, d, s, na, nr); rep.Verdict != rule.VerdictUNKNOWN {
		t.Errorf("unparsable names: %s", rep.Verdict)
	}

	sb := v02Context("standby", "system", "local")
	port := int32(54321)
	u := facts.InstUpstream{Status: facts.StatusOK, Rows: []facts.Upstream{{Status: str("streaming"), SenderHost: str("10.0.0.1"), SenderPort: &port, SlotName: str("repmgr_slot_2"), LastMsgAgeS: f64(95), WALReceiverTimeoutS: f64(30)}}}
	r := facts.ReplReplay{Status: facts.StatusOK, Rows: []facts.Replay{{ReceiveLSN: str("0/C0000000"), ReplayLSN: str("0/10000000"), ReplayGapBytes: i64(3 << 30), LastReplayAgeS: f64(600), ReplayPaused: true}}}
	cascade := facts.ReplDownstreams{Status: facts.StatusOK, Rows: []facts.Replica{{PID: 9, ApplicationName: str("node3"), State: str("streaming"), SyncState: str("async"), SentLagBytes: i64(0), FlushLagBytes: i64(0), ReplayLagBytes: i64(0)}}}
	rep = Repl(sb, cascade, facts.ReplSync{Status: facts.StatusNotApplicable, Reason: "standby: synchronous replication is decided on the primary"}, u, r)
	assertGolden(t, "repl_edges_standby", rep)
	// the receiver has heard nothing for 95s, past wal_receiver_timeout: stuck
	if rep.Verdict != rule.VerdictWARN || len(rep.Findings) != 2 || rep.Findings[0].ID != "inst.upstream" || rep.Findings[1].ID != "repl.replay_paused" {
		t.Errorf("verdict=%s findings=%+v", rep.Verdict, rep.Findings)
	}
}
