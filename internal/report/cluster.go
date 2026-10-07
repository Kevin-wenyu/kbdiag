package report

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// clusterView is the cluster text layout (plan 2026-09-27 appendix B.7):
// repmgr's nodes, how this node compares with them, then repmgr's latest
// events.
type clusterView struct {
	n facts.ClusterNodes
	e facts.ClusterEvents
	d facts.InstDownstreams
	s facts.ClusterSyncs
}

func (r *Report) SetCluster(n facts.ClusterNodes, e facts.ClusterEvents, d facts.InstDownstreams, s facts.ClusterSyncs) {
	v := &clusterView{n: n, e: e, d: d, s: s}
	r.layout = func(r *Report, w io.Writer) error { return v.write(r, w) }
}

func (v *clusterView) write(r *Report, w io.Writer) error {
	names := map[int32]string{}
	for _, x := range v.n.Rows {
		names[x.NodeID] = x.NodeName
	}
	if v.n.Status != facts.StatusOK {
		writeNotOKAs(w, "nodes", v.n.Status, v.n.Reason)
	} else if err := v.writeNodes(r, w, names); err != nil {
		return err
	}
	v.writeSync(w)
	if v.e.Status != facts.StatusOK {
		writeNotOKAs(w, "events", v.e.Status, v.e.Reason)
		return nil
	}
	fmt.Fprintf(w, "\nevents: %d  (the latest, at most %d)\n", len(v.e.Rows), facts.ClusterEventLimit)
	if len(v.e.Rows) == 0 {
		return nil
	}
	var rows [][]string
	for _, x := range v.e.Rows {
		node, ok := names[x.NodeID]
		if !ok {
			node = fmt.Sprint(x.NodeID)
		}
		details := "-"
		if x.Details != nil {
			details = fitWidth(escapeControl(strings.Join(strings.Fields(*x.Details), " ")), 100)
		}
		rows = append(rows, []string{x.Time.Format(time.DateTime), escapeControl(node), escapeControl(x.Event), map[bool]string{true: "yes", false: "no"}[x.Successful], details})
	}
	return writeTable(w, "  ", []string{"time", "node", "event", "ok", "details"}, rows)
}

func (v *clusterView) writeNodes(r *Report, w io.Writer, names map[int32]string) error {
	fmt.Fprintf(w, "\nnodes: %d  (repmgr metadata in database %s)\n", len(v.n.Rows), escapeControl(r.Context.Database))
	if len(v.n.Rows) == 0 {
		return nil
	}
	var rows [][]string
	var local *facts.ClusterNode
	for i, x := range v.n.Rows {
		up := "-"
		if x.UpstreamNodeID != nil {
			up = fmt.Sprint(*x.UpstreamNodeID)
			if n, ok := names[*x.UpstreamNodeID]; ok {
				up = escapeControl(n)
			}
		}
		if x.IsLocal {
			local = &v.n.Rows[i]
		}
		rows = append(rows, []string{fmt.Sprint(x.NodeID), escapeControl(x.NodeName), escapeControl(x.Type), up, map[bool]string{true: "yes", false: "no"}[x.Active],
			cell(x.Priority), cell(x.Location), cell(x.SlotName)})
	}
	if err := writeTable(w, "  ", []string{"id", "name", "type", "upstream", "active", "priority", "location", "slot"}, rows); err != nil {
		return err
	}
	if local == nil {
		fmt.Fprintln(w, "  this node: not identified (no repmgr node's slot is this instance's primary_slot_name)")
		return nil
	}
	fmt.Fprintf(w, "  this node is %s: repmgr says %s, the database is a %s\n", escapeControl(local.NodeName), escapeControl(local.Type), r.Context.Role)
	if r.Context.Role != "primary" || local.Type != "primary" || v.d.Status != facts.StatusOK {
		return nil
	}
	var in, out []string
	for _, x := range rule.Followers(v.n, local.NodeID) {
		if rule.Attached(v.d, x.NodeName) {
			in = append(in, escapeControl(x.NodeName))
		} else {
			out = append(out, escapeControl(x.NodeName))
		}
	}
	if len(in) > 0 {
		fmt.Fprintf(w, "  attached here: %s\n", strings.Join(in, ", "))
	}
	if len(out) > 0 {
		fmt.Fprintf(w, "  not attached here: %s\n", strings.Join(out, ", "))
	}
	return nil
}

// writeSync shows repmgr's configured synchronous mode against the
// primary's list; on a standby, outside repmgr, or when nodes could not be
// read (which the text already says) there is nothing to show.
func (v *clusterView) writeSync(w io.Writer) {
	switch {
	case v.s.Status == facts.StatusNotApplicable, v.n.Status != facts.StatusOK: // nodes already said why
		return
	case v.s.Status != facts.StatusOK || len(v.s.Rows) != 1:
		writeNotOKAs(w, "synchronous", v.s.Status, v.s.Reason)
		return
	}
	s := v.s.Rows[0]
	mode := "(not set)"
	if s.Synchronous != nil {
		mode = escapeControl(*s.Synchronous)
	}
	names := "(empty: asynchronous)"
	if s.StandbyNames != nil && strings.TrimSpace(*s.StandbyNames) != "" {
		names = escapeControl(*s.StandbyNames)
	}
	fmt.Fprintln(w, "\nsynchronous")
	_ = writeTable(w, "  ", nil, [][]string{
		{"repmgr.conf", mode + "  (" + escapeControl(s.ConfPath) + ")"},
		{"synchronous_standby_names", names},
	})
}
