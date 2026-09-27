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
	"strings"
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
		newTxn(g, stdout), newWaits(g, stdout), newStatus(g, stdout), newSlots(g, stdout),
		newSpace(g, stdout), newFreeze(g, stdout), newVacuum(g, stdout),
		newArchive(g, stdout), newParams(g, stdout), newRepl(g, stdout),
		newCluster(g, stdout), newTopObjects(g, stdout), newTable(g, stdout, stderr),
		newTop(g, stdout), newProgress(g, stdout), newCheckpoint(g, stdout))
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
		Short: "Who holds the connections and which sessions are doing something; flag long idle-in-transaction ones",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Sessions(info, probe.SessionActivity(ctx, x), o)
			})
		},
	}
	f := c.Flags()
	f.BoolVar(&o.All, "all", false, "list every session, including idle ones and background processes")
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
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Txn(info, probe.SessionActivity(ctx, x), probe.TxnPrepared(ctx, x, info), o)
			})
		},
	}
	limitFlag(c, &o.Limit)
	f := c.Flags()
	f.Float64Var(&o.Thresholds.XactWarnS, "xact-warn", rule.Defaults.XactWarnS, "transaction age in seconds before WARN")
	f.Float64Var(&o.Thresholds.PreparedWarnS, "prepared-warn", rule.Defaults.PreparedWarnS, "prepared transaction age in seconds before WARN")
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

func newSpace(g *globalFlags, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "space",
		Short: "Show where the space goes: filesystems, WAL, databases, tablespaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				t := probe.SpaceTablespaces(ctx, x)
				disk := probe.SpaceDisk(probe.InstInfo(ctx, x), t, g.cfg.Local(), g.cfg.Loopback())
				return scenario.Space(info, probe.InstDatabases(ctx, x), t, probe.SpaceWAL(ctx, x), disk)
			})
		},
	}
}

func newFreeze(g *globalFlags, stdout io.Writer) *cobra.Command {
	var o scenario.FreezeOptions
	c := &cobra.Command{
		Use:   "freeze",
		Short: "How far each database is from transaction ID wraparound; the oldest tables of this one",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Freeze(info, probe.FreezeDatabases(ctx, x), probe.FreezeTables(ctx, x), probe.FreezeLimits(ctx, x), o)
			})
		},
	}
	limitFlagN(c, &o.Limit, 20)
	return c
}

func newVacuum(g *globalFlags, stdout io.Writer) *cobra.Command {
	var o scenario.VacuumOptions
	c := &cobra.Command{
		Use:   "vacuum",
		Short: "Which tables have the most dead tuples, whether autovacuum will clean them, and what is vacuuming now",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Vacuum(info, probe.VacuumTables(ctx, x, info), probe.VacuumProgress(ctx, x, info), probe.VacuumSettings(ctx, x), o)
			})
		},
	}
	limitFlagN(c, &o.Limit, 20)
	return c
}

func newArchive(g *globalFlags, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "archive",
		Short: "Whether WAL archiving works: settings, last success and failure, WAL waiting to be archived",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Archive(info, probe.ArchiveStatus(ctx, x), probe.ArchiveReady(ctx, x))
			})
		},
	}
}

func newParams(g *globalFlags, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "params",
		Short: "Which parameters are not at their default and where they are set; flag changes waiting for a restart",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Params(info, probe.ParamsChanged(ctx, x))
			})
		},
	}
}

func newRepl(g *globalFlags, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "repl",
		Short: "Replication from this node's side: each standby's lag and the synchronous settings on a primary; receive and replay on a standby",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Repl(info, probe.ReplDownstreams(ctx, x), probe.ReplSync(ctx, x, info), probe.InstUpstream(ctx, x, info), probe.ReplReplay(ctx, x, info))
			})
		},
	}
}

// repmgrDB is where KES keeps repmgr's metadata (stage 0 of plan
// 2026-09-27); cluster connects there unless -d is given.
const repmgrDB = "esrep"

