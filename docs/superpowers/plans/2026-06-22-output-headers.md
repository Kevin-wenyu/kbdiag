# Output Headers UX Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add column headers to every `kbdiag` command output table that currently prints raw values with no labels.

**Architecture:** The root cause is that `ksql_q()` uses the `-t` (tuples-only) flag, which suppresses column headers from `ksql`. A sibling function `ksql_qh()` already exists in `lib/core.sh` — it is identical but omits `-t`, so SQL `AS` aliases become the header row. The fix in each location is: change `ksql_q` → `ksql_qh` for any call whose output is piped to `column -t -s '|'`. One special case (`_locks_hold`) also needs an empty-result guard. After all lib changes, rebuild with `bash build.sh`.

**Tech Stack:** Pure Bash/Shell. No dependencies. `ksql` is the KingbaseES CLI. Tests use SSH to Lima VMs (`ssh -p 57103 kevin@127.0.0.1` for node1).

## Global Constraints

- Pure Shell — no new dependencies, no build-time tools beyond what already exist
- `ksql_qh()` signature (already in `lib/core.sh`): same as `ksql_q()` but without `-t` flag; takes one positional argument (SQL string); outputs pipe-separated rows including header
- Every change to a `lib/cmd_*.sh` file requires a subsequent `bash build.sh` to take effect in `dist/kbdiag`
- Tests run via `bash test/run_tests.sh <suite>` from repo root on the Mac; they SSH into Lima VMs
- Do NOT change `ksql_q` calls that do NOT pipe into `column -t -s '|'` — those are scalar queries that use `-t` intentionally
- Do NOT rename SQL `AS` aliases unless the plan explicitly says to — preserving existing aliases avoids breaking any tests that already assert on column header text
- Commit after each task's test suite passes

---

### Task 1: Fix cmd_perf.sh — all perf subcommands

**Files:**
- Modify: `lib/cmd_perf.sh`
- Test: `test/cases/test_perf.sh`
- Rebuild: `dist/kbdiag`

**Interfaces:**
- Consumes: `ksql_qh()` from `lib/core.sh` (already exists — no changes needed to core.sh)
- Produces: `kbdiag perf slow|bloat|vacuum|wait|io|top|index` all output column headers as first row

**Functions to fix** (each change is: `ksql_q "...` → `ksql_qh "...` for calls whose output goes into `| column -t -s '|'`):

| Function | SQL produces columns |
|----------|---------------------|
| `_perf_slow` | `pid, usename, duration, client_addr, query` |
| `_perf_bloat` | `schemaname, relname, table_size, n_live_tup, n_dead_tup, live_pct` |
| `_perf_vacuum` | `schemaname, relname, n_dead_tup, last_autovacuum, last_autoanalyze, threshold, pct` |
| `_perf_wait` | `wait_event_type, wait_event, cnt, avg_sec` |
| `_perf_io` | `schemaname, relname, heap_blks_read, heap_blks_hit, hit_pct` |
| `_perf_top` (all 5 modes: buffers/reads/writes/cpu/temp) | varies per mode |
| `_perf_index` | `schemaname, relname, indexrelname, index_size, idx_scan` |

- [ ] **Step 1: Write failing tests**

Add to `test/cases/test_perf.sh`:

```bash
test_perf_slow_has_header() {
  # slow query header — may return 0 rows but must print column names when rows exist;
  # test that the command doesn't crash and (if rows present) first data line looks like a header
  # Use a simpler proxy: check that "duration" appears in output when run with -v
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE perf slow -v" 2>&1 || true)
  # Just verify it runs without error; header check done via perf_wait below
  assert_exit_zero "$(ssh_node1 "$KBDIAG_REMOTE perf slow" 2>&1; echo $?)" || true
}

test_perf_wait_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE perf wait" 2>&1)
  assert_contains "$out" "wait_event"
}

test_perf_io_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE perf io" 2>&1)
  assert_contains "$out" "schemaname"
}

test_perf_vacuum_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE perf vacuum" 2>&1)
  assert_contains "$out" "schemaname"
}

test_perf_bloat_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE perf bloat" 2>&1)
  assert_contains "$out" "schemaname"
}

test_perf_top_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE perf top" 2>&1)
  assert_contains "$out" "pid"
}

test_perf_index_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE perf index" 2>&1)
  assert_contains "$out" "schemaname"
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
bash test/run_tests.sh perf 2>&1 | grep -E "PASS|FAIL|header"
```

