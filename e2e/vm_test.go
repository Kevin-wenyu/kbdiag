//go:build vm

// Package e2e runs the real kbdiag binary on a Lima VM (engineering.md §6.3):
// cross-compile, copy to KB_TEST_NODE, run as kingbase, parse the JSON.
// Tests share one instance and inject instance-wide state, so none run in parallel.
package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const remoteBin = "./kbdiag-e2e" // relative to kingbase's home (sudo -i)

var (
	node string
	role string // primary | standby, read from the instance, not assumed from the node name
)

func TestMain(m *testing.M) {
	node = os.Getenv("KB_TEST_NODE")
	if node == "" {
		fmt.Fprintln(os.Stderr, "KB_TEST_NODE is required, e.g. KB_TEST_NODE=kes-node1")
		os.Exit(2)
	}
	if err := deploy(); err != nil {
		fmt.Fprintln(os.Stderr, "deploy:", err)
		os.Exit(2)
	}
	out, err := ksqlErr("select sys_is_in_recovery()")
	if err != nil {
		fmt.Fprintln(os.Stderr, "read role:", err)
		os.Exit(2)
	}
	role = map[string]string{"t": "standby", "f": "primary"}[out]
	code := m.Run()
	vm("rm", "-f", remoteBin).Run()
	os.Exit(code)
}

// deploy builds a linux binary and streams it to the node as kingbase, so the
// file is owned by the user that runs it.
func deploy() error {
	arch, err := vm("uname", "-m").Output()
	if err != nil {
		return fmt.Errorf("uname: %w", err)
	}
	goarch := map[string]string{"x86_64": "amd64", "aarch64": "arm64"}[strings.TrimSpace(string(arch))]
	if goarch == "" {
		return fmt.Errorf("unsupported arch %q", arch)
	}
	bin := filepath.Join(os.TempDir(), "kbdiag-e2e-"+goarch)
	build := exec.Command("go", "build", "-o", bin, "../cmd/kbdiag")
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+goarch, "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %w\n%s", err, out)
	}
	f, err := os.Open(bin)
	if err != nil {
		return err
	}
	defer f.Close()
	up := vm("sh", "-c", "cat > "+remoteBin+" && chmod 755 "+remoteBin)
	up.Stdin = f
	if out, err := up.CombinedOutput(); err != nil {
		return fmt.Errorf("copy: %w\n%s", err, out)
	}
	return nil
}

// vm runs a command on the node as kingbase. Arguments are joined by a
// remote shell, so they must not contain spaces or quotes.
func vm(args ...string) *exec.Cmd {
	return exec.Command("limactl", append([]string{"shell", node, "sudo", "-iu", "kingbase"}, args...)...)
}

func ksqlErr(sql string) (string, error) {
	c := vm("ksql", "-d", "test", "-U", "system", "-p", "54321", "-v", "ON_ERROR_STOP=1", "-Atq", "-f", "-")
	c.Stdin = strings.NewReader(sql)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, stderr.String())
	}
	return strings.TrimSpace(string(out)), nil
}

// ksql runs one query as system over the local socket; it is the independent
// source of injection fingerprints, never kbdiag itself.
func ksql(t *testing.T, sql string) string {
	t.Helper()
	out, err := ksqlErr(sql)
	if err != nil {
		t.Fatalf("ksql %q: %v", sql, err)
	}
	return out
}

// inject brings an e2e/inject scenario up and registers its teardown, which
// runs whether or not the test passes. A leftover from a run that crashed
// before its teardown is cleared first, so fingerprints stay unambiguous.
func inject(t *testing.T, name string) {
	t.Helper()
	script := filepath.Join("inject", name+".sh")
	if out, err := injectCmd(script, "down").CombinedOutput(); err != nil {
		t.Fatalf("%s pre-clean: %v\n%s", name, err, out)
	}
	t.Cleanup(func() {
		if out, err := injectCmd(script, "down").CombinedOutput(); err != nil {
			t.Errorf("%s down: %v\n%s", name, err, out)
		}
	})
	if out, err := injectCmd(script, "up").CombinedOutput(); err != nil {
		t.Fatalf("%s up: %v\n%s", name, err, out)
	}
}

func injectCmd(script, action string) *exec.Cmd {
	c := exec.Command("bash", script, action)
	c.Env = append(os.Environ(), "KB_TEST_NODE="+node)
	return c
}

// report mirrors the PRD §5 contract on its own (not internal/report), so a
// silent rename in the code breaks these tests.
type report struct {
	Command string `json:"command"`
	Verdict string `json:"verdict"`
	Context struct {
		Version  string `json:"version"`
		Role     string `json:"role"`
		Location string `json:"location"`
		User     string `json:"user"`
	} `json:"context"`
	Data     map[string]probeData `json:"data"`
	Findings []struct {
		ID       string `json:"id"`
		Level    string `json:"level"`
		Evidence []struct {
			ProbeID string         `json:"probe_id"`
			Fields  map[string]any `json:"fields"`
		} `json:"evidence"`
		Next []struct {
			Kind    string `json:"kind"`
			Command string `json:"command"`
		} `json:"next"`
	} `json:"findings"`
	Redacted []struct {
		ProbeID      string `json:"probe_id"`
		Field        string `json:"field"`
		Reason       string `json:"reason"`
		RowsAffected int    `json:"rows_affected"`
	} `json:"redacted"`
}

type probeData struct {
	Status    string   `json:"status"`
	Reason    *string  `json:"reason"`
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Truncated int      `json:"truncated"`
}

// row returns the row whose column col equals v, as a column → value map.
func (p probeData) row(col string, v any) map[string]any {
	for _, r := range p.Rows {
		m := map[string]any{}
		for i, c := range p.Columns {
			m[c] = r[i]
		}
		if m[col] == v {
			return m
		}
	}
	return nil
}

// kbdiag runs the binary with --json and returns the parsed report and exit code.
// env entries are KEY=VALUE, passed through env(1).
func kbdiag(t *testing.T, env []string, args ...string) (report, int) {
	t.Helper()
	full := append(append([]string{"env"}, env...), remoteBin)
	c := vm(append(append(full, args...), "--json")...)
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	code := 0
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run kbdiag: %v", err)
		}
		code = ee.ExitCode()
	}
	var r report
	if err := json.Unmarshal(stdout.Bytes(), &r); err != nil {
		t.Fatalf("exit %d, stdout is not a report: %v\nstdout: %s\nstderr: %s", code, err, stdout.String(), stderr.String())
	}
	return r, code
}

// kbdiagText runs the binary without --json and returns stdout and the exit code.
func kbdiagText(t *testing.T, env []string, args ...string) (string, int) {
	t.Helper()
	full := append(append([]string{"env"}, env...), remoteBin)
	c := vm(append(full, args...)...)
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	code := 0
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run kbdiag: %v", err)
		}
		code = ee.ExitCode()
	}
	return stdout.String(), code
}
