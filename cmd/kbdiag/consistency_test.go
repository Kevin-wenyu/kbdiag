package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// nextCommand matches every "kbdiag ..." a finding's next step tells the
// reader to run, literal or built with fmt.Sprintf.
var nextCommand = regexp.MustCompile(`Command:\s*(?:fmt\.Sprintf\()?"(kbdiag[^"]*)"`)

// Every verify step points at a command and flags that exist: a renamed or
// removed flag would otherwise leave findings sending people to a usage
// error (stage 7 of the polish plan).
func TestNextStepsParse(t *testing.T) {
	var files []string
	for _, dir := range []string{"../../internal/rule", "../../internal/scenario"} {
		m, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range m {
			if !strings.HasSuffix(f, "_test.go") {
				files = append(files, f)
			}
		}
	}
	found := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range nextCommand.FindAllSubmatch(src, -1) {
			found++
			line := strings.ReplaceAll(string(m[1]), "%d", "123")
			args := strings.Fields(line)[1:]
			root := newRoot(&bytes.Buffer{}, &bytes.Buffer{})
			cmd, rest, err := root.Find(args)
			if err != nil || cmd == root {
				t.Errorf("%s: %q is not a command: %v", f, line, err)
				continue
			}
			if err := cmd.ParseFlags(rest); err != nil {
				t.Errorf("%s: %q: %v", f, line, err)
				continue
			}
			if err := cmd.ValidateArgs(cmd.Flags().Args()); err != nil {
				t.Errorf("%s: %q: %v", f, line, err)
			}
		}
	}
	if found < 5 {
		t.Fatalf("found %d next commands; the pattern no longer matches the rule code", found)
	}
}

// commandFlags lists a command's own flags as the README spells them.
func commandFlags(c *cobra.Command) []string {
	var out []string
	c.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Name != "help" {
			out = append(out, "--"+f.Name)
		}
	})
	sort.Strings(out)
	return out
}

// The README command tables (English and Chinese) list exactly the commands
// and flags the binary has, and the PRD §4 table the same commands.
func TestDocsListTheRealCommands(t *testing.T) {
	root := newRoot(&bytes.Buffer{}, &bytes.Buffer{})
	want := map[string][]string{}
	for _, c := range root.Commands() {
		if c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		want[c.Name()] = commandFlags(c)
	}
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	row := regexp.MustCompile("(?m)^\\| `([a-z]+)(?: <pid>)?` \\|.*\\| *([^|]*)\\|$")
	flag := regexp.MustCompile("`(--[a-z-]+)")
	tables := 0
	for _, part := range strings.Split(string(readme), "\n## ") {
		got := map[string][]string{}
		for _, m := range row.FindAllStringSubmatch(part, -1) {
			if _, ok := want[m[1]]; !ok {
				continue // a flag table row, not a command
			}
			var flags []string
			for _, f := range flag.FindAllStringSubmatch(m[2], -1) {
				flags = append(flags, f[1])
			}
			sort.Strings(flags)
			got[m[1]] = flags
		}
		if len(got) == 0 {
			continue
		}
		tables++
		for name, flags := range want {
			g, ok := got[name]
			if !ok {
				t.Errorf("README table %d has no row for %s", tables, name)
				continue
			}
			if !slices.Equal(g, flags) && !(len(g) == 0 && len(flags) == 0) {
				t.Errorf("README table %d, %s: flags %v, the command has %v", tables, name, g, flags)
			}
		}
		for name := range got {
			if _, ok := want[name]; !ok {
				t.Errorf("README table %d lists %s, which is not a command", tables, name)
			}
		}
	}
	if tables != 2 {
		t.Errorf("found %d README command tables, want 2 (English and Chinese)", tables)
	}

	prd, err := os.ReadFile("../../docs/PRD.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(prd)
	start := strings.Index(doc, "**v0.1 命令清单**")
	end := strings.Index(doc[start:], "开关感知")
	if start < 0 || end < 0 {
		t.Fatal("PRD §4 command table not found")
	}
	var listed []string
	for _, m := range regexp.MustCompile("(?m)^\\| `([a-z]+)").FindAllStringSubmatch(doc[start:start+end], -1) {
		listed = append(listed, m[1])
	}
	var names []string
	for n := range want {
		names = append(names, n)
	}
	sort.Strings(names)
	sort.Strings(listed)
	if !slices.Equal(names, listed) {
		t.Errorf("PRD §4 lists %v, the binary has %v", listed, names)
	}
}
