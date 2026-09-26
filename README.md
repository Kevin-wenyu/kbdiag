# kbdiag

[English](#english) | [中文](#中文)

---

<a name="english"></a>

KingbaseES command-line diagnostics. One static binary that talks the wire protocol directly: no `ksql`, no interactive screens. Each command runs a single read-only look at a live instance and prints a verdict, the evidence, and the next command to run. Works on single nodes and on repmgr primary/standby clusters.

This is kbdiag 2.0, rewritten in Go; `v2.0.0-alpha.1` is its first release. The shell toolkit (`v1.x`) is frozen; use release [`v1.0.0`](https://github.com/Kevin-wenyu/kbdiag/releases/tag/v1.0.0) if you need it.

## Install

Build a static Linux binary (Go from `go.mod`), then copy it to the database host:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(git describe --tags --match 'v2*' --always)" -o kbdiag ./cmd/kbdiag
scp kbdiag kingbase@db-host:~/kbdiag
```

Use `GOARCH=arm64` for ARM hosts. The binary has no runtime dependencies.

## Quick start

```bash
sudo -iu kingbase        # run as the database OS user
~/kbdiag status          # what is this instance
~/kbdiag sessions        # who is connected, who is idle in transaction
~/kbdiag locks           # who waits for a lock, and who blocks them
echo $?                  # 0 OK, 1 WARN, 2 FAIL, 3 UNKNOWN
```

## Commands

| Command | What it shows | Flags |
|---|---|---|
| `status` | Version, data directory, port, role; each standby (on a primary) or the WAL upstream (on a standby); uptime, connections used / usable, database sizes, disk of the data directory (local runs only). FAIL when ordinary users can no longer connect; WARN when a standby is not receiving WAL | |
| `sessions` | Who holds the connections (counted by user, database, application, client), then the client sessions that are not idle, longest transaction first; WARN on long idle in transaction. JSON always carries every session | `--all`, `--limit N`, `--idle-in-txn-warn S` |
| `session <pid>` | One session: who and what it is, its full SQL, the lock it waits for and who blocks it, whom it blocks, what it holds | `--lock-wait-warn S`, `--idle-in-txn-warn S` |
| `locks` | Who blocks the most (and what it holds), then every lock wait, longest first, with its direct blockers; WARN on long waits | `--limit N`, `--lock-wait-warn S` |
| `txn` | Open transactions and prepared (2PC) ones; WARN/FAIL on old ones | `--limit N`, `--xact-warn S`, `--xact-fail S`, `--prepared-fail S` |
| `waits` | Sessions grouped by wait event and state | |
| `slots` | Replication slots; FAIL on inactive ones | |

Defaults: idle in transaction 300 s, lock wait 10 s, transaction 300 s (WARN) / 1800 s (FAIL), prepared transaction 900 s (FAIL). `--limit` only trims what is shown; findings always cover every row.

## Connection

| Flag | Default |
|---|---|
| `--host` | `/tmp` (local socket `/tmp/.s.KINGBASE.54321`); a host name or IP for TCP |
| `-p, --port` | `54321` |
| `-d, --dbname` | `test` |
| `-U, --user` | `system` |
| `--timeout` | `10s` per query |
| `--json` | print the report as JSON |

The password comes from `PGPASSWORD` or `~/.pgpass`. Every connection is a read-only transaction with a `lock_timeout`; kbdiag never changes anything. Suggested fixes (such as `ROLLBACK PREPARED`) are printed, never run.

## Output

```text
locks  WARN  (KingbaseES V008R006C009B0014, primary, system@local, 2026-09-26T19:41:41+08:00)

[WARN] lock.waiting  会话 803890 等 public.kbdiag_inj_lock 的 AccessShareLock 已 14 秒，被 803881 挡住
  verify: kbdiag session 803881  # 看挡路的会话在干什么

blockers: 1
  pid     blocks  holds
  803881  1       public.kbdiag_inj_lock AccessExclusiveLock

waiting: 1
  pid     object                  wants            waited  blocked by
  803890  public.kbdiag_inj_lock  AccessShareLock  14s     803881
```

- First line: command, verdict, and context (version, role, user@location, collection time).
- Findings: an id, a symptom (in Chinese), and a `verify` or `fix` next step.
- Data: laid out for reading, sizes and durations in readable units; `-` is null, `?` is hidden from this account. `--json` gives every probe's raw table (bytes, seconds) with stable field names.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | OK |
| 1 | WARN |
| 2 | FAIL |
| 3 | UNKNOWN: something could not be collected or seen, so kbdiag will not claim OK |
| 64 | Usage error |
| 69 | Cannot connect |

## Privileges

Run as `system` over the local socket for the full picture. An account without a monitoring role cannot see other sessions' state, timings or SQL; kbdiag lists those fields under `redacted` and answers UNKNOWN instead of OK, unless what it can see is already WARN or FAIL. Granting `sys_monitor` lifts this. On a standby, prepared transactions are `not_applicable` (run `txn` on the primary); this does not change the verdict.

## Requirements

- KingbaseES V8R6 (tested on V008R006C009B0014)
- Linux amd64 or arm64
- repmgr is optional

---

<a name="中文"></a>

KingbaseES 命令行诊断工具。单个静态二进制，直连线协议：不调 `ksql`，不进交互界面。每条命令对运行中的实例做一次只读查询，输出结论、证据和下一步该跑的命令。支持单机和 repmgr 主备集群。

这是用 Go 重写的 kbdiag 2.0，首个发布版本是 `v2.0.0-alpha.1`。shell 版（`v1.x`）已冻结，需要时用 [`v1.0.0`](https://github.com/Kevin-wenyu/kbdiag/releases/tag/v1.0.0)。

## 安装

编译 Linux 静态二进制（Go 版本见 `go.mod`），拷到数据库主机：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(git describe --tags --match 'v2*' --always)" -o kbdiag ./cmd/kbdiag
scp kbdiag kingbase@db-host:~/kbdiag
```

ARM 主机用 `GOARCH=arm64`。二进制没有运行时依赖。

## 快速开始

```bash
sudo -iu kingbase        # 以数据库 OS 用户运行
~/kbdiag status          # 这是个什么实例
~/kbdiag sessions        # 谁连着，谁 idle in transaction
~/kbdiag locks           # 谁在等锁，被谁挡住
echo $?                  # 0 OK，1 WARN，2 FAIL，3 UNKNOWN
```

## 命令

| 命令 | 看什么 | 参数 |
|---|---|---|
| `status` | 版本、数据目录、端口、角色；主库列出每个备库，备库看上游在不在收 WAL；运行时长、连接数已用/可用、各库大小、数据目录所在磁盘（只在本机运行时有）。普通用户已经连不上报 FAIL，备库没在收 WAL 报 WARN | |
| `sessions` | 连接是谁占的（按用户、库、应用、客户端计数），再列出不是 idle 的客户端会话，事务最长的在前；idle in transaction 过久报 WARN。JSON 总是全部会话 | `--all`、`--limit N`、`--idle-in-txn-warn 秒` |
| `session <pid>` | 一个会话：是谁、在干什么、完整 SQL，在等什么锁、被谁挡住，挡住了谁，持有哪些锁 | `--lock-wait-warn 秒`、`--idle-in-txn-warn 秒` |
| `locks` | 谁挡的人最多、它持有什么锁，再列出每个等锁的会话（等得最久的在前）和直接挡路者；等太久报 WARN | `--limit N`、`--lock-wait-warn 秒` |
| `txn` | 开着的事务和两阶段事务；过久报 WARN/FAIL | `--limit N`、`--xact-warn 秒`、`--xact-fail 秒`、`--prepared-fail 秒` |
| `waits` | 按等待事件和状态汇总会话 | |
| `slots` | 复制槽；未激活报 FAIL | |

默认阈值：idle in transaction 300 秒，等锁 10 秒，事务 300 秒 WARN、1800 秒 FAIL，两阶段事务 900 秒 FAIL。`--limit` 只影响显示，判定始终覆盖全部行。

## 连接

| 参数 | 默认值 |
|---|---|
| `--host` | `/tmp`（本地 socket `/tmp/.s.KINGBASE.54321`）；写主机名或 IP 走 TCP |
| `-p, --port` | `54321` |
| `-d, --dbname` | `test` |
| `-U, --user` | `system` |
| `--timeout` | 每条查询 `10s` |
| `--json` | 输出 JSON |

密码从 `PGPASSWORD` 或 `~/.pgpass` 取。连接是只读事务并设了 `lock_timeout`，kbdiag 不改任何东西；建议的处理语句（如 `ROLLBACK PREPARED`）只打印，不执行。

## 输出

```text
locks  WARN  (KingbaseES V008R006C009B0014, primary, system@local, 2026-09-26T19:41:41+08:00)

[WARN] lock.waiting  会话 803890 等 public.kbdiag_inj_lock 的 AccessShareLock 已 14 秒，被 803881 挡住
  verify: kbdiag session 803881  # 看挡路的会话在干什么

blockers: 1
  pid     blocks  holds
  803881  1       public.kbdiag_inj_lock AccessExclusiveLock

waiting: 1
  pid     object                  wants            waited  blocked by
  803890  public.kbdiag_inj_lock  AccessShareLock  14s     803881
```

- 第一行：命令、结论和上下文（版本、角色、用户@位置、采集时间）。
- finding：编号、症状、下一步（`verify` 看什么或 `fix` 怎么处理）。
- 数据：按阅读排版，大小和时长换成易读单位；`-` 表示空值，`?` 表示当前账号看不到。`--json` 给每个探针的原始表（字节、秒），字段名稳定。

## 退出码

| 退出码 | 含义 |
|---|---|
| 0 | OK |
| 1 | WARN |
| 2 | FAIL |
| 3 | UNKNOWN：有东西没采到或看不到，所以不说 OK |
| 64 | 参数错误 |
| 69 | 连不上数据库 |

## 权限

以 `system` 走本地 socket 能看到全部信息。没有监控角色的账号看不到别人会话的状态、时间和 SQL；kbdiag 把这些字段列进 `redacted`，结论给 UNKNOWN 而不是 OK；看得到的部分已经是 WARN 或 FAIL 时照报 WARN 或 FAIL。授予 `sys_monitor` 后解除。备库上两阶段事务是 `not_applicable`（到主库跑 `txn`），不影响结论。

## 运行要求

- KingbaseES V8R6（在 V008R006C009B0014 上测试）
- Linux amd64 或 arm64
- repmgr 可选
