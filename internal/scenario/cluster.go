package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

// Cluster reads repmgr's metadata and checks it against this node's own
// view (its role, and on a primary its walsenders).
func Cluster(c facts.Context, n facts.ClusterNodes, e facts.ClusterEvents, d facts.InstDownstreams, s facts.ClusterSyncs) *report.Report {
	rep := report.New("cluster", c, rule.Cluster(n, d, s, c.Role))
	rep.AddProbe(facts.ClusterNodesID, n.Status, n.Reason, facts.ClusterNodeColumns, rows(n.Rows), 0)
	rep.AddProbe(facts.ClusterEventsID, e.Status, e.Reason, facts.ClusterEventColumns, rows(e.Rows), 0)
	rep.AddProbe(facts.InstDownstreamsID, d.Status, d.Reason, facts.DownstreamsColumns, rows(d.Rows), 0)
	rep.AddProbe(facts.ClusterSyncID, s.Status, s.Reason, facts.ClusterSyncColumns, rows(s.Rows), 0)
	rep.SetCluster(n, e, d, s)
	return rep
}
