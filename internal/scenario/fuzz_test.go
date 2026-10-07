package scenario

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
)

// v02Reports builds every v0.2 command's report from one string, one
// integer and one float, in every field that takes them, NULLs where the
// flag says so.
func v02Reports(s string, n int64, f float64, null bool) []*report.Report {
	c := facts.Context{Version: s, Role: "primary", Location: "local", User: s, Database: s, CollectedAt: time.Unix(0, 0)}
	p := func() *string {
		if null {
			return nil
		}
		return &s
	}
	pi := func() *int64 {
		if null {
			return nil
		}
		return &n
	}
	pf := func() *float64 {
		if null {
			return nil
		}
		return &f
	}
	i32 := int32(n)
	pi32 := func() *int32 {
		if null {
			return nil
		}
		return &i32
	}
	u := uint64(n)
	now := time.Unix(n%1e10, 0)
	pt := func() *time.Time {
		if null {
			return nil
		}
		return &now
	}
	ok := facts.StatusOK
	var out []*report.Report
	disk := facts.Disk{TotalBytes: u, UsedBytes: u / 2, AvailBytes: u / 3, FSID: s}
	out = append(out, Space(c, facts.InstDatabases{Status: ok, Rows: []facts.Database{{Datname: s, SizeBytes: pi()}}},
		facts.SpaceTablespaces{Status: ok, Rows: []facts.Tablespace{{Name: s, Location: p(), SizeBytes: pi()}}},
		facts.SpaceWAL{Status: ok, Rows: []facts.WAL{{Files: n, Bytes: n, MaxWALSizeBytes: pi(), WALKeepBytes: pi(), WALSegmentBytes: pi()}}},
		facts.SpaceDisk{Status: ok, Rows: []facts.Mount{{Kind: "tablespace", Path: s, Disk: disk}}}))
	out = append(out, Freeze(c, facts.FreezeDatabases{Status: ok, Rows: []facts.FrozenDatabase{{Datname: s, XIDAge: i32, MXIDAge: i32, AllowConn: !null}}},
		facts.FreezeTables{Status: ok, Rows: []facts.FrozenTable{{Relation: s, Relkind: s, XIDAge: i32, MXIDAge: i32, HeapBytesEst: n}}},
		facts.FreezeLimits{Status: ok, Rows: []facts.FreezeLimit{{FreezeMaxAge: n, MultiFreezeMaxAge: n}}}, FreezeOptions{Limit: 1}))
	out = append(out, Vacuum(c, facts.VacuumTables{Status: ok, Rows: []facts.VacuumTable{{Schemaname: s, Relname: s, NDeadTup: n, Reltuples: float32(f), Reloptions: []string{s, "autovacuum_enabled=" + s}, LastVacuumAgeS: pf()}}},
		facts.VacuumProgress{Status: ok, Rows: []facts.VacuumRun{{PID: i32, Datname: p(), Relation: p(), Phase: p(), HeapBlksTotal: pi(), HeapBlksScanned: pi(), XactAgeS: pf()}}},
		facts.VacuumSettings{Status: ok, Rows: []facts.VacuumSetting{{Autovacuum: s, TrackCounts: s, Threshold: n, ScaleFactor: f, NaptimeS: n}}}, VacuumOptions{}))
	out = append(out, Archive(c, facts.ArchiveStatus{Status: ok, Rows: []facts.Archiver{{Mode: s, Command: p(), TimeoutS: n, ArchivedCount: n, LastArchivedWAL: p(), LastArchivedTime: pt(),
		LastArchivedAgeS: pf(), FailedCount: n, LastFailedWAL: p(), LastFailedTime: pt(), LastFailedAgeS: pf(), StatsReset: pt()}}},
		facts.ArchiveReady{Status: ok, Rows: []facts.ArchiveQueue{{Ready: n, Done: n, OldestReadyAgeS: pf()}}}))
	out = append(out, Params(c, facts.ParamsChanged{Status: ok, Rows: []facts.Param{{Name: s, Setting: p(), Unit: p(), Source: s, Sourcefile: p(), Sourceline: pi32(), Context: s, PendingRestart: !null}}}))
	out = append(out, Repl(c, facts.ReplDownstreams{Status: ok, Rows: []facts.Replica{{PID: i32, ApplicationName: p(), ClientAddr: p(), State: p(), SyncState: p(), SentLagBytes: pi(), ReplayLagS: pf(), ReplyAgeS: pf()}}},
		facts.ReplSync{Status: ok, Rows: []facts.SyncSetting{{StandbyNames: p(), Commit: s}}}, facts.InstUpstream{Status: facts.StatusNotApplicable}, facts.ReplReplay{Status: facts.StatusNotApplicable}))
	sb := c
	sb.Role = "standby"
	out = append(out, Repl(sb, facts.ReplDownstreams{Status: ok}, facts.ReplSync{Status: facts.StatusNotApplicable},
		facts.InstUpstream{Status: ok, Rows: []facts.Upstream{{Status: p(), SenderHost: p(), SenderPort: pi32(), SlotName: p(), LastMsgAgeS: pf()}}},
		facts.ReplReplay{Status: ok, Rows: []facts.Replay{{ReceiveLSN: p(), ReplayLSN: p(), ReplayGapBytes: pi(), LastReplayAgeS: pf(), ReplayPaused: !null}}}))
	out = append(out, Cluster(c, facts.ClusterNodes{Status: ok, Rows: []facts.ClusterNode{{NodeID: i32, NodeName: s, Type: s, UpstreamNodeID: pi32(), Active: null, Priority: pi32(), Location: p(), SlotName: p(), IsLocal: !null}}},
		facts.ClusterEvents{Status: ok, Rows: []facts.ClusterEvent{{NodeID: i32, Event: s, Time: now, AgeS: f, Details: p()}}}, facts.InstDownstreams{Status: ok},
		facts.ClusterSyncs{Status: ok, Rows: []facts.ClusterSync{{ConfPath: s, Synchronous: p(), StandbyNames: p()}}}))
	// a configured synchronous mode with the list empty: the finding quotes the server-side path
	out = append(out, Cluster(c, facts.ClusterNodes{Status: ok, Rows: []facts.ClusterNode{{NodeID: 1, NodeName: s, Type: "primary", Active: true, IsLocal: true}}},
		facts.ClusterEvents{Status: ok}, facts.InstDownstreams{Status: ok},
		facts.ClusterSyncs{Status: ok, Rows: []facts.ClusterSync{{ConfPath: s, Synchronous: str("quorum")}}}))
	out = append(out, TopObjects(c, facts.ObjectTables{Status: ok, Rows: []facts.ObjectTable{{Schemaname: s, Relname: s, Relkind: s, TotalBytes: n, ToastBytes: pi()}}},
		facts.ObjectIndexes{Status: ok, Rows: []facts.ObjectIndex{{Schemaname: s, Relname: s, TableName: s, Bytes: n}}}, TopObjectsOptions{}))
	tbl, _ := Table(c, facts.TableInfos{Status: ok, Rows: []facts.TableInfo{{Schemaname: s, Relname: s, Relkind: "r", Relpersistence: s, Reltuples: float32(f), Reloptions: []string{s}, XIDAge: pi32(), MXIDAge: pi32()}}},
		facts.TableSizes{Status: ok, Rows: []facts.TableSize{{TotalBytes: n, ToastBytes: pi()}}},
		facts.TableStats{Status: ok, Rows: []facts.TableStat{{NDeadTup: n, LastVacuumAgeS: pf(), IdxScan: pi(), HeapBlksRead: pi(), HeapBlksHit: pi()}}},
		facts.TableIndexes{Status: ok, Rows: []facts.TableIndex{{Name: s, Definition: s, Bytes: pi(), IdxScan: pi()}}},
		facts.FreezeLimits{Status: ok, Rows: []facts.FreezeLimit{{FreezeMaxAge: n}}}, facts.VacuumSettings{Status: ok, Rows: []facts.VacuumSetting{{Autovacuum: s, TrackCounts: s, Threshold: n, ScaleFactor: f}}})
	out = append(out, tbl)
	out = append(out, Top(c, facts.SQLTop{Status: ok, Track: s, Rows: []facts.Statement{{QueryID: pi(), Username: p(), Datname: p(), Calls: n, TotalExecS: f, MeanExecS: f, Query: p()}}}, TopOptions{By: "time"}))
	out = append(out, Progress(c, facts.ProgressList{Status: ok, Rows: []facts.Operation{{PID: i32, Command: s, Datname: p(), Relation: p(), Phase: p(), Done: pi(), Total: pi(), Unit: s, RunningS: pf(), WaitingLockers: pi()}}},
		facts.ProgressList{Status: facts.StatusError, Reason: s}))
	out = append(out, Checkpoint(c, facts.CheckpointStats{Status: ok, Rows: []facts.BGWriter{{CheckpointsTimed: n, CheckpointsReq: n, CheckpointWriteS: f, BuffersBackend: n, StatsReset: pt(), StatsResetAgeS: pf()}}},
		facts.CheckpointLast{Status: ok, Rows: []facts.LastCheckpoint{{Time: pt(), AgeS: pf(), LSN: p(), RedoLSN: p(), RedoWALFile: p()}}},
		facts.CheckpointSettings{Status: ok, Rows: []facts.CheckpointSetting{{TimeoutS: n, MaxWALSizeBytes: n, CompletionTarget: f, WarningS: n, LogCheckpoints: s}}}))
	out = append(out, WAL(c, facts.WALPosition{Status: ok, Rows: []facts.Position{{InRecovery: null, LSN: p(), WALFile: p()}}},
		facts.SpaceWAL{Status: ok, Rows: []facts.WAL{{Files: n, Bytes: n, WALKeepBytes: pi(), WALSegmentBytes: pi()}}},
		facts.SlotList{Status: ok, Rows: []facts.Slot{{Name: s, Type: s, RetainedWALBytes: pi()}}}, facts.ArchiveReady{Status: facts.StatusSkipped, Reason: s}))
	out = append(out, Seq(c, facts.SeqList{Status: ok, Rows: []facts.Sequence{{Schemaname: s, Sequencename: s, DataType: s, StartValue: n, MinValue: -n, MaxValue: n, IncrementBy: n % 7, LastValue: pi(), Readable: !null}}}, SeqOptions{}))
	return out
}