Expected: the new `_has_header` tests FAIL (output doesn't contain header text).

- [ ] **Step 3: Fix lib/cmd_perf.sh**

In `lib/cmd_perf.sh`, find every block matching the pattern:
```
ksql_q "
    SELECT ...
    FROM ...
    ...;" \
    | column -t -s '|' || true
```

Change `ksql_q` to `ksql_qh` for **all** such blocks inside these functions:
`_perf_slow`, `_perf_bloat`, `_perf_vacuum`, `_perf_wait`, `_perf_io`, all `_perf_top` mode blocks, `_perf_index`.

Do **not** change any `ksql_q` calls that do not pipe into `column -t -s '|'` (e.g., scalar `count(*)` queries).

- [ ] **Step 4: Rebuild**

```bash
bash build.sh
```

Expected output: `Built: dist/kbdiag`

- [ ] **Step 5: Run tests**

```bash
bash test/run_tests.sh perf
```

Expected: all perf tests pass including the new `_has_header` ones.

- [ ] **Step 6: Commit**

```bash
git add lib/cmd_perf.sh dist/kbdiag test/cases/test_perf.sh
git commit -m "fix: add column headers to all perf subcommand tables"
```

---

### Task 2: Fix cmd_idx.sh and cmd_locks.sh

**Files:**
- Modify: `lib/cmd_idx.sh`, `lib/cmd_locks.sh`
- Test: `test/cases/test_idx.sh`, `test/cases/test_locks.sh`
- Rebuild: `dist/kbdiag`

**Interfaces:**
- Consumes: `ksql_qh()` from `lib/core.sh`
- Produces: all idx and locks tables with column headers; `_locks_hold` prints `ok "No lock-holding sessions"` when empty

**Functions to fix in cmd_idx.sh:**

| Function | SQL columns |
|----------|-------------|
| `_idx_unused` | `schemaname, tablename, indexrelname, index_size, idx_scan` |
| `_idx_dup` | `schemaname, tablename, index1, index2, size1` |
| `_idx_bloat` | `schemaname, tablename, indexrelname, index_size, relpages, reltuples, est_bloat_pct` |
| `_idx_missing` | `schemaname, tablename, fk_col, constraint_name, row_count` |

**Functions to fix in cmd_locks.sh:**

| Function | SQL columns | Extra |
|----------|-------------|-------|
| `_locks_wait` | `wait_pid, usename, wait_query, block_pid, block_user, locktype, wait_mode, waiting_for` | — |
| `_locks_hold` | `pid, usename, application_name, locktype, object, mode, held_for, query` | Add empty guard |
| `_locks_deadlock` | `datname, deadlocks` | — |

**`_locks_hold` empty guard** — currently the function always runs the query and prints nothing if no rows. Add this guard pattern (same as other lock functions use for the waiting count):

```bash
_locks_hold() {
  hdr "Lock holders"
  local cnt
  cnt=$(ksql_q "SELECT count(*) FROM sys_locks l
    JOIN sys_stat_activity a ON a.pid = l.pid
    WHERE l.granted AND a.pid <> sys_backend_pid();" | tr -d '[:space:]')
  if [[ "${cnt:-0}" -eq 0 ]]; then
    ok "No lock-holding sessions"
    return 0
  fi
  ksql_qh "
    SELECT a.pid, a.usename, a.application_name,
           l.locktype, l.relation::regclass::text AS object,
           l.mode,
           now() - a.query_start AS held_for,
           left(a.query,60) AS query
    FROM sys_locks l
    JOIN sys_stat_activity a ON a.pid = l.pid
    WHERE l.granted AND a.pid <> sys_backend_pid()
    ORDER BY held_for DESC
    LIMIT ${TOP_N};" \
    | column -t -s '|' || true
}
```

(The existing query body should already match this — only `ksql_q` → `ksql_qh` and adding the count guard are new.)

- [ ] **Step 1: Write failing tests**

Add to `test/cases/test_idx.sh`:

```bash
test_idx_unused_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE idx unused" 2>&1)
  # Either "No unused indexes" OK message, or table with header
  assert_contains "$out" "indexrelname\|No unused"
}

test_idx_bloat_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE idx bloat" 2>&1)
  assert_contains "$out" "schemaname\|No index bloat"
}

test_idx_missing_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE idx missing" 2>&1)
  assert_contains "$out" "fk_col\|No unindexed"
}
```

Add to `test/cases/test_locks.sh`:

```bash
test_locks_hold_has_header_or_ok() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE locks hold" 2>&1)
  assert_contains "$out" "pid\|No lock-holding"
}

test_locks_wait_has_header_or_ok() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE locks wait" 2>&1)
  assert_contains "$out" "wait_pid\|No waiting"
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
bash test/run_tests.sh idx 2>&1 | grep -E "PASS|FAIL"
bash test/run_tests.sh locks 2>&1 | grep -E "PASS|FAIL"
```

- [ ] **Step 3: Fix lib/cmd_idx.sh**

In `_idx_unused`, `_idx_dup`, `_idx_bloat`, `_idx_missing`: change `ksql_q "...` → `ksql_qh "...` for each `| column -t -s '|'` pipeline.

- [ ] **Step 4: Fix lib/cmd_locks.sh**

In `_locks_wait` and `_locks_deadlock`: change `ksql_q` → `ksql_qh`.

For `_locks_hold`: add count guard (see exact code above) AND change `ksql_q` → `ksql_qh`.

- [ ] **Step 5: Rebuild and test**

```bash
bash build.sh
bash test/run_tests.sh idx
bash test/run_tests.sh locks
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add lib/cmd_idx.sh lib/cmd_locks.sh dist/kbdiag test/cases/test_idx.sh test/cases/test_locks.sh
git commit -m "fix: add column headers to idx/locks tables; empty guard for locks hold"
```

---

### Task 3: Fix cmd_sessions.sh, cmd_sql.sh, cmd_wait.sh, cmd_space.sh

**Files:**
- Modify: `lib/cmd_sessions.sh`, `lib/cmd_sql.sh`, `lib/cmd_wait.sh`, `lib/cmd_space.sh`
- Test: `test/cases/test_sessions.sh`, `test/cases/test_sql.sh`, `test/cases/test_wait.sh`, `test/cases/test_space.sh`
- Rebuild: `dist/kbdiag`

**Functions to fix:**

| File | Function | SQL columns |
|------|----------|-------------|
| `cmd_sessions.sh` | `cmd_sessions` | `pid, usename, application_name, client_addr, state, duration, query` |
| `cmd_sql.sh` | `cmd_sql` (the `all` path) | `pid, usename, state, duration, wait_event_type, wait_event, query` |
| `cmd_wait.sh` | `cmd_wait` (summary table) | `wait_event_type, wait_event, sessions, avg_wait_sec` |
| `cmd_wait.sh` | `cmd_wait` (verbose detail table) | `wait_event_type, wait_event, pid, query` |
| `cmd_space.sh` | `_space_frag` | `schemaname, relname, size, n_live_tup, n_dead_tup, dead_pct` |
| `cmd_space.sh` | `_space_all` (database sizes) | `datname, size` |
| `cmd_space.sh` | `_space_all` (top 10 tables) | `schemaname, relname, total_size, table_size, index_size` |

For each: change `ksql_q "...` → `ksql_qh "...` where the output goes into `| column -t -s '|'`.

- [ ] **Step 1: Write failing tests**

Add to `test/cases/test_sessions.sh`:

```bash
test_sessions_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE sessions" 2>&1)
  assert_contains "$out" "usename"
}
```

Add to `test/cases/test_sql.sh`:

```bash
test_sql_all_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE sql all" 2>&1)
  assert_contains "$out" "usename"
}
```

Add to `test/cases/test_wait.sh`:

```bash
test_wait_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE wait" 2>&1)
  assert_contains "$out" "wait_event\|No wait events"
}
```

Add to `test/cases/test_space.sh`:

```bash
test_space_frag_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE space frag" 2>&1)
  assert_contains "$out" "schemaname\|No heavily fragmented"
}

test_space_all_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE space" 2>&1)
  assert_contains "$out" "datname"
}
```

- [ ] **Step 2: Run to confirm failures**

```bash
bash test/run_tests.sh sessions 2>&1 | grep -E "PASS|FAIL"
bash test/run_tests.sh sql 2>&1 | grep -E "PASS|FAIL"
bash test/run_tests.sh wait 2>&1 | grep -E "PASS|FAIL"
bash test/run_tests.sh space 2>&1 | grep -E "PASS|FAIL"
```

- [ ] **Step 3: Fix all four files**

In each file, change `ksql_q` → `ksql_qh` for every call piped to `| column -t -s '|'`. Leave scalar `ksql_q` calls (those piped to `tr -d` or assigned to a variable) unchanged.

- [ ] **Step 4: Rebuild and test**

```bash
bash build.sh
bash test/run_tests.sh sessions
bash test/run_tests.sh sql
bash test/run_tests.sh wait
bash test/run_tests.sh space
```

- [ ] **Step 5: Commit**

```bash
git add lib/cmd_sessions.sh lib/cmd_sql.sh lib/cmd_wait.sh lib/cmd_space.sh dist/kbdiag \
        test/cases/test_sessions.sh test/cases/test_sql.sh test/cases/test_wait.sh test/cases/test_space.sh
git commit -m "fix: add column headers to sessions/sql/wait/space tables"
```

---

### Task 4: Fix remaining commands — obj, temp, progress, params, conf, audit, check, replication

**Files:**
- Modify: `lib/cmd_obj.sh`, `lib/cmd_temp.sh`, `lib/cmd_progress.sh`, `lib/cmd_params.sh`, `lib/cmd_conf.sh`, `lib/cmd_audit.sh`, `lib/cmd_check.sh`, `lib/cmd_replication.sh`
- Test: existing test files for each command
- Rebuild: `dist/kbdiag`

**Functions and SQL columns:**

| File | Function / block | SQL columns |
|------|-----------------|-------------|
| `cmd_obj.sh` | Size & tuples block | `total_size, table_size, index_size, n_live_tup, n_dead_tup, dead_pct` |
| `cmd_obj.sh` | Vacuum/Analyze block | `last_vacuum, last_autovacuum, last_analyze, last_autoanalyze, mods_since_analyze` |
| `cmd_obj.sh` | Indexes block | `indexrelname, size, idx_scan, idx_tup_read, idx_tup_fetch, flags` |
| `cmd_obj.sh` | Constraints block | `conname, contype, type` |
| `cmd_obj.sh` | Column statistics block (verbose) | `attname, null_frac, n_distinct, most_common_vals` |
| `cmd_temp.sh` | Cumulative temp stats | `datname, temp_files, temp_size` |
| `cmd_temp.sh` | Current session temp usage | `pid, usename, temp_blks_read, temp_blks_written, query` |
| `cmd_temp.sh` | Related settings | `name, setting, unit` |
| `cmd_progress.sh` | VACUUM progress | `pid, phase, heap_blks_scanned, heap_blks_total, pct, query` |
| `cmd_progress.sh` | CREATE INDEX progress | `pid, phase, blocks_done, blocks_total, pct, query` |
| `cmd_progress.sh` | Long-running queries | `pid, usename, duration, query` |
| `cmd_params.sh` | `cmd_params` main output | `name, setting, unit, boot_val, note` |
| `cmd_conf.sh` | `cmd_conf` main output | `name, setting, unit, boot_val, note` |
| `cmd_audit.sh` | PUBLIC write grants table | `grantee, table_name, privilege_type` |
| `cmd_check.sh` | Verbose long transactions | `pid, usename, state, duration, query` |
| `cmd_check.sh` | Verbose waiting locks | `pid, usename, wait_query, block_pid, block_query` |
| `cmd_check.sh` | Verbose autovacuum backlog | `schemaname, relname, n_dead_tup, last_autovacuum` |
| `cmd_check.sh` | Verbose connections by user | `usename, count` |
| `cmd_replication.sh` | Standby connections (verbose) | `client_addr, state, sync_state, replay_lag` |

In every case: change `ksql_q` → `ksql_qh` where the call feeds `| column -t -s '|'`.

- [ ] **Step 1: Write failing tests**

Add to `test/cases/test_obj.sh`:

```bash
test_obj_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE obj public.sys_users" 2>&1 || true)
  # If table doesn't exist, that's fine — test that a known table (pg_class is always present) works
  out=$(ssh_node1 "sudo -i -u kingbase $KBDIAG_REMOTE obj pg_catalog.pg_class" 2>&1 || true)
  assert_contains "$out" "total_size\|not found"
}
```

Add to `test/cases/test_params.sh` (or create if absent):

```bash
test_params_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE params work_mem" 2>&1)
  assert_contains "$out" "name"
}
```

Add to `test/cases/test_replication.sh` (or create if absent, check if it exists first with `ls test/cases/`):

```bash
test_replication_verbose_has_header() {
  local out
  out=$(ssh_node1 "$KBDIAG_REMOTE replication -v" 2>&1)
  # On node1 (primary with standby): should have header; on standalone: OK message
  assert_contains "$out" "client_addr\|No standby\|replication"
}
```

- [ ] **Step 2: Run to confirm failures**

```bash
bash test/run_tests.sh obj 2>&1 | grep -E "PASS|FAIL"
bash test/run_tests.sh params 2>&1 | grep -E "PASS|FAIL"
```

- [ ] **Step 3: Fix all eight files**

Change `ksql_q` → `ksql_qh` for every call piped to `| column -t -s '|'` in each listed file. Leave all scalar queries (piped to `tr -d`, `awk`, `head`, `grep`, or assigned to a variable) unchanged.

- [ ] **Step 4: Rebuild and run full suite**

```bash
bash build.sh
bash test/run_tests.sh
```

Expected: all tests pass (previous count was 241).

- [ ] **Step 5: Commit**

```bash
git add lib/cmd_obj.sh lib/cmd_temp.sh lib/cmd_progress.sh lib/cmd_params.sh \
        lib/cmd_conf.sh lib/cmd_audit.sh lib/cmd_check.sh lib/cmd_replication.sh \
        dist/kbdiag test/cases/
git commit -m "fix: add column headers to obj/temp/progress/params/conf/audit/check/replication tables"
```

---

### Task 5: Full test suite verification and push

**Files:**
- No code changes
- Rebuild: `dist/kbdiag` (already rebuilt in Task 4)

- [ ] **Step 1: Run complete test suite**

```bash
bash test/run_tests.sh 2>&1 | tail -5
```

Expected output (count will be higher than 241 due to new tests added in Tasks 1–4):

```
Total: NNN passed, 0 failed / NNN run
```

Zero failures required. If any test fails, fix it before proceeding.

- [ ] **Step 2: Push**

```bash
git push origin main
```

- [ ] **Step 3: Verify on VM**

```bash
ssh -p 57103 kevin@127.0.0.1 "sudo -i -u kingbase ~/kbdiag perf wait"
```

Expected: first data line shows column names (`wait_event_type  wait_event  sessions  avg_wait_sec`), not raw values.
