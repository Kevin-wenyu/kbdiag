# kbdiag

KingbaseES command-line DBA toolkit. Non-interactive — runs directly against a live instance and outputs status, topology, replication lag, performance bottlenecks, index health, column statistics, and optimization advice. Supports single-node and HA clusters (repmgr).

## Install

Single-file, no dependencies:

```bash
curl -fsSL https://raw.githubusercontent.com/Kevin-wenyu/kbdiag/main/dist/kbdiag \
  -o /tmp/kbdiag && chmod +x /tmp/kbdiag
```

## Quick Start

```bash
# Switch to the kingbase OS user first
sudo -i -u kingbase

# Full scan (daily health check)
/tmp/kbdiag all

# Health gate (suitable for monitoring scripts)
/tmp/kbdiag check
echo $?   # 0=all OK  1=WARN  2=FAIL

# Verbose DBA view
/tmp/kbdiag check -v
/tmp/kbdiag perf slow -v
```

## Global Flags

```
kbdiag [global-flags] <command> [subcommand] [command-flags]

  -v, --verbose       Show full underlying data
  -q, --quiet         Only show WARN/FAIL (suppress OK/INFO)
  -n N, --top N       Limit result rows (default: 10)
  --format text|json  Output format (default: text)
  --no-color          Disable ANSI colors
  --timeout N         DB query timeout in seconds (default: 10)
```

## Command Reference

### [OPS] Operations Commands (no DBA context required)

| Command | Description |
|---------|-------------|
| `status` | Process, connectivity, role, uptime (includes license summary) |
| `license` | License validity, expiry date, type (trial/commercial), serial and owner |
| `cluster` | Repmgr cluster topology (skipped automatically on standalone) |
| `replication` | Primary: standby connections; standby: WAL lag |
| `check` | 14-item health threshold check — exit code 0=OK / 1=WARN / 2=FAIL |
| `space [frag]` | Disk usage, table sizes, WAL, archive; `frag` shows table fragmentation |
| `diagnose [--full]` | Root-cause diagnostic report (fast <15s; `--full` ~90s) |

### [DBA] Deep-Dive Commands (require DBA interpretation)

| Command | Description |
|---------|-------------|
| `sessions` | Non-idle session list |
| `locks [hold\|wait\|deadlock]` | Lock analysis: holders / waiters / deadlock detection |
| `perf [slow\|bloat\|vacuum\|wait\|io\|wal\|top]` | Slow queries / table bloat / vacuum status / wait events / IO / WAL / top SQL |
| `sql [pid\|all]` | SQL text + EXPLAIN for a session |
| `stmt [queryid]` | SQL history statistics — Top N by mean/total/IO/calls (AWR-style); drill down with queryid |
| `wait` | Wait event distribution |
| `progress` | Long-running operation progress (VACUUM, CREATE INDEX, etc.) |
| `params [pattern]` | Instance parameters with optional pattern filter |
| `stat` | Throughput metrics (TPS, buffer hit rate) with delta sampling |
| `obj <schema.table>` | Object deep-dive: size, indexes, constraints |
| `colstat <schema.table> [--col <col>]` | Column statistics deep-dive (n_distinct, MCV, correlation) |
| `temp` | Temp file and sort spill analysis |
| `watch <N> <cmd>` | Repeat any kbdiag command every N seconds |
| `conf [diff]` | Configuration audit; `diff` compares nodes |
| `audit` | Security and compliance checks |
| `logs` | Log file analysis (slow queries, errors) |
| `remote <nodes> <cmd>` | Multi-node batch diagnostics |
| `kill [--terminate] [pid\|--long N\|--idle-txn N] [--dry-run] [--force]` | Cancel or terminate queries / sessions |
| `idx [unused\|dup\|bloat\|missing]` | Index health analysis |
| `advisor [index\|vacuum\|params\|analyze] [--fix]` | Consolidated DBA recommendations; `--fix` emits executable SQL |

### [ALL]

| Command | Description |
|---------|-------------|
| `all` | Run status / cluster / replication / sessions / locks / check / perf / space |

## Environment Variables

### Connection

| Variable | Default | Description |
|----------|---------|-------------|
| `KB_PORT` | `54321` | Database port |
| `KB_BIN_DIR` | `/home/kingbase/cluster/install/kingbase/bin` | Binary directory |
| `KB_DATA_DIR` | `/home/kingbase/cluster/install/kingbase/data` | Data directory |
| `KB_REPMGR_CONF` | `/home/kingbase/cluster/install/kingbase/etc/repmgr.conf` | Repmgr config file |
| `KB_SUPERUSER` | `system` | Database superuser |
| `KB_DB` | `test` | Database name |

### Thresholds

| Variable | Default | Description |
|----------|---------|-------------|
| `KB_WARN_CONN` / `KB_FAIL_CONN` | `70` / `90` | Connection usage % |
| `KB_WARN_LAG` / `KB_FAIL_LAG` | `30` / `300` | Replication lag (seconds) |
| `KB_WARN_TXN` / `KB_FAIL_TXN` | `300` / `1800` | Long transaction threshold (seconds) |
| `KB_WARN_OLDEST_TXN` / `KB_FAIL_OLDEST_TXN` | `3600` / `14400` | Oldest active transaction (seconds) |
| `KB_WARN_XID` / `KB_FAIL_XID` | `1000000000` / `1900000000` | XID age (wraparound protection) |
| `KB_WARN_DEAD_TUPLES` | `100000` | Dead tuples per table |
| `KB_SLOW_THRESHOLD` | `5` | Slow query threshold (seconds) |
| `KB_WARN_HIT` / `KB_FAIL_HIT` | `95` / `90` | Buffer hit rate % lower bound |
| `KB_WARN_CKPT` / `KB_FAIL_CKPT` | `1` / `1` | Checkpoint warning threshold |
| `KB_WARN_TEMP` / `KB_FAIL_TEMP` | `104857600` / `1073741824` | Temp file size in bytes (100MB / 1GB) |
| `KB_WARN_SLOT` / `KB_FAIL_SLOT` | `104857600` / `1073741824` | Replication slot lag in bytes (100MB / 1GB) |
| `KB_WARN_BGW` / `KB_FAIL_BGW` | `30` / `60` | Bgwriter checkpoint threshold |

## Requirements

- Run as the `kingbase` OS user (`sudo -i -u kingbase`)
- KingbaseES V8R6+
- repmgr optional (cluster commands are skipped automatically when absent)