// No v0.2 report panics, and no control, format or separator character a
// server string carries reaches the terminal unescaped (plan stage 18).
func FuzzV02Reports(f *testing.F) {
	f.Add("public.orders", int64(1311000), 0.5, false)
	f.Add("\x1b[2J‮ \t\r", int64(-1), -1.5, true)
	f.Add(strings.Repeat("表", 200), int64(1<<62), 1e15, false)
	f.Add("\x1b[2J\u202e\u200b\x9b\xc2\x9b", int64(42), 2.5, false)
	f.Add("", int64(-1<<63), 0.0, false)
	f.Add("\xff\xfe", int64(1<<63-1), -1e-300, true)
	f.Fuzz(func(t *testing.T, s string, n int64, x float64, null bool) {
		// Every float kbdiag reads is a real/double column or a seconds count
		// the server computed: finite and far inside float32.
		if math.IsNaN(x) || math.Abs(x) > 1e15 {
			t.Skip()
		}
		for _, rep := range v02Reports(s, n, x, null) {
			var text, js bytes.Buffer
			if err := rep.WriteText(&text); err != nil {
				t.Fatal(err)
			}
			if err := rep.WriteJSON(&js); err != nil {
				t.Fatalf("%s: JSON: %v", rep.Command, err)
			}
			checkContract(t, rep)
			if !utf8.Valid(text.Bytes()) {
				t.Fatalf("%s: bytes that are not UTF-8 in the text:\n%q", rep.Command, text.String())
			}
			for _, r := range text.String() {
				if r != '\n' && (unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp)) {
					t.Fatalf("%s: raw %U in the text:\n%s", rep.Command, r, text.String())
				}
			}
		}
	})
}
