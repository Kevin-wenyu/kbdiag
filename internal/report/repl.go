package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// replView is the repl text layout (plan 2026-09-27 appendix B.6): on a
// primary the synchronous settings, then each downstream and how far it is
// behind; on a standby its upstream and replay first.
type replView struct{ d facts.ReplDownstreams }

func (r *Report) SetRepl(d facts.ReplDownstreams) {
	v := &replView{d: d}
	r.layout = func(r *Report, w io.Writer) error { return v.write(r, w) }
}

func (v *replView) write(r *Report, w io.Writer) error {
	if r.Context.Role == "standby" {
		if p := r.Data[facts.InstUpstreamID]; p.Status != facts.StatusOK {
			writeNotOKAs(w, "upstream", p.Status, reasonOf(p))
		} else {
			if err := writeUpstreamAs(w, p, "upstream"); err != nil {
				return err
			}
		}
		writeReplay(w, r.Data[facts.ReplReplayID])
	} else {
		writeSync(w, r.Data[facts.ReplSyncID], v.d)
	}
	return v.writeDownstreams(w)
}

func writeSync(w io.Writer, p Probe, d facts.ReplDownstreams) {
	if p.Status != facts.StatusOK {
		writeNotOKAs(w, "sync", p.Status, reasonOf(p))
		return
	}
	for _, row := range p.rows() {
		names := ""
		if !isNil(row["synchronous_standby_names"]) {
			names = *row["synchronous_standby_names"].(*string)
		}
		fmt.Fprintln(w, "\nsync")
		shown := "(none: asynchronous only)"
		kv := [][2]string{}
		if names != "" {
			shown = escapeControl(names)
			need, _, desc, ok := rule.SyncNames(names)
			switch {
			case !ok:
				shown += "  (not understood)"
			case need > 0:
				shown += "  (" + escapeControl(desc) + ")"
			}
			kv = append(kv, [2]string{"synchronous_standby_names", shown}, [2]string{"synchronous_commit", cell(row["synchronous_commit"])})
			if ok && need > 0 && d.Status == facts.StatusOK {
				s := rule.SyncStreaming(d)
				line := fmt.Sprint(len(s))
				if len(s) > 0 {
					line += " (" + escapeControl(strings.Join(s, ", ")) + ")"
				}
				kv = append(kv, [2]string{"synchronous now", line})
			}
		} else {
			kv = append(kv, [2]string{"synchronous_standby_names", shown}, [2]string{"synchronous_commit", cell(row["synchronous_commit"])})
		}
		writeKV(w, 0, kv)
	}
}

func writeReplay(w io.Writer, p Probe) {
	if p.Status != facts.StatusOK {
		writeNotOKAs(w, "replay", p.Status, reasonOf(p))
		return
	}
	for _, row := range p.rows() {
		replayed := cell(row["replay_lsn"])
		if gap, ok := number(row["replay_gap_bytes"]); ok {
			if gap <= 0 {
				replayed += "  (caught up with what was received)"
			} else {
				replayed += "  (" + size(gap) + " received, not replayed yet)"
			}
		}
		last := "-"
		if s, ok := number(row["last_replay_age_s"]); ok {
			last = duration(s) + " ago"
		}
		paused := "no"
		if b, ok := row["replay_paused"].(bool); ok && b {
			paused = "yes"
		}
		fmt.Fprintln(w, "\nreplay")
		writeKV(w, 0, [][2]string{{"received", cell(row["receive_lsn"])}, {"replayed", replayed}, {"last replayed transaction", last}, {"paused", paused}})
	}
}

func (v *replView) writeDownstreams(w io.Writer) error {
	if v.d.Status != facts.StatusOK {
		writeNotOKAs(w, "downstreams", v.d.Status, v.d.Reason)
		return nil
	}
	fmt.Fprintf(w, "\ndownstreams: %d", len(v.d.Rows))
	if len(v.d.Rows) == 0 {
		fmt.Fprintln(w)
		return nil
	}
	fmt.Fprintln(w, "  (bytes behind this node's current WAL position)")
	var rows [][]string
	for _, x := range v.d.Rows {
		reply := "-"
		if x.ReplyAgeS != nil {
			reply = duration(*x.ReplyAgeS) + " ago"
		}
		lag := "-"
		if x.ReplayLagS != nil {
			lag = duration(*x.ReplayLagS)
		}
		rows = append(rows, []string{name(x.ApplicationName), cell(x.ClientAddr), cell(x.State), cell(x.SyncState),
			behind(x.SentLagBytes), behind(x.FlushLagBytes), behind(x.ReplayLagBytes), lag, reply})
	}
	return writeTable(w, "  ", []string{"name", "address", "state", "sync", "sent", "flushed", "replayed", "replay lag", "last reply"}, rows)
}

func behind(b *int64) string {
	if b == nil {
		return "-"
	}
	return size(max(0, float64(*b)))
}
