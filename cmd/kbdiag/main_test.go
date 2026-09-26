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
		{"session without pid", []string{"session"}, 64, false},
		{"session with two pids", []string{"session", "1", "2"}, 64, false},
		{"session pid not a number", []string{"session", "abc"}, 64, false},
		{"session pid zero", []string{"session", "0"}, 64, false},
		{"session pid negative", []string{"session", "--", "-5"}, 64, false},
		{"session pid overflows int32", []string{"session", "2147483648"}, 64, false},
		{"xact-fail below xact-warn", []string{"txn", "--xact-warn", "600", "--xact-fail", "60"}, 64, false},
		{"status has no threshold flags", []string{"status", "--conn-warn", "80"}, 64, false},
		{"status has no conn-fail", []string{"status", "--conn-fail", "100"}, 64, false},
		{"status takes no argument", []string{"status", "x"}, 64, false},
		{"locks bad limit", []string{"locks", "--limit", "x"}, 64, false},
		{"waits takes no argument", []string{"waits", "x"}, 64, false},
		{"session cannot connect", []string{"session", "1", "--host", sock}, 69, false},
		{"locks cannot connect", []string{"locks", "--host", sock}, 69, false},
		{"txn cannot connect", []string{"txn", "--host", sock}, 69, false},
		{"waits cannot connect", []string{"waits", "--host", sock}, 69, false},
		{"status cannot connect", []string{"status", "--host", sock}, 69, false},
		{"slots cannot connect", []string{"slots", "--host", sock}, 69, false},
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
