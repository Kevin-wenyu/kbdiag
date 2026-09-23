package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestExitCodesWithoutDatabase(t *testing.T) {
	sock := t.TempDir() // no KES socket here
	cases := []struct {
		name string
		args []string
		code int
		out  bool // anything on stdout
	}{
		{"help", []string{"--help"}, 0, true},
		{"unknown command", []string{"nosuch"}, 64, false},
		{"unknown flag", []string{"sessions", "--nosuch"}, 64, false},
		{"bad flag value", []string{"sessions", "--limit", "x"}, 64, false},
		{"extra argument", []string{"sessions", "extra"}, 64, false},
		{"zero timeout would disable statement_timeout", []string{"sessions", "--timeout", "0s"}, 64, false},
		{"negative timeout", []string{"sessions", "--timeout", "-1s"}, 64, false},
		{"cannot connect", []string{"sessions", "--host", sock}, 69, false},
		{"cannot connect, json", []string{"sessions", "--json", "--host", sock}, 69, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(c.args, &stdout, &stderr)
			if code != c.code {
				t.Errorf("exit = %d, want %d (stderr: %s)", code, c.code, stderr.String())
			}
			if got := stdout.Len() > 0; got != c.out {
				t.Errorf("stdout written = %v, want %v: %q", got, c.out, stdout.String())
			}
			if c.code != 0 && stderr.Len() == 0 {
				t.Error("error must be explained on stderr")
			}
		})
	}
}

func TestConnectErrorNamesKingbaseSocket(t *testing.T) {
	var stdout, stderr bytes.Buffer
	run([]string{"sessions", "--host", t.TempDir()}, &stdout, &stderr)
	if !strings.Contains(stderr.String(), ".s.KINGBASE.54321") {
		t.Errorf("stderr = %q", stderr.String())
	}
}
