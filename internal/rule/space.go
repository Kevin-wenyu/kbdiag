package rule

import "github.com/Kevin-wenyu/kbdiag/internal/facts"

// Display is the verdict of a command that only shows: OK says every probe
// was collected and nothing was hidden, never that the numbers are fine; a
// probe not collected or a hidden column makes it UNKNOWN.
func Display(hidden bool, sts ...facts.Status) Result {
	unknown := hidden
	for _, st := range sts {
		_, x := collected(st)
		unknown = unknown || x
	}
	return Result{Verdict: verdictOf(nil, unknown)}
}

// Space only shows: how much is left on a disk is not wrong in itself, since
// how much is too little depends on the database.
func Space(d facts.InstDatabases, t facts.SpaceTablespaces, w facts.SpaceWAL, disk facts.SpaceDisk) Result {
	hidden := len(d.Redacted())+len(t.Redacted()) > 0
	return Display(hidden, d.Status, t.Status, w.Status, disk.Status)
}
