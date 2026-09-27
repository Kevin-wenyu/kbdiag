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
	columns  map[string][]string // in order
	commands map[string][]string // command → the probes §5.2 registers for it
	evidence map[string][]string // sorted
	v01      string              // §5.1, searched for reused finding ids
	v01cols  map[string][]string // probe → columns in the §5.1 examples
}

var (
	prdContractOnce sync.Once
	prdContract     contract
	prdContractErr  error
)

var (
	paren    = regexp.MustCompile(`（[^）]*）`)
	backtick = regexp.MustCompile("`([^`]+)`")
	example  = regexp.MustCompile(`"([a-z_]+\.[a-z_]+)": \{[^{}]*?"columns": \[([^\]]*)\]`)
	quoted   = regexp.MustCompile(`"([^"]+)"`)
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
		c := contract{columns: map[string][]string{}, commands: map[string][]string{}, evidence: map[string][]string{}, v01: doc[i:j], v01cols: map[string][]string{}}
		for _, m := range example.FindAllStringSubmatch(doc[i:j], -1) {
			var cols []string
			for _, q := range quoted.FindAllStringSubmatch(m[2], -1) {
				cols = append(cols, q[1])
			}
			c.v01cols[m[1]] = cols
		}
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
			for _, m := range backtick.FindAllStringSubmatch(cells[1], -1) {
				if len(cells) == 6 { // finding.id | level | command | evidence
					c.evidence[m[1]] = slices.Sorted(slices.Values(fields))
					continue
				}
				c.columns[m[1]] = fields
				cmd := strings.TrimSpace(cells[2])
				c.commands[cmd] = append(c.commands[cmd], m[1])
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

// checkContract holds a v0.2 report to PRD §5.2: it carries every probe
// registered for its command, each probe has exactly the registered columns
// in order (a reused v0.1 probe, those of its §5.1 example), and each
// evidence entry of a finding has exactly the registered fields (plan
// stage 18).
func checkContract(t *testing.T, rep *report.Report) {
	t.Helper()
	if slices.Contains(v01Commands, rep.Command) {
		return
	}
	c := loadContract(t)
	for _, id := range c.commands[rep.Command] {
		if _, ok := rep.Data[id]; !ok {
			t.Errorf("%s: no probe %s, which PRD §5.2 registers for it", rep.Command, id)
		}
	}
	for id, p := range rep.Data {
		want, ok := c.columns[id]
		if !ok {
			if want, ok = c.v01cols[id]; !ok {
				t.Errorf("%s: probe %s is in neither PRD §5.2 nor the §5.1 examples", rep.Command, id)
				continue
			}
		}
		if !slices.Equal(p.Columns, want) {
			t.Errorf("%s: probe %s has columns %v, the PRD registers %v", rep.Command, id, p.Columns, want)
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
		// each evidence entry carries every registered field
		for _, e := range f.Evidence {
			var got []string
			for k := range e.Fields {
				got = append(got, k)
			}
			sort.Strings(got)
			if !slices.Equal(got, want) {
				t.Errorf("%s: finding %s has evidence %v, PRD §5.2 registers %v", rep.Command, f.ID, got, want)
			}
		}
	}
}

// The parser finds the whole table: a format change would otherwise make
// checkContract pass vacuously.
func TestContractParses(t *testing.T) {
	c := loadContract(t)
	if len(c.columns) < 30 || len(c.evidence) < 13 || len(c.commands) < 14 {
		t.Fatalf("parsed %d probes, %d findings and %d commands from PRD §5.2", len(c.columns), len(c.evidence), len(c.commands))
	}
	for _, id := range []string{"session.activity", "lock.list", "slot.list", "inst.databases", "inst.upstream", "inst.downstreams", "inst.disk"} {
		if len(c.v01cols[id]) == 0 {
			t.Errorf("no columns for %s in the PRD §5.1 examples", id)
		}
	}
	if got := c.columns["progress.checkpoint"]; !slices.Contains(got, "waiting_lockers") {
		t.Errorf("progress.checkpoint = %v", got)
	}
	if got := c.evidence["seq.exhausted"]; len(got) != 7 {
		t.Errorf("seq.exhausted = %v", got)
	}
}
