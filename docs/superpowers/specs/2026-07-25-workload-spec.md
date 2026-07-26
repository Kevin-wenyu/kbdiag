# `kbdiag workload` Spec

Source material: `docs/superpowers/specs/2026-07-25-capability-gap-research.md` (capability-gap research vs. `ora`/pg_profile/pgBadger/pgmetrics/pganalyze) and `docs/superpowers/specs/2026-07-25-workload-requirements.md` (grilled requirements, definitive). This spec synthesizes that conversation — no new decisions made here beyond what was already grilled.

## Problem Statement

kbdiag's diagnostic commands are all point-in-time snapshots — `perf`, `stat`, `wait`, `locks` each answer "what does the instance look like right now." There is no command that answers "how did the workload change between two points in time" — no historical wait-event trend, no SQL-time diff, no metric trend across a window. This is the single biggest capability gap kbdiag has relative to Oracle's `ora` (AWR/ASH-style historical reports) and PostgreSQL's pg_profile, and it was flagged directly by the user as evidence kbdiag is less capable than those tools.

kbdiag's architecture is deliberately stateless, single-file, pure-shell — no daemon, no background sampling process, no kbdiag-maintained metrics repository. A naive port of pg_profile's approach (kbdiag samples and stores its own history) would violate that architecture and was explicitly rejected previously (`docs/superpowers/specs/2026-07-14-next-features-requirements.md`).

## Solution

KingbaseES itself already persists the state needed for this, without kbdiag doing any storing of its own:

- `sys_kwr`, an optional extension, maintains its own AWR/ASH-equivalent snapshot repository inside the database and can render a point-in-time-diff report via `perf.kwr_report(start_id, end_id, 'text')`.
- `sys_stat_sysmetric_history`, a built-in view, rolls a lighter-weight metric history (CPU/wait-class trend) once the `track_real_stats` GUC is turned on — zero installation cost, coarser granularity.

Add a new DBA-tier (查层) command, `kbdiag workload`, that detects which of these two data sources is available, degrades gracefully through a fixed priority order when neither or only the lighter one is available, and surfaces KingbaseES's own generated report content unmodified. kbdiag's value-add is entirely in "should a report be generated, and from which source" — not in re-parsing or re-formatting content KingbaseES already produced.

## User Stories

