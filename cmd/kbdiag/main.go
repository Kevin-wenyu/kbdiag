// Command kbdiag diagnoses a KingbaseES instance from the command line.
// This file only parses flags and wires packages together.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"

	"github.com/Kevin-wenyu/kbdiag/internal/conn"
	"github.com/Kevin-wenyu/kbdiag/internal/facts"
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
	root := newRoot(stdout, stderr)
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

func newRoot(stdout, stderr io.Writer) *cobra.Command {
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
	root.AddCommand(newSessions(g, stdout), newSession(g, stdout, stderr), newLocks(g, stdout),
		newTxn(g, stdout), newWaits(g, stdout), newStatus(g, stdout), newSlots(g, stdout))
	return root
}

// diagnose opens the connection, identifies the instance and hands both to
// build. A failed identify is exit 3: we reached the server but cannot say
// what it is, which is not "unavailable".
func diagnose(ctx context.Context, g *globalFlags, stdout io.Writer, build func(*pgx.Conn, facts.Context) *report.Report) error {
	x, err := conn.Open(ctx, g.cfg)
	if err != nil {
		return &exitError{code: report.ExitUnavailable, err: err}
	}
	defer x.Close(context.Background())
	info, err := conn.Identify(ctx, x, g.cfg)
	if err != nil {
		return &exitError{code: report.ExitCode(rule.VerdictUNKNOWN), err: fmt.Errorf("identify instance: %w", err)}
	}
	return write(build(x, info), g.json, stdout)
}

func newSessions(g *globalFlags, stdout io.Writer) *cobra.Command {
	o := scenario.SessionsOptions{Thresholds: rule.Defaults}
	c := &cobra.Command{
		Use:   "sessions",
		Short: "List sessions; flag long idle-in-transaction ones",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Sessions(info, probe.SessionActivity(ctx, x), o)
			})
		},
	}
	f := c.Flags()
	f.BoolVar(&o.ActiveOnly, "active", false, "show only sessions running a query")
	limitFlag(c, &o.Limit)
	f.Float64Var(&o.Thresholds.IdleInTxnWarnS, "idle-in-txn-warn", rule.Defaults.IdleInTxnWarnS, "seconds idle in transaction before WARN")
	return c
}

func newSession(g *globalFlags, stdout, stderr io.Writer) *cobra.Command {
	o := scenario.SessionOptions{Thresholds: rule.Defaults}
	c := &cobra.Command{
		Use:   "session <pid>",
		Short: "Show one session: its SQL, waits, locks held and who blocks it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, err := strconv.ParseInt(args[0], 10, 32)
			if err != nil || pid <= 0 {
				return fmt.Errorf("pid must be a positive integer, got %q", args[0])
			}
			o.PID = int32(pid)
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				rep, found := scenario.Session(info, probe.SessionActivity(ctx, x), probe.LockList(ctx, x), o)
				if !found {
					fmt.Fprintf(stderr, "kbdiag: no session with pid %d (it may have ended)\n", o.PID)
				}
				return rep
			})
		},
	}
	f := c.Flags()
	f.Float64Var(&o.Thresholds.IdleInTxnWarnS, "idle-in-txn-warn", rule.Defaults.IdleInTxnWarnS, "seconds idle in transaction before WARN")
	f.Float64Var(&o.Thresholds.LockWaitWarnS, "lock-wait-warn", rule.Defaults.LockWaitWarnS, "seconds waiting for a lock before WARN")
	return c
}

func newLocks(g *globalFlags, stdout io.Writer) *cobra.Command {
	o := scenario.LocksOptions{Thresholds: rule.Defaults}
	c := &cobra.Command{
		Use:   "locks",
		Short: "List lock waits and their direct blockers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Locks(info, probe.LockList(ctx, x), o)
			})
		},
	}
	limitFlag(c, &o.Limit)
	c.Flags().Float64Var(&o.Thresholds.LockWaitWarnS, "lock-wait-warn", rule.Defaults.LockWaitWarnS, "seconds waiting for a lock before WARN")
	return c
}

func newTxn(g *globalFlags, stdout io.Writer) *cobra.Command {
	o := scenario.TxnOptions{Thresholds: rule.Defaults}
	c := &cobra.Command{
		Use:   "txn",
		Short: "List open transactions and prepared (2PC) ones; flag long ones",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if o.Thresholds.XactFailS < o.Thresholds.XactWarnS {
				return fmt.Errorf("--xact-fail (%v) must not be below --xact-warn (%v)", o.Thresholds.XactFailS, o.Thresholds.XactWarnS)
			}
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Txn(info, probe.SessionActivity(ctx, x), probe.TxnPrepared(ctx, x, info), o)
			})
		},
	}
	limitFlag(c, &o.Limit)
	f := c.Flags()
	f.Float64Var(&o.Thresholds.XactWarnS, "xact-warn", rule.Defaults.XactWarnS, "transaction age in seconds before WARN")
	f.Float64Var(&o.Thresholds.XactFailS, "xact-fail", rule.Defaults.XactFailS, "transaction age in seconds before FAIL")
	f.Float64Var(&o.Thresholds.PreparedFailS, "prepared-fail", rule.Defaults.PreparedFailS, "prepared transaction age in seconds before FAIL")
	return c
}

func newWaits(g *globalFlags, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "waits",
		Short: "Summarize what sessions are waiting on right now",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Waits(info, probe.WaitSummary(ctx, x))
			})
		},
	}
}

func newStatus(g *globalFlags, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show what this instance is and whether its basics hold: role, replication, connections, sizes, disk",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				i := probe.InstInfo(ctx, x)
				return scenario.Status(info, i, probe.InstDatabases(ctx, x), probe.InstDownstreams(ctx, x),
					probe.InstUpstream(ctx, x, info), probe.InstDisk(i, g.cfg.Local(), g.cfg.Loopback()))
			})
		},
	}
}

func newSlots(g *globalFlags, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "slots",
		Short: "List replication slots; flag inactive ones",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Slots(info, probe.SlotList(ctx, x))
			})
		},
	}
}

func limitFlag(c *cobra.Command, limit *int) {
	c.Flags().IntVar(limit, "limit", 50, "max rows to show, 0 for all (findings still cover every row)")
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