func newCluster(g *globalFlags, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "cluster",
		Short: "repmgr's view of the cluster (nodes, roles, upstreams, latest events), checked against this node; connects to database esrep unless -d is given",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			build := func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Cluster(info, probe.ClusterNodes(ctx, x, info), probe.ClusterEvents(ctx, x, info), probe.InstDownstreams(ctx, x))
			}
			if f := cmd.Flag("dbname"); f != nil && f.Changed {
				return diagnose(ctx, g, stdout, build)
			}
			// no esrep database: not a KES repmgr cluster; the default
			// database then says so (no repmgr schema: not_applicable)
			cg := *g
			cg.cfg.DBName = repmgrDB
			if x, err := conn.Open(ctx, cg.cfg); err == nil {
				x.Close(ctx)
			} else if conn.MissingDatabase(err) {
				return diagnose(ctx, g, stdout, build)
			}
			return diagnose(ctx, &cg, stdout, build)
		},
	}
}

func newTopObjects(g *globalFlags, stdout io.Writer) *cobra.Command {
	var o scenario.TopObjectsOptions
	c := &cobra.Command{
		Use:   "top-objects",
		Short: "The largest tables (heap, indexes, TOAST) and indexes of this database",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.TopObjects(info, probe.ObjectTables(ctx, x), probe.ObjectIndexes(ctx, x), o)
			})
		},
	}
	limitFlagN(c, &o.Limit, 20)
	return c
}

func newTable(g *globalFlags, stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "table <name>",
		Short: "One table: size, rows, vacuum and analyze, freeze age, access, indexes (the name follows SQL rules: unquoted folds to lower case)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("table name must not be empty")
			}
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				i := probe.TableInfo(ctx, x, name)
				st, reason := scenario.TableAfter(i)
				z := facts.TableSizes{Status: st, Reason: reason}
				s := facts.TableStats{Status: st, Reason: reason}
				ix := facts.TableIndexes{Status: st, Reason: reason}
				if i.Status == facts.StatusOK && len(i.Rows) == 1 && scenario.TableKinds[i.Rows[0].Relkind] {
					oid := i.Rows[0].OID
					z, s, ix = probe.TableSize(ctx, x, oid), probe.TableStats(ctx, x, info, oid), probe.TableIndexes(ctx, x, oid)
				}
				rep, found := scenario.Table(info, i, z, s, ix, probe.FreezeLimits(ctx, x), probe.VacuumSettings(ctx, x))
				switch {
				case found:
				case len(i.Rows) == 0:
					fmt.Fprintf(stderr, "kbdiag: no table %q in database %s (unquoted names fold to lower case; quote them as in SQL: '\"Name\"'; use -d for another database)\n", name, info.Database)
				default:
					fmt.Fprintf(stderr, "kbdiag: %q is not a table (relkind %s)\n", name, i.Rows[0].Relkind)
				}
				return rep
			})
		},
	}
}

func newTop(g *globalFlags, stdout io.Writer) *cobra.Command {
	o := scenario.TopOptions{By: "time"}
	c := &cobra.Command{
		Use:   "top",
		Short: "Cumulative top SQL from sys_stat_statements; says so when statements are not being collected",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, ok := scenario.TopOrders[o.By]; !ok {
				return fmt.Errorf("--by must be time, mean, calls, io or temp, got %q", o.By)
			}
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Top(info, probe.SQLTop(ctx, x, info), o)
			})
		},
	}
	limitFlagN(c, &o.Limit, 20)
	c.Flags().StringVar(&o.By, "by", "time", "order: time (total), mean, calls, io (blocks read), temp (temp blocks written)")
	return c
}

func newProgress(g *globalFlags, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "progress",
		Short: "How far running VACUUM, CREATE INDEX, CLUSTER / VACUUM FULL and CHECKPOINT have got",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Progress(info, probe.ProgressList(ctx, x))
			})
		},
	}
}

func newCheckpoint(g *globalFlags, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "checkpoint",
		Short: "The last checkpoint, timed vs requested checkpoints, who writes dirty buffers, and the settings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return diagnose(ctx, g, stdout, func(x *pgx.Conn, info facts.Context) *report.Report {
				return scenario.Checkpoint(info, probe.CheckpointStats(ctx, x), probe.CheckpointLast(ctx, x), probe.CheckpointSettings(ctx, x))
			})
		},
	}
}

func limitFlag(c *cobra.Command, limit *int) { limitFlagN(c, limit, 50) }

func limitFlagN(c *cobra.Command, limit *int, n int) {
	c.Flags().IntVar(limit, "limit", n, "max rows to show, 0 for all (findings still cover every row)")
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