1. As a DBA with `sys_kwr` installed and enabled, I want `kbdiag workload` to give me a diff report between the two most recent snapshots by default, so that I can see recent workload change without having to remember snapshot IDs.
2. As a DBA with `sys_kwr` installed, I want to pass `--from 2h --to 30m` to scope the report to a specific window, so that I can investigate a specific incident window instead of always looking at the most recent interval.
3. As a DBA without `sys_kwr` installed but where it's available to install, I want `workload` to tell me it exists and how valuable it would be, so that I know an upgrade path exists without kbdiag silently doing nothing.
4. As a DBA without `sys_kwr` and without `track_real_stats` enabled, I want `workload` to tell me exactly which GUC to flip and how, so that I can opt into the lightweight path myself.
5. As a DBA with only `track_real_stats=on` (no `sys_kwr`), I want `workload` to give me a metric-trend report over the last 15 minutes by default, so that I still get some historical signal even without the heavier extension.
6. As a DBA running `workload` for the first time on an instance with fewer than two `sys_kwr` snapshots, I want kbdiag to automatically take one snapshot for me (with a clear warning that this comparison is thin), so that I don't have to know to run `perf.create_snapshot()` myself before my first useful report.
7. As a DBA who doesn't want kbdiag taking any write action on my instance, I want `--no-snapshot` to suppress the auto-snapshot behavior entirely, so that `workload` stays strictly read-only when I need it to.
8. As a DBA connected to a standby node in a repmgr cluster, I want `workload` to never attempt to write a snapshot (since standby is read-only and it would just fail), and to be told to run it from the primary instead, so that I get a clear, actionable message instead of a raw SQL error.
9. As a DBA connected to a standby node, I want to still be able to read an existing `sys_kwr`/`sys_stat_sysmetric_history` report (these are replicated, read-only-safe), so that the standby-specific restriction is scoped only to the write action, not to the whole command.
10. As a DBA scripting against kbdiag, I want `kbdiag workload --format json` to emit machine-readable output with the same verdict semantics (OK/WARN/FAIL) as every other kbdiag command, so that `workload` composes into existing automation without special-casing it.
11. As a DBA using `--exit-code`, I want the process exit code to reflect whether the report came from a clean source (OK), a degraded source (WARN), or failed outright (FAIL — bad flags or a query error), so that `workload` can gate CI/monitoring checks like other 查层 commands do.
12. As a DBA who passes `--to` without `--from`, I want a clear usage error rather than kbdiag guessing an arbitrary start point, so that I never get a report silently scoped to something I didn't ask for.
13. As a DBA who passes a reversed range (`--from` resolving to a time after `--to`), I want a clear usage error, so that I don't get a confusing empty or nonsensical diff.
14. As a DBA reading a `sys_kwr`-sourced report, I want the full original KingbaseES report text preserved verbatim (not re-parsed into fields kbdiag invented), so that I can trust it matches what KingbaseES itself would show me and that it won't silently break across KingbaseES version upgrades.
15. As a developer running kbdiag's `all` command, I want `workload`'s non-zero internal state to never abort the overall run (matching every other command's `_exit` + `EXIT_CODE_MODE` gating convention), so that one degraded data source doesn't take down an entire diagnostic sweep.

## Implementation Decisions

- New command tier placement: 查/DBA layer, alongside `obj`/`colstat`/`locks`/`perf`/`sql`/`wait`/`slow`/`bloat`/`vacuum`. Not merged into `perf` (different output contract: point-in-time snapshot vs. interval diff).
- Domain vocabulary: "Workload" is defined in `CONTEXT.md` as an interval-diff report, decoupled from which underlying KingbaseES data source produced it. Do not use "history" (collides with existing log/slow-query history) or "kwr"/"awr" (ties the command name to one specific backing implementation).
- Data-source detection order, evaluated fresh on every invocation (no caching, matching kbdiag's stateless design):
  1. `sys_kwr` extension present and installed → use `perf.kwr_report(start_id, end_id, 'text')`.
  2. `sys_kwr` available but not installed → WARN, suggest installing it; kbdiag never runs `CREATE EXTENSION` itself.
  3. `track_real_stats` GUC on → use `sys_stat_sysmetric_history`.
  4. Neither → WARN, suggest the exact `ALTER SYSTEM SET track_real_stats = on; SELECT sys_reload_conf();` to run themselves.
  This mirrors the existing degrade pattern used for `sys_stat_statements` detection.
- Node-role awareness: role is checked via the same mechanism already used elsewhere in the codebase (`pg_is_in_recovery()`/`sys_is_in_recovery()`). On a standby:
  - The auto-snapshot behavior is unconditionally suppressed (behaves as if `--no-snapshot` were always passed), with a standby-specific message pointing the operator at the primary.
  - Reads (`kwr_report`, `sys_stat_sysmetric_history`) are unaffected — these are physically replicated and safe to read on a standby.
  - The `track_real_stats` enablement suggestion, when shown on a standby, is worded to say "run this on the primary" rather than presenting a command that would itself fail on a read-only replica.
- Default time window when `--from`/`--to` are omitted: for the `sys_kwr` path, the two most recent available snapshots (resolution determined entirely by DBA-configured collection frequency, not a fixed interval); for the `sys_stat_sysmetric_history` path, a fixed most-recent 15 minutes.
- Auto-snapshot fallback: when fewer than two `sys_kwr` snapshots are available (and not suppressed by `--no-snapshot` or standby role), `workload` takes one snapshot via `perf.create_snapshot()` itself, and reports WARN (not OK) with an explanatory note that the comparison is based on a thin/just-created baseline.
- CLI surface, scoped to what's new for this command (global flags — `--format`, `--exit-code`, etc. — are unaffected and reused as-is):
  - `--from <dur>` / `--to <dur>`: relative-duration-only syntax, identical grammar to the existing `logs --since` flag (`1h`, `30m`, `1h30m`, meaning "N ago"). Reuses the same parsing approach already implemented for `logs` rather than introducing a second time-parsing convention. `--to` defaults to "now" when only `--from` is given.
  - `--no-snapshot`: suppresses the auto-snapshot fallback described above.
  - No new `--json` flag — the existing global `--format json` covers this, consistent with every other command.
  - No new `--exit-code` flag — the existing global flag is reused.
  - No `--database` flag in this iteration — `perf.kwr_report()`'s underlying `database` parameter is passed at its default (whole-instance scope). Per-database scoping is deliberately deferred, not designed here.
- Usage-error validation (mapped to the existing `_exit=2`/FAIL convention used across the codebase):
  - `--to` given without `--from` → usage error.
  - `--from`/`--to` resolve such that the start is after the end → usage error.
- Verdict semantics (reusing the existing `_exit=0/1/2` → OK/WARN/FAIL convention, gated behind `--exit-code` exactly like every other command so `all` never aborts on a WARN):
  - OK: report generated from `sys_kwr` (≥2 real snapshots) or `sys_stat_sysmetric_history`, no degraded branch taken.
  - WARN: any degraded branch — extension/GUC-not-enabled advisory, auto-snapshot fallback used, standby-forced no-snapshot fallback.
  - FAIL: flag validation failure, or an actual query error from the underlying data source (distinct from "no data available," which is WARN).
  - The verdict reflects only "did a report get produced, and from which source" — it does not attempt to interpret report contents (e.g., whether wait-event proportions look unhealthy); that interpretation is left to the human reading the report.
- Output shape:
  - `sys_kwr` path: `data.report` holds the full original KingbaseES report text (currently observed as ~900 lines of Chinese-language AWR-style text), stored verbatim, not parsed into structured sub-fields. Rationale: `ora` treats Oracle's own AWR/ASH report output the same way; KingbaseES's report format is not a stable contract to parse against and will drift across versions; the report is already a finished artifact — kbdiag's job is deciding whether/how to generate it, not re-rendering it.
  - `sys_stat_sysmetric_history` path: `data.metrics` holds an array of raw `{metric, ts, value}` rows — this source has no pre-rendered text to begin with, so the same "pass the DB's own shape through unmodified" principle produces a different shape than the `sys_kwr` path. The two paths are intentionally not unified into one `data` shape.

## Testing Decisions

Good tests here exercise `workload`'s observable CLI behavior (stdout content, exit code, JSON shape) against the two already-provisioned VM fixtures — not internal shell function units. Prior art: `test/cases/test_logs.sh`, which is the closest existing command in shape (also has a `--since`-style flag, also degrades gracefully on missing data, also has a JSON-validity assertion) and uses the `assert_contains` / `assert_exit_code` / `assert_json_valid` helpers from `test/lib/`.

Fixtures were re-verified live on 2026-07-26 (not just assumed from the requirements doc) — one correction from what was originally grilled: **`kes-node2` is a physical streaming-replication standby of `kes-node1`, so `sys_kwr`'s catalog state (installed or not) and its `perf.kwr_snapshots` data both replicate to node2 automatically.** Node2 is NOT a "neither data source available" fixture — it sees the same 2 snapshots node1 does and will take the same `sys_kwr` OK path. Only `track_real_stats` (a node-local GUC) actually differs (`on` on node1, `off` on node2), but that never matters on node2 because `sys_kwr`-installed always wins priority over the GUC fallback.

- `kes-node1` (primary): `sys_kwr` installed, two existing snapshots (`snap_id` 1/2), `track_real_stats=on` — exercises the "clean `sys_kwr` path" and "auto-snapshot not needed" branches.
- `kes-node2` (standby, reached via the existing `ssh_node2`-style helpers in `test/lib/ssh_helpers.sh`): same `sys_kwr` data as node1 (replicated), `track_real_stats=off` locally — exercises the "clean `sys_kwr` path" too, but from a standby, so it's the fixture for the "auto-snapshot forced off on standby" branch (Q5), not a data-source-degrade fixture.

**No live fixture exists for the two "data source unavailable" WARN branches** (`sys_kwr` available-but-not-installed; neither available) — `sys_kwr`, once installed on the primary, is permanently present on every standby via replication, so there is no way to construct a "not installed" node without dropping the extension on the primary and destroying the node1 OK-path fixture. Decided 2026-07-26: don't stand up a third throwaway instance for this — those two branches stay logic-only, verified by code review, not by the SSH black-box suite. Mirrors `test_logs.sh`'s existing "fixture unavailable → graceful `_pass`" convention.

Coverage to write, mirroring `test_logs.sh`'s structure (one `test_workload_*` function per behavior):

- Happy path on `kes-node1`: exit 0, `--exit-code` reflects OK, `data.report` present and non-empty.
- `--from`/`--to` accepted and narrows the window (reuse of `logs`' relative-duration parser means this is largely a regression check that `workload` wires the same parser correctly, not a re-test of the parser itself).
- `--to` without `--from` → usage error, correct message, `_exit=2`.
- Reversed `--from`/`--to` → usage error.
- `kes-node2` as standby: auto-snapshot branch does not fire even without `--no-snapshot` passed; message names the primary. Reads (the `sys_kwr` report itself) still succeed.
- `--no-snapshot` on `kes-node1`: accepted as a no-op when snapshots are already sufficient (can't force the "insufficient snapshots" state without disrupting the node1 fixture, so this only checks the flag doesn't break the happy path — the actual suppression behavior is covered by the standby test above, which exercises the same code path from the other trigger).
- The two data-source-unavailable WARN branches: graceful skip, documented as logic-only per above.
- `--format json` validity (`assert_json_valid`) on the `sys_kwr` `data.report` shape (both fixtures produce this shape; `sys_stat_sysmetric_history`'s `data.metrics` shape has no live fixture for the same reason as the WARN branches above and is graceful-skipped too).

## Out of Scope

- A kbdiag-maintained stateful metrics repository (pg_profile-style). Explicitly rejected — conflicts with kbdiag's stateless/single-file architecture.
- Rich HTML report rendering (pgBadger-style).
- Any new default-on background collection or scheduling (cron, systemd timers, or otherwise). All data collection is either something KingbaseES already does on its own, or a one-shot action `workload` takes at invocation time.
- Multi-database scoping (`--database` flag) — deferred to a future spec if needed.
- HypoPG-style what-if index advisor, a log-histogram addition to `logs`, and a JSON-unification pass on `snapshot` — these were separate items on the same capability-gap research shortlist but are unrelated to `workload` specifically and are not part of this spec.
- Absolute-timestamp support for `--from`/`--to` (only relative-duration `1h`/`30m`-style values are supported, matching `logs --since`).
- Auto-installing `sys_kwr` or auto-flipping `track_real_stats` on the user's behalf — both are schema/permission-affecting write actions `workload` only ever suggests, never performs.

## Further Notes

- During research, the `sys_kwr` extension was found to ship a family of `kddm_*` advisor functions (`kddm_index_advisor`, `kddm_guc_advisor`, `kddm_checkpoint_advisor`, `kddm_cpu_load_advisor`, `kddm_wal_*_advisor`, and others) — a built-in KingbaseES recommendation engine. This is unrelated to `workload` but is a plausible future evidence source for the existing `advisor` (断层) command, following the same "let the database do the stateful/complex work, kbdiag reads it" pattern as this spec. Its output shape hasn't been investigated yet — worth a follow-up research pass after `workload` ships, not part of this spec.
- The `docs/superpowers/specs/2026-07-25-capability-gap-research.md` document has four other shortlist items beyond `workload`; none are addressed here.
