package scenario

import (
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
)

func Repl(c facts.Context, d facts.ReplDownstreams, s facts.ReplSync, u facts.InstUpstream, r facts.ReplReplay) *report.Report {
	rep := report.New("repl", c, rule.Repl(d, s, u, r))
	rep.AddProbe(facts.ReplSyncID, s.Status, s.Reason, facts.ReplSyncColumns, rows(s.Rows), 0)
	rep.AddProbe(facts.InstUpstreamID, u.Status, u.Reason, facts.UpstreamColumns, rows(u.Rows), 0)
	rep.AddProbe(facts.ReplReplayID, r.Status, r.Reason, facts.ReplReplayColumns, rows(r.Rows), 0)
	rep.AddProbe(facts.ReplDownstreamsID, d.Status, d.Reason, facts.ReplDownstreamColumns, rows(d.Rows), 0)
	rep.AddRedacted(u.Redacted())
	rep.SetRepl(d)
	return rep
}
