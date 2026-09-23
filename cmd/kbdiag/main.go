// Command kbdiag diagnoses a KingbaseES instance from the command line.
// This file only parses flags and wires packages together.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Kevin-wenyu/kbdiag/internal/conn"
	"github.com/Kevin-wenyu/kbdiag/internal/probe"
	"github.com/Kevin-wenyu/kbdiag/internal/report"
	"github.com/Kevin-wenyu/kbdiag/internal/rule"
	"github.com/Kevin-wenyu/kbdiag/internal/scenario"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// exitError carries a non-usage exit code out of a command.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

// run returns the process exit code. Any error cobra raises on its own
// (unknown command or flag, bad arguments) is a usage error: 64, not 1,
// which would read as WARN.
func run(args []string, stdout, stderr io.Writer) int {
	root := newRoot(stdout)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.Execute()
	if err == nil {
		return 0
	}
	var ee *exitError
	if errors.As(err, &ee) {
		if ee.err != nil {
			fmt.Fprintln(stderr, "kbdiag:", ee.err)
		}
		return ee.code
	}
	fmt.Fprintln(stderr, "kbdiag:", err)
	fmt.Fprintln(stderr, "Run 'kbdiag --help' for usage.")
	return report.ExitUsage
}

type globalFlags struct {
	cfg  conn.Config
	json bool
}

func newRoot(stdout io.Writer) *cobra.Command {
	g := &globalFlags{}
	root := &cobra.Command{
		Use:           "kbdiag",
		Short:         "Diagnose a KingbaseES instance from the command line",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		// statement_timeout=0 would mean no timeout at all.
		PersistentPreRunE: func(*cobra.Command, []string) error {
			if g.cfg.QueryTimeout <= 0 {
				return fmt.Errorf("--timeout must be positive, got %v", g.cfg.QueryTimeout)
			}
			return nil
		},
	}
	pf := root.PersistentFlags()
	pf.StringVar(&g.cfg.Host, "host", "", "server host, or socket directory (default /tmp)")
	pf.IntVarP(&g.cfg.Port, "port", "p", 54321, "server port")
	pf.StringVarP(&g.cfg.User, "user", "U", "system", "database user (password from PGPASSWORD or ~/.pgpass)")
	pf.StringVarP(&g.cfg.DBName, "dbname", "d", "test", "database to connect to")
	pf.DurationVar(&g.cfg.QueryTimeout, "timeout", 10*time.Second, "statement timeout for each query")
	pf.BoolVar(&g.json, "json", false, "print the report as JSON")
	root.AddCommand(newSessions(g, stdout))
	return root
}

func newSessions(g *globalFlags, stdout io.Writer) *cobra.Command {
	o := scenario.SessionsOptions{Thresholds: rule.Defaults}
	c := &cobra.Command{
		Use:   "sessions",
		Short: "List sessions; flag long idle-in-transaction ones",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			x, err := conn.Open(ctx, g.cfg)
			if err != nil {
				return &exitError{code: report.ExitUnavailable, err: err}
			}
			defer x.Close(context.Background())
			info, err := conn.Identify(ctx, x, g.cfg)
			if err != nil {
				return &exitError{code: report.ExitUnavailable, err: fmt.Errorf("identify instance: %w", err)}
			}
			rep := scenario.Sessions(info, probe.SessionActivity(ctx, x), o)
			return write(rep, g.json, stdout)
		},
	}
	f := c.Flags()
	f.BoolVar(&o.ActiveOnly, "active", false, "show only sessions running a query")
	f.IntVar(&o.Limit, "limit", 50, "max rows to show, 0 for all (findings still cover every row)")
	f.Float64Var(&o.Thresholds.IdleInTxnWarnS, "idle-in-txn-warn", rule.Defaults.IdleInTxnWarnS, "seconds idle in transaction before WARN")
	return c
}

func write(rep *report.Report, asJSON bool, stdout io.Writer) error {
	var err error
	if asJSON {
		err = rep.WriteJSON(stdout)
	} else {
		err = rep.WriteText(stdout)
	}
	if err != nil {
		return &exitError{code: 3, err: err}
	}
	if code := report.ExitCode(rep.Verdict); code != 0 {
		return &exitError{code: code}
	}
	return nil
}
