// Package conn opens the single read-only diagnostic connection and
// identifies the instance behind it.
package conn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Kevin-wenyu/kbdiag/internal/facts"
)

type Config struct {
	Host         string // empty or an absolute path: Unix socket directory
	Port         int
	User         string
	DBName       string
	QueryTimeout time.Duration
}

const defaultSocketDir = "/tmp"

// Local reports whether the connection goes through a Unix socket.
func (c Config) Local() bool {
	return c.Host == "" || strings.HasPrefix(c.Host, "/")
}

// Loopback reports whether a TCP connection names this machine. It may still
// reach another one through a forwarded port, so it alone does not make the
// connection local.
func (c Config) Loopback() bool {
	switch strings.ToLower(c.Host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func (c Config) dsn() string {
	host := c.Host
	if host == "" {
		host = defaultSocketDir
	}
	kv := [][2]string{{"host", host}, {"port", fmt.Sprint(c.Port)}, {"user", c.User}, {"dbname", c.DBName}}
	parts := make([]string, len(kv))
	for i, p := range kv {
		parts[i] = p[0] + "=" + quote(p[1])
	}
	return strings.Join(parts, " ")
}

func quote(v string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v) + "'"
}

// Open connects read-only with lock and statement timeouts. The password,
// if any, comes from PGPASSWORD or ~/.pgpass (handled by pgx).
func Open(ctx context.Context, c Config) (*pgx.Conn, error) {
	cc, err := connConfig(c)
	if err != nil {
		return nil, err
	}
	x, err := pgx.ConnectConfig(ctx, cc)
	if err != nil {
		return nil, errors.New(kingbaseSocket(err.Error()))
	}
	return x, nil
}

func connConfig(c Config) (*pgx.ConnConfig, error) {
	cc, err := pgx.ParseConfig(c.dsn())
	if err != nil {
		return nil, err
	}
	cc.ConnectTimeout = 5 * time.Second
	cc.RuntimeParams["application_name"] = "kbdiag"
	cc.RuntimeParams["default_transaction_read_only"] = "on"
	cc.RuntimeParams["lock_timeout"] = "500ms"
	cc.RuntimeParams["statement_timeout"] = fmt.Sprint(timeoutMillis(c.QueryTimeout))
	// KES names its socket .s.KINGBASE.<port>; pgx only knows .s.PGSQL.<port>.
	cc.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if network == "unix" {
			addr = kingbaseSocket(addr)
		}
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
	return cc, nil
}

func kingbaseSocket(s string) string {
	return strings.ReplaceAll(s, ".s.PGSQL.", ".s.KINGBASE.")
}

// Identify reads the report context. role is only the recovery role.
func Identify(ctx context.Context, x *pgx.Conn, c Config) (facts.Context, error) {
	var version, user string
	var standby bool
	err := x.QueryRow(ctx, "select version(), sys_is_in_recovery(), current_user::text").Scan(&version, &standby, &user)
	if err != nil {
		return facts.Context{}, err
	}
	out := facts.Context{Version: shortVersion(version), Role: "primary", Location: "remote", User: user, CollectedAt: time.Now()}
	if standby {
		out.Role = "standby"
	}
	if c.Local() {
		out.Location = "local"
	}
	return out, nil
}

// shortVersion keeps "KingbaseES V008R006C009B0014" from the full version()
// text ("... on x86_64-pc-linux-gnu, compiled by ...").
func shortVersion(v string) string {
	f := strings.Fields(v)
	if len(f) < 2 {
		return v
	}
	return f[0] + " " + f[1]
}

// timeoutMillis rounds up, so a positive sub-millisecond timeout does not
// become statement_timeout=0, which means no timeout.
func timeoutMillis(d time.Duration) int64 {
	return int64((d + time.Millisecond - 1) / time.Millisecond)
}
