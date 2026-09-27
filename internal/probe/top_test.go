package probe

import (
	"strings"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

func TestTopCollecting(t *testing.T) {
	s := func(v string) *string { return &v }
	cases := []struct {
		name   string
		view   bool
		track  *string
		st     facts.Status
		reason string
	}{
		{"lab: installed, track none (stage 0)", true, s("none"), facts.StatusSkipped, "sys_stat_statements.track=none"},
		{"collecting", true, s("top"), facts.StatusOK, ""},
		{"all", true, s("all"), facts.StatusOK, ""},
		{"not installed in this database", false, s("top"), facts.StatusSkipped, "not installed in database test"},
		{"not loaded", true, nil, facts.StatusSkipped, "shared_preload_libraries"},
		{"neither", false, nil, facts.StatusSkipped, "shared_preload_libraries"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, reason := topCollecting(c.view, c.track, "test")
			if st != c.st || !strings.Contains(reason, c.reason) {
				t.Errorf("st=%s reason=%q", st, reason)
			}
		})
	}
}
