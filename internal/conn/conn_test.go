package conn

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestShortVersion(t *testing.T) {
	cases := map[string]string{
		"KingbaseES V008R006C009B0014 on x86_64-pc-linux-gnu, compiled by gcc (GCC) 4.8.5 20150623 (Red Hat 4.8.5-28), 64-bit": "KingbaseES V008R006C009B0014",
		"KingbaseES V008R006C009B0014": "KingbaseES V008R006C009B0014",
		"KingbaseES":                   "KingbaseES",
		"":                             "",
		"  spaced   out  ":             "spaced out",
	}
	for in, want := range cases {
		if got := shortVersion(in); got != want {
			t.Errorf("shortVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func FuzzShortVersion(f *testing.F) {
	f.Add("KingbaseES V008R006C009B0014 on x86_64")
	f.Fuzz(func(t *testing.T, s string) { shortVersion(s) })
}

func TestLocal(t *testing.T) {
	for host, want := range map[string]bool{"": true, "/tmp": true, "/var/run/kes": true, "127.0.0.1": false, "kes-node1": false} {
		if got := (Config{Host: host}).Local(); got != want {
			t.Errorf("Local(%q) = %v, want %v", host, got, want)
		}
	}
}

// statement_timeout=0 means no timeout, so a sub-millisecond value must not
// round down to 0.
func TestTimeoutMillis(t *testing.T) {
	cases := map[time.Duration]int64{
		time.Nanosecond:                    1,
		999 * time.Microsecond:             1,
		time.Millisecond:                   1,
		time.Millisecond + time.Nanosecond: 2,
		10 * time.Second:                   10000,
	}
	for d, want := range cases {
		if got := timeoutMillis(d); got != want {
			t.Errorf("timeoutMillis(%v) = %d, want %d", d, got, want)
		}
	}
}

func TestDSNQuoting(t *testing.T) {
	d := Config{Host: "", Port: 54321, User: `o'brien\x`, DBName: "test db"}.dsn()
	want := `host='/tmp' port='54321' user='o\'brien\\x' dbname='test db'`
	if d != want {
		t.Errorf("dsn = %s, want %s", d, want)
	}
}

// Every connection, socket or TCP, is read-only with lock and statement timeouts.
func TestConnConfigSafety(t *testing.T) {
	for _, host := range []string{"", "127.0.0.1"} {
		cc, err := connConfig(Config{Host: host, Port: 54321, User: "system", DBName: "test", QueryTimeout: 10 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{"default_transaction_read_only": "on", "lock_timeout": "500ms", "statement_timeout": "10000"}
		for k, v := range want {
			if cc.RuntimeParams[k] != v {
				t.Errorf("host %q: %s = %q, want %q", host, k, cc.RuntimeParams[k], v)
			}
		}
	}
}

// A missing socket fails fast, and the error names the KES socket, not .s.PGSQL.
func TestOpenMissingSocket(t *testing.T) {
	_, err := Open(context.Background(), Config{Host: t.TempDir(), Port: 54321, User: "system", DBName: "test", QueryTimeout: time.Second})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), ".s.KINGBASE.54321") || strings.Contains(err.Error(), ".s.PGSQL.") {
		t.Errorf("error = %v", err)
	}
}
