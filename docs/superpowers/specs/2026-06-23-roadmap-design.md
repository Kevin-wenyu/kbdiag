# kbdiag Roadmap Design — Platform → Quality → Feature

**Date:** 2026-06-23  
**Status:** Approved  
**Approach:** C — Platform first, then Quality, then Feature

---

## Background

kbdiag is a 28-command KingbaseES CLI diagnostic tool written in pure Shell. All planned features from the v2/v3/v1.4 design docs are implemented. This roadmap addresses the three remaining gaps identified in a completeness audit:

1. No safety net (no CI, no git guards, no pre-commit linting)
2. Uneven test coverage (2–28 tests/command; 根因层 is smoke-tested only)
3. Two thin commands (watch, stat) that haven't reached their design potential

**Constraint accepted:** GitHub Actions CI cannot run integration tests (Lima VM is local). CI scope = shellcheck + build verification only. Integration tests remain local-manual.

---

## Phase 1: Platform Layer

### 1a. git-guardrails (Claude Code hook)

Install the `git-guardrails-claude-code` skill to block destructive git operations inside Claude Code sessions:

- Blocked: `git push --force`, `git reset --hard`, `git clean -f`, `git branch -D`
- Scope: project-level `.claude/settings.json`
- Why: direct push to main with no branch protection; one wrong command loses history

### 1b. GitHub Actions CI

File: `.github/workflows/ci.yml`

Triggers: push and pull_request to main

Jobs:
1. **shellcheck** — run `shellcheck lib/*.sh` on ubuntu-latest
2. **build** — run `bash build.sh` and verify `dist/kbdiag` exists and is executable
3. **smoke** — run `dist/kbdiag --version` and `dist/kbdiag --help` (no DB required)

Not in scope: integration tests (require local Lima VM).

### 1c. shellcheck pre-commit hook

File: `.git/hooks/pre-commit` (local, not committed)  
Install script: `scripts/setup-dev.sh` (committed — lets teammates reproduce)

Behavior: on every `git commit`, run `shellcheck lib/*.sh`. Abort commit if any errors. Warnings allowed (SC2034 etc. may be suppressed with inline annotations where justified).

---

## Phase 2: Quality Layer

### 2a. diagnose — per-item assertion tests

Current state: 9 smoke tests for 16 `_diag_*` sub-checks.  
Target: each sub-check has at least one test that asserts on output structure, not just exit code.

Priority sub-checks to cover:
- `_diag_long_txn` — assert finding or clean label present
- `_diag_connections` — assert connection count number present
- `_diag_locks` — assert lock count present
- `_diag_replication` — assert lag value or standalone label
- `_diag_disk` — assert disk usage percentage present
- `_diag_xid_age` — assert age number present
- `_diag_vacuum_debt` — assert dead tuple count present
- `_diag_bloat` — assert bloat percentage or clean
- `_diag_buffer_hit` — assert hit ratio percentage present
- `_diag_checkpoint` — assert checkpoint interval present
- `_diag_temp` — assert temp size value present
- `_diag_params` — assert at least one param finding present
- `_diag_indexes` — assert finding or clean
- `_diag_slow_queries` — assert finding or "no slow queries"
- `_diag_stmt_top` — assert skip message when extension absent

Test file: `test/cases/test_diagnose.sh` (extend existing)

### 2b. advisor --fix SQL validation

Current state: 3 tests check SQL keyword presence only.  
Target: validate SQL is syntactically executable (pipe through `ksql -c` in dry-run / EXPLAIN mode where possible, or at minimum validate with `echo "SQL" | ksql --single-transaction` check).

Add tests:
- `test_advisor_fix_index_sql_executable` — verify generated CREATE INDEX SQL is parseable
- `test_advisor_fix_vacuum_sql_parseable`
- `test_advisor_fix_analyze_sql_parseable`

### 2c. Thin test suites — minimum viable coverage

| Command | Current | Target | Key new tests |
|---------|---------|--------|--------------|
| sessions | 3 | 8 | state filter, long query detection, idle-in-txn count |
| replication | 4 | 8 | primary shows standby count, standby shows lag, verbose columns |
| watch | 4 | 6 | watch locks runs, watch wait runs |
| remote | 4 | 6 | remote --all syntax, remote single-node syntax |
| update | 2 | 4 | offline error message, version output format |

### 2d. /tmp → scratchpad fix

`lib/cmd_logs.sh:47` uses `/tmp` directly. Change to `${TMPDIR:-/tmp}` to respect environment.

---

## Phase 3: Feature Layer

### 3a. watch — multi-subcommand

Current state: `watch` only wraps `status` (20 lines, 5s interval).  
Design: extend to support `watch [status|sessions|locks|wait]` with clear-screen refresh.

Subcommands:
- `watch status` (current behavior, default)
- `watch sessions` — live refresh of active sessions
- `watch locks` — live refresh of lock waits
- `watch wait` — live refresh of wait event summary

Interface: `kbdiag watch [subcommand] [interval]`  
Default interval: 5s. Clear-screen between refreshes (already implemented for status).

### 3b. stat — AWR expansion

Current state: `stat` captures two snapshots and computes delta for ~6 metrics (40 lines).  
Design: expand to cover the standard AWR dimensions a DBA expects.

Add metrics:
- Top SQL by total time (if sys_stat_statements available)
- Wait event delta (events that spiked during the interval)
- Lock acquisition count delta
- Temp file generation delta
- Checkpoint frequency

Output: one structured report per section, same header style as other commands.

---

## Skill Usage Map

| Phase | Task | Skill |
|-------|------|-------|
| 1a | git guards | `git-guardrails-claude-code` |
| 1b–1c | CI + pre-commit | `setup-pre-commit` (pattern reference), manual shell |
| 2–3 | Implementation | `implement` |

---

## Success Criteria

- Phase 1: `shellcheck lib/*.sh` passes clean; CI badge green on main; git force-push blocked in Claude Code
- Phase 2: all commands have ≥ 6 tests; diagnose has per-item assertions; 0 shellcheck errors in lib/
- Phase 3: `watch` supports 4 subcommands; `stat` covers 5 AWR dimensions; all new code passes CI
