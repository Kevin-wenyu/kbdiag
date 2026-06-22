# kbdiag

[English](#english) | [中文](#中文)

---

<a name="english"></a>

KingbaseES command-line DBA toolkit. Non-interactive — runs directly against a live instance and outputs status, topology, replication lag, performance bottlenecks, index health, column statistics, and optimization advice. Supports single-node and HA clusters (repmgr).

## Install

Single-file, no dependencies:

```bash
curl -fsSL https://raw.githubusercontent.com/Kevin-wenyu/kbdiag/main/dist/kbdiag \
  -o ~/kbdiag && chmod +x ~/kbdiag
```

## Update

```bash
~/kbdiag update
```

## Team Deployment

Push kbdiag to multiple hosts at once:

```bash
# Create a hosts file (one user@host per line)
cp scripts/hosts.example my-hosts.txt
vim my-hosts.txt

# Deploy
bash scripts/deploy.sh my-hosts.txt

# Custom SSH port or destination
SSH_PORT=2222 DB_USER=kingbase bash scripts/deploy.sh my-hosts.txt
```

## Quick Start

```bash
# Switch to the kingbase OS user first
sudo -i -u kingbase

# Full scan (daily health check)
~/kbdiag all

# Health gate (suitable for monitoring scripts)
~/kbdiag check
echo $?   # 0=all OK  1=WARN  2=FAIL

# Verbose DBA view
~/kbdiag check -v
~/kbdiag perf slow -v
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

### [OPS] Operations Commands

| Command | Description |
|---------|-------------|
| `status` | Process, connectivity, role, uptime |
| `license` | License validity, expiry date, type (trial/commercial) |
| `cluster` | Repmgr cluster topology |
| `replication` | Replication lag / standby connections |
| `check` | 14-item health threshold check — exit 0=OK / 1=WARN / 2=FAIL |
| `space [frag]` | Disk, tables, WAL, archive; `frag` adds fragmentation |
| `diagnose [--full]` | Root-cause diagnostic report (fast <15s; `--full` ~90s) |

### [DBA] Deep-Dive Commands

| Command | Description |
|---------|-------------|
| `sessions` | Non-idle session list |
| `locks [hold\|wait\|deadlock]` | Lock analysis |
| `perf [slow\|bloat\|vacuum\|wait\|io\|wal\|top]` | Performance diagnostics |
| `sql [pid\|all]` | SQL text + EXPLAIN for a session |
| `stmt [queryid]` | SQL history stats — Top N by mean/total/IO/calls (AWR-style) |
| `wait` | Wait event distribution |
| `progress` | Long-running operation progress |
| `params [pattern]` | Instance parameters |
| `stat` | Throughput metrics (TPS, buffer hit rate) |
| `obj <schema.table>` | Object deep-dive: size, indexes, constraints |
| `colstat <schema.table> [--col <col>]` | Column statistics (n_distinct, MCV, correlation) |
| `temp` | Temp file and sort spill analysis |
| `watch <N> <cmd>` | Repeat any command every N seconds |
| `conf [diff]` | Configuration audit / node comparison |
| `audit` | Security and compliance checks |
| `logs` | Log file analysis (slow queries, errors) |
| `remote <nodes> <cmd>` | Multi-node batch diagnostics |
| `kill [--terminate] [pid\|--long N\|--idle-txn N] [--dry-run] [--force]` | Cancel or terminate queries |
| `idx [unused\|dup\|bloat\|missing]` | Index health analysis |
| `advisor [index\|vacuum\|params\|analyze] [--fix]` | DBA recommendations; `--fix` emits executable SQL |
| `all` | Run all checks |

## Configuration File

Per-host settings can be placed in `~/.kbdiagrc` (sourced at startup before any defaults):

```bash
# ~/.kbdiagrc — example
KB_PORT=5432
KB_BIN_DIR=/opt/kingbase/bin
KB_SUPERUSER=dba
KB_SLOW_THRESHOLD=3
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KB_PORT` | `54321` | Database port |
| `KB_BIN_DIR` | `/home/kingbase/cluster/install/kingbase/bin` | Binary directory |
| `KB_SUPERUSER` | `system` | Superuser name |
| `KB_DB` | `test` | Database name |
| `KB_WARN_CONN` / `KB_FAIL_CONN` | `70` / `90` | Connection usage % |
| `KB_WARN_LAG` / `KB_FAIL_LAG` | `30` / `300` | Replication lag (seconds) |
| `KB_SLOW_THRESHOLD` | `5` | Slow query threshold (seconds) |
| `KB_WARN_HIT` / `KB_FAIL_HIT` | `95` / `90` | Buffer hit rate % lower bound |

## Requirements

- Run as the `kingbase` OS user (`sudo -i -u kingbase`)
- KingbaseES V8R6+
- repmgr optional (cluster commands skipped when absent)

---

<a name="中文"></a>

KingbaseES 命令行 DBA 工具集。非交互式——直接对运行实例查询，输出状态、拓扑、复制延迟、性能瓶颈、索引健康、列统计和优化建议。支持单机和主备集群（repmgr）。

## 安装

单文件，无依赖：

```bash
curl -fsSL https://raw.githubusercontent.com/Kevin-wenyu/kbdiag/main/dist/kbdiag \
  -o ~/kbdiag && chmod +x ~/kbdiag
```

## 更新

```bash
~/kbdiag update
```

## 团队批量部署

一次推送到多台主机：

```bash
# 创建主机列表文件（每行 user@host）
cp scripts/hosts.example my-hosts.txt
vim my-hosts.txt

# 批量部署
bash scripts/deploy.sh my-hosts.txt

# 自定义 SSH 端口或目标路径
SSH_PORT=2222 bash scripts/deploy.sh my-hosts.txt
```

## 快速开始

```bash
# 先切换到 kingbase 系统用户
sudo -i -u kingbase

# 全量扫描（每日巡检）
~/kbdiag all

# 健康状态门控（适合监控脚本）
~/kbdiag check
echo $?   # 0=全部正常  1=告警  2=故障

# DBA 详细模式
~/kbdiag check -v
~/kbdiag perf slow -v
```

## 全局参数

```
kbdiag [全局参数] <命令> [子命令] [命令参数]

  -v, --verbose       显示底层完整数据
  -q, --quiet         仅显示 WARN/FAIL（屏蔽 OK/INFO）
  -n N, --top N       限制结果行数（默认：10）
  --format text|json  输出格式（默认：text）
  --no-color          关闭 ANSI 颜色
  --timeout N         数据库查询超时秒数（默认：10）
```

## 命令速查

### [看] 运维命令（无需 DBA 背景）

| 命令 | 说明 |
|------|------|
| `status` | 进程状态、连接性、角色、运行时长 |
| `license` | 授权有效期、类型（试用/正式）、序列号 |
| `cluster` | Repmgr 集群拓扑 |
| `replication` | 复制延迟 / 备节点连接数 |
| `check` | 14 项健康阈值检查，exit 0=正常 / 1=告警 / 2=故障 |
| `space [frag]` | 磁盘、表大小、WAL、归档；`frag` 增加碎片分析 |
| `diagnose [--full]` | 根因诊断报告（快速 <15s；`--full` 完整约 90s） |

### [查] DBA 精准深查命令

| 命令 | 说明 |
|------|------|
| `sessions` | 非空闲会话列表 |
| `locks [hold\|wait\|deadlock]` | 锁分析：持有者 / 等待者 / 死锁检测 |
| `perf [slow\|bloat\|vacuum\|wait\|io\|wal\|top]` | 慢查询 / 表膨胀 / 垃圾回收 / 等待事件 / IO / WAL / Top SQL |
| `sql [pid\|all]` | 会话 SQL 全文 + EXPLAIN 计划 |
| `stmt [queryid]` | SQL 历史统计 AWR 报告（均值/总耗时/IO/调用频率 Top N）；指定 queryid 下钻 |
| `wait` | 等待事件分布 |
| `progress` | 长时间操作进度（VACUUM、CREATE INDEX 等） |
| `params [pattern]` | 实例参数查询（支持模糊匹配） |
| `stat` | 吞吐量指标（TPS、Buffer 命中率，差值采样） |
| `obj <schema.table>` | 对象深查：大小、索引、约束 |
| `colstat <schema.table> [--col <col>]` | 列统计深查（n_distinct、MCV、相关性） |
| `temp` | 临时文件与排序溢出分析 |
| `watch <N> <cmd>` | 每隔 N 秒重复执行任意 kbdiag 命令 |
| `conf [diff]` | 配置审计；`diff` 比对节点差异 |
| `audit` | 安全与合规检查 |
| `logs` | 日志文件分析（慢查询、报错） |
| `remote <nodes> <cmd>` | 多节点批量诊断 |
| `kill [--terminate] [pid\|--long N\|--idle-txn N] [--dry-run] [--force]` | 取消或终止查询 / 会话 |
| `idx [unused\|dup\|bloat\|missing]` | 索引健康分析 |
| `advisor [index\|vacuum\|params\|analyze] [--fix]` | 综合 DBA 建议；`--fix` 输出可执行 SQL |
| `all` | 运行所有检查 |

## 配置文件

每台主机的个性化配置放在 `~/.kbdiagrc`（启动时自动加载，优先于默认值）：

```bash
# ~/.kbdiagrc 示例
KB_PORT=5432
KB_BIN_DIR=/opt/kingbase/bin
KB_SUPERUSER=dba
KB_SLOW_THRESHOLD=3
```

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KB_PORT` | `54321` | 数据库端口 |
| `KB_BIN_DIR` | `/home/kingbase/cluster/install/kingbase/bin` | 二进制目录 |
| `KB_SUPERUSER` | `system` | 超级用户名 |
| `KB_DB` | `test` | 数据库名 |
| `KB_WARN_CONN` / `KB_FAIL_CONN` | `70` / `90` | 连接数使用率 % |
| `KB_WARN_LAG` / `KB_FAIL_LAG` | `30` / `300` | 复制延迟（秒） |
| `KB_SLOW_THRESHOLD` | `5` | 慢查询阈值（秒） |
| `KB_WARN_HIT` / `KB_FAIL_HIT` | `95` / `90` | Buffer 命中率下限 % |

## 运行要求

- 以 `kingbase` 系统用户运行（`sudo -i -u kingbase`）
- KingbaseES V8R6+
- repmgr 可选（不存在时集群命令自动跳过）
