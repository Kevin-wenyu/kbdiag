package scenario

import (
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Kevin-wenyu/kbdiag/internal/report"
)

// contract is PRD §5.2: the columns of each v0.2 probe and the evidence
// fields of each v0.2 finding, plus the probe and finding ids §5.1 shows
// for v0.1, which v0.2 commands reuse.
type contract struct {
	columns  map[string][]string
	evidence map[string][]string
	v01      string // §5.1, searched for reused ids
}

var (
	prdContractOnce sync.Once
	prdContract     contract
	prdContractErr  error
)

var (
	paren    = regexp.MustCompile(`（[^）]*）`)
	backtick = regexp.MustCompile("`([^`]+)`")
)

func loadContract(t *testing.T) contract {
	t.Helper()
	prdContractOnce.Do(func() {
		b, err := os.ReadFile("../../docs/PRD.md")
		if err != nil {
			prdContractErr = err
			return
		}
		doc := string(b)
		i, j, k := strings.Index(doc, "### 5.1 "), strings.Index(doc, "### 5.2 "), strings.Index(doc, "## 6. ")
		if i < 0 || j < i || k < j {
			prdContractErr = os.ErrNotExist
			return
		}
		c := contract{columns: map[string][]string{}, evidence: map[string][]string{}, v01: doc[i:j]}
		for _, line := range strings.Split(doc[j:k], "\n") {
			cells := strings.Split(line, "|")
			if len(cells) < 5 || !strings.HasPrefix(strings.TrimSpace(cells[1]), "`") {
				continue
			}
			// notes in parentheses or after "；" are not fields
			cell, _, _ := strings.Cut(paren.ReplaceAllString(cells[len(cells)-2], ""), "；")
			var fields []string
			for _, m := range backtick.FindAllStringSubmatch(cell, -1) {
				fields = append(fields, m[1])
			}
			sort.Strings(fields)
			target := c.columns
			if len(cells) == 6 { // finding.id | level | command | evidence
				target = c.evidence
			}
			for _, m := range backtick.FindAllStringSubmatch(cells[1], -1) {
				target[m[1]] = fields
			}
		}
		prdContract = c
	})
	if prdContractErr != nil {
		t.Fatalf("PRD §5.2: %v", prdContractErr)
	}
	return prdContract
}

// v01Commands are the commands whose contract is the §5.1 examples.
var v01Commands = []string{"status", "sessions", "session", "locks", "txn", "waits", "slots"}

// checkContract holds a v0.2 report to PRD §5.2: every probe it carries is
// registered there with exactly these columns (or is a v0.1 probe), and
// every finding's evidence has exactly the registered fields (plan stage 18).
func checkContract(t *testing.T, rep *report.Report) {
	t.Helper()
	if slices.Contains(v01Commands, rep.Command) {
		return
	}
	c := loadContract(t)
	for id, p := range rep.Data {
		want, ok := c.columns[id]
		if !ok {
			if !strings.Contains(c.v01, `"`+id+`"`) {
				t.Errorf("%s: probe %s is in neither PRD §5.2 nor §5.1", rep.Command, id)
			}
			continue
		}
		got := slices.Sorted(slices.Values(p.Columns))
		if !slices.Equal(got, want) {
			t.Errorf("%s: probe %s has columns %v, PRD §5.2 registers %v", rep.Command, id, got, want)
		}
	}
	for _, f := range rep.Findings {
		want, ok := c.evidence[f.ID]
		if !ok {
			if !strings.Contains(c.v01, `"`+f.ID+`"`) {
				t.Errorf("%s: finding %s is in neither PRD §5.2 nor §5.1", rep.Command, f.ID)
			}
			continue
		}
		var got []string
		for _, e := range f.Evidence {
			for k := range e.Fields {
				if !slices.Contains(got, k) {
					got = append(got, k)
				}
			}
		}
		sort.Strings(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s: finding %s has evidence %v, PRD §5.2 registers %v", rep.Command, f.ID, got, want)
		}
	}
}

// The parser finds the whole table: a format change would otherwise make
// checkContract pass vacuously.
func TestContractParses(t *testing.T) {
	c := loadContract(t)
	if len(c.columns) < 30 || len(c.evidence) < 13 {
		t.Fatalf("parsed %d probes and %d findings from PRD §5.2", len(c.columns), len(c.evidence))
	}
	if got := c.columns["progress.checkpoint"]; !slices.Contains(got, "waiting_lockers") {
		t.Errorf("progress.checkpoint = %v", got)
	}
	if got := c.evidence["seq.exhausted"]; len(got) != 7 {
		t.Errorf("seq.exhausted = %v", got)
	}
}
