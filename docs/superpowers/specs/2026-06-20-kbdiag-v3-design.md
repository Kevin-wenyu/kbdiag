# kbdiag v3 Feature Design

Date: 2026-06-20

## Context

kbdiag v2 已具备：status / cluster / replication / sessions / locks / check / perf / space / all，支持 `-v` verbose 模式，单文件分发。

v3 目标：从"诊断工具"升级为**DBA 专家工具箱**。参考 Oracle `ora` 工具（10000+ 行 Shell）、pg_profile、pgBadger、pgmetrics 的能力，在零侵入（不建表、不装扩展）约束下最大化覆盖 DBA 日常场景。

约束：
- 纯 Shell，`kingbase` 用户运行，不建表，不装扩展
- 所有 DB 查询通过 `ksql_q`，禁止交互会话
- 单文件分发（`build.sh` 打包 `dist/kbdiag`）
- HTML 报告留 v4，v3 不做

---

## 架构变更：参数解析增强

### 当前 v2 参数结构

```
kbdiag [-v] <command> [subcommand]
```

### v3 参数结构

```
kbdiag [global-flags] <command> [subcommand] [command-flags]
```

**全局标志**（`lib/core.sh` 统一解析，所有命令可用）：

| 标志 | 变量 | 说明 |
|------|------|------|
| `-v` / `--verbose` | `VERBOSE=1` | 显示底层详细数据（v2 已有） |
| `-q` / `--quiet` | `QUIET=1` | 只输出 WARN/FAIL，屏蔽 OK/INFO |
| `-n N` / `--top N` | `TOP_N=N`（默认 10） | 限制结果行数 |
| `--format text\|json\|csv` | `OUTPUT_FMT=text\|json\|csv`（默认 text） | 输出格式 |
| `--no-color` | `NO_COLOR=1` | 强制关闭 ANSI 颜色 |
| `--timeout N` | `KB_QUERY_TIMEOUT=N`（默认 10） | DB 查询超时秒数 |

**解析逻辑**（`core.sh`）：将全局标志从 `$@` 中剥离，剩余参数传给对应 `cmd_*` 函数：

```bash
parse_global_args() {
  ARGS=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -v|--verbose)  VERBOSE=1   ;;
      -q|--quiet)    QUIET=1     ;;
      --no-color)    NO_COLOR=1  ;;
      -n|--top)      TOP_N="$2"; shift ;;
      --top=*)       TOP_N="${1#--top=}" ;;
      --format)      OUTPUT_FMT="$2"; shift ;;
      --format=*)    OUTPUT_FMT="${1#--format=}" ;;
      --timeout)     KB_QUERY_TIMEOUT="$2"; shift ;;
      *)             ARGS+=("$1") ;;
    esac
    shift
  done
}
```

`ok`/`warn`/`fail`/`info` 输出函数在 `QUIET=1` 时屏蔽 `ok`/`info`；`NO_COLOR=1` 时清空颜色变量。

---

## 新增顶层命令

### `kbdiag sql [pid|all]`（对标 Oracle `plan`+`fulltext`+`sqlstats`）

针对**具体会话**的 SQL 深度分析。

```bash
kbdiag sql 1234          # pid=1234 的完整 SQL 文本 + EXPLAIN
kbdiag sql all           # 所有非 idle 会话摘要
kbdiag sql 1234 -v       # 加上 EXPLAIN ANALYZE（需 DBA 谨慎用）
```

输出内容：
- pid / 用户 / 应用 / 客户端 IP / 状态 / 已运行时长
- 完整 SQL 文本（不截断）
- 等待事件（wait_event_type + wait_event）
- `EXPLAIN` 执行计划（默认）；`-v` 时用 `EXPLAIN (ANALYZE, BUFFERS)`

实现：`sys_stat_activity` 取 SQL 文本和 pid，通过 `ksql -c "EXPLAIN ..."` 查询执行计划。
- 默认：`EXPLAIN` 只查计划，安全无副作用
- `-v`：`EXPLAIN (VERBOSE, COSTS)` 加详细节点信息
- `--analyze`：`EXPLAIN (ANALYZE, BUFFERS)` 实际执行，**有真实 IO，DBA 需知情后使用**

---

### `kbdiag wait`（对标 Oracle `events`）

等待事件分布分析。

```bash
kbdiag wait              # 当前等待事件类型汇总（按等待数量排序）
kbdiag wait -v           # 每个等待事件 + 具体会话 + SQL 摘要
```

数据源：`sys_stat_activity` 的 `wait_event_type`、`wait_event`、`state`。

---

### `kbdiag progress`（对标 Oracle `longops`）

长时间操作进度监控。

```bash
kbdiag progress          # 所有进行中的长操作（VACUUM/CREATE INDEX/CLUSTER 等）
```

数据源：
- `sys_stat_progress_vacuum` — VACUUM 进度（堆块/索引扫描百分比）
- `sys_stat_progress_create_index` — CREATE INDEX 进度
- `sys_stat_progress_cluster` — CLUSTER 进度
- `sys_stat_activity` 中 `state='active'` 且 duration > 60s 的其他查询

---

### `kbdiag params [pattern]`（对标 Oracle `params`）

实例参数查询，支持模式匹配。

```bash
kbdiag params                    # 所有非默认参数
kbdiag params work_mem           # 精确匹配
kbdiag params log                # 含 log 的所有参数
kbdiag params --all              # 全部参数（包含默认值）
```

数据源：`sys_settings`，字段：`name`、`setting`、`unit`、`boot_val`（默认值）、`pending_restart`。

显示逻辑：默认只显示 `setting != boot_val` 的参数（非默认值）；`pending_restart=true` 的参数标记 `[RESTART]`。

---

### `kbdiag temp`（对标 Oracle `temp`+`sort_usage`）

临时文件和排序溢出分析。

```bash
kbdiag temp              # 当前 temp 文件使用总量 + 产生 temp 的会话
```

数据源：
- `sys_stat_database`：`temp_files`、`temp_bytes`（累计）
- `sys_stat_activity`：`temp_blks_read`、`temp_blks_written`（当前会话）
- `sys_stat_bgwriter`：相关字段

---

### `kbdiag stat [--interval N] [--count N]`（对标 AWR 速率指标）

实例吞吐量速率指标，采样两次 `sys_stat_database` 计算 delta。

```bash
kbdiag stat                      # 默认采样间隔 5 秒，显示 1 次
kbdiag stat --interval 2         # 每 2 秒采样
kbdiag stat --interval 5 --count 10  # 采样 10 次后退出
```

输出指标（per second）：
- TPS（commits/s、rollbacks/s）
- 缓冲命中率 = `blks_hit / (blks_hit + blks_read)`
- 临时文件生成速率（temp_bytes/s）
- 死锁频率（deadlocks/s）
- 行操作速率（tup_inserted + tup_updated + tup_deleted /s）

---

### `kbdiag obj <schema.table>`（对标 Oracle `idxdesc`+`optstats`+`segsize`）

针对**具体表**的全面分析。

```bash
kbdiag obj public.orders
kbdiag obj orders                # 省略 schema 时搜索 search_path
kbdiag obj orders -v             # 加上列统计信息
```

输出内容：
1. **基本信息**：行数估算（`reltuples`）、物理大小（`pg_relation_size`）、总大小含索引
2. **Bloat**：live/dead tuple 比率
3. **统计信息新鲜度**：`last_analyze`、`last_autovacuum`、修改行数（n_mod_since_analyze）
4. **索引列表**：每个索引的大小、扫描次数、是否唯一、是否被使用
5. **约束**：PK、FK、CHECK、UNIQUE（名称 + 列）
6. **`-v` 附加**：列级统计（null 率、distinct 值数、最常见值）

数据源：`sys_stat_user_tables`、`sys_indexes`、`sys_stat_user_indexes`、`sys_constraint`、`sys_attribute`、`sys_stats`。

---

### `kbdiag watch <interval> <command> [command-args]`（对标 Oracle `repeat`）

周期性刷新执行任意 kbdiag 命令。

```bash
kbdiag watch 5 sessions          # 每 5 秒刷新 sessions
kbdiag watch 2 wait --top 5      # 每 2 秒刷新 wait，只看 top 5
kbdiag watch 10 check -q         # 每 10 秒，只打印 WARN/FAIL
```

实现：Shell `while true` 循环，`clear`（或打印分隔线），执行子命令，`sleep <interval>`。支持 Ctrl+C 退出。

---

### `kbdiag conf [diff]`（配置审计）

```bash
kbdiag conf              # 所有非默认参数 + 待重启参数
kbdiag conf diff         # 与 node2 的参数差异（通过 SSH 对比）
```

`conf diff` 逻辑：SSH 到集群另一节点取 `sys_settings`，与本节点对比，输出差异行。节点列表从 `$KB_REPMGR_CONF` 推断，或通过 `KB_PEER_HOST` 环境变量指定。

---

### `kbdiag audit`（安全审计）

```bash
kbdiag audit             # 安全/合规巡检
kbdiag audit -v          # 详细：每项问题的具体对象列表
```

检查项：

| 检查项 | 数据源 | WARN 条件 |
|--------|--------|-----------|
| 超级用户账号列表 | `sys_roles` | superuser=true 且非系统角色 |
| 免密码角色 | `sys_roles` | rolpassword IS NULL + 非 peer 认证 |
| public schema 写权限 | `information_schema.role_table_grants` | 非 owner 有 INSERT/UPDATE/DELETE |
| 无主键的表 | `sys_constraint` | 无 PK 约束的用户表 |
| 长期未使用账号 | `sys_stat_activity` last login | >90 天无登录的角色 |
| 无 owner 对象 | `sys_class` | relowner = 0 或 dropped user |

---

### `kbdiag logs [options]`（日志分析）

```bash
kbdiag logs                              # 分析最新日志文件
kbdiag logs --since 2h                   # 最近 2 小时
kbdiag logs --file /path/to/kbs.log     # 指定文件
kbdiag logs --level error               # 只看 ERROR/FATAL
kbdiag logs --top 20                    # 慢查询 Top 20
```

实现：纯 Shell + awk，读取 KingbaseES 日志文件（CSV 格式优先，text 格式兜底）。

输出：
- 慢查询 Top N（duration > `KB_SLOW_THRESHOLD`，按耗时排序）
- ERROR/FATAL 按小时分布（ASCII 柱状图）
- 最频繁的错误类型 Top 10
- 连接/断开事件统计

日志路径自动探测：从 `sys_settings` 读 `log_directory` + `log_filename`，不需手动指定。

---

### `kbdiag remote <nodes> <command>`（多节点）

```bash
kbdiag remote node1,node2 check         # 批量健康检查
kbdiag remote --all status              # 所有配置节点
kbdiag remote node2 sql all             # 查 node2 的活跃 SQL
```

节点来源：repmgr 集群信息（优先）或 `~/.kbdiag/nodes.conf`（每行：`alias ssh-连接串`）。

实现：串行 SSH，在每个节点上执行 `/tmp/kbdiag <command>`（需提前部署），聚合输出。加前缀标识节点来源。

---

## 扩展现有命令

### `check` 新增 8 项

| 检查项 | WARN | FAIL | 环境变量 |
|--------|------|------|----------|
| 缓冲命中率 | <95% | <90% | `KB_WARN_HIT` / `KB_FAIL_HIT` |
| Checkpoint 频率 | >每 5 分钟 1 次 | >每 1 分钟 1 次 | `KB_WARN_CKPT` / `KB_FAIL_CKPT` |
| 临时文件使用 | >100MB | >1GB | `KB_WARN_TEMP` / `KB_FAIL_TEMP` |
| 死锁计数（自上次重置） | >0 | — | — |
| 复制槽延迟 | lag >100MB | >1GB | `KB_WARN_SLOT` / `KB_FAIL_SLOT` |
| bgwriter 压力 | buffers_backend/total >30% | >60% | `KB_WARN_BGW` / `KB_FAIL_BGW` |
| **XID 年龄 / Wraparound 风险** | age >10亿 | age >19亿 | `KB_WARN_XID` / `KB_FAIL_XID` |
| **最老活跃事务年龄** | >1h | >4h | `KB_WARN_OLDEST_TXN` / `KB_FAIL_OLDEST_TXN` |

XID 数据源：`sys_class.relfrozenxid`（表级）、`sys_database.datfrozenxid`（库级），取最大值。
最老活跃事务：`sys_stat_activity` 中 `xact_start` 最早的非 idle 会话。

### `perf` 新增子命令

- `perf wait` — 等待事件统计（类型 + 计数 + 平均等待时长）
- `perf io` — 表/索引 I/O 热点（`sys_statio_user_tables`、`sys_statio_user_indexes`）
- `perf wal` — WAL 生成速率、checkpoint 统计、bgwriter 统计
- `perf top [buffers|reads|writes|cpu|temp]` — 当前 Top SQL 按维度排序
- `perf vacuum` 增强：每张表显示距 autovacuum 触发阈值的进度
  - 公式：`threshold = autovacuum_vacuum_threshold + autovacuum_vacuum_scale_factor × reltuples`
  - 输出示例：`dead tuples: 45,000 / threshold: 50,200 (89%) — 即将触发`

### `locks` 新增子命令

- `locks hold` — 持锁会话详情（对标 Oracle `hold_txlock`）
- `locks wait` — 等锁会话详情（对标 Oracle `wait_txlock`）
- `locks deadlock` — 死锁统计（从 `sys_stat_database.deadlocks`）

### `space` 新增子命令

- `space frag` — 表/索引碎片率（live ratio < 阈值的表，对标 Oracle `tab_frag`/`index_frag`）

---

## 命令参数速查

### 各命令专用参数

| 命令 | 专用参数 | 说明 |
|------|----------|------|
| `logs` | `--file <path>` | 指定日志文件路径 |
| `logs` | `--since <dur>` | 时间窗口（1h/30m/2d） |
| `logs` | `--level error\|warn\|all` | 日志级别过滤 |
| `sql` | `<pid>\|all` | 目标进程或全部 |
| `obj` | `<schema.table>` | 目标表（必须） |
| `stat` | `--interval N` | 采样间隔秒数（默认 5） |
| `stat` | `--count N` | 采样次数（默认 1） |
| `watch` | `<interval> <cmd>` | 间隔秒数 + 子命令 |
| `params` | `[pattern]` | 参数名模糊匹配 |
| `params` | `--all` | 显示所有参数含默认值 |
| `conf` | `diff` | 与对端节点参数对比 |
| `remote` | `<nodes>` / `--all` | 目标节点列表 |
| `perf top` | `buffers\|reads\|writes\|cpu\|temp` | 排序维度 |

---

## 命令分层：OPS / DBA

`usage` 输出中明确标注，运维无需理解 DBA 命令：

```
[OPS]  status       实例进程、连通性、角色、运行时长
[OPS]  cluster      repmgr 集群拓扑
[OPS]  replication  复制延迟 / standby 连接
[OPS]  check        健康阈值，输出 OK/WARN/FAIL，exit 0/1/2
[OPS]  space        磁盘 / 表大小 / WAL
[DBA]  sessions     非 idle 会话详情
[DBA]  locks        等待锁链 / 持锁分析
[DBA]  perf         性能诊断（慢查询/bloat/vacuum/wait/io/wal）
[DBA]  sql <pid>    SQL 深度分析 + 执行计划
[DBA]  wait         等待事件分布
[DBA]  progress     长时间操作进度
[DBA]  params       实例参数查询
[DBA]  stat         吞吐量速率指标（TPS/命中率）
[DBA]  obj <table>  对象级全面分析
[DBA]  temp         临时文件 / 排序溢出
[DBA]  conf         配置审计 / 节点参数对比
[DBA]  audit        安全合规审计
[DBA]  watch        周期刷新执行任意命令
[DBA]  logs         日志文件分析
[DBA]  remote       多节点批量诊断
[ALL]  all          运行全部检查
```

---

## 验证框架（Test Task 0）

**原则：测试脚本跑在本地 Mac，通过 SSH 编排 VM，VM 上零依赖。**

```
test/
├── run_tests.sh              # 主入口：bash test/run_tests.sh [command]
├── lib/
│   ├── assert.sh             # assert_contains / assert_exit_code / assert_json_valid
│   └── ssh_helpers.sh        # ssh_node1 / ssh_node2 / deploy_to_nodes
├── setup/
│   ├── make_slow_query.sql   # pg_sleep 制造慢查询
│   ├── make_lock.sql         # LOCK TABLE 制造锁竞争
│   ├── make_bloat.sql        # 批量 DELETE 制造 bloat
│   └── cleanup.sql           # 清理所有测试状态
└── cases/
    ├── test_status.sh
    ├── test_check.sh
    ├── test_replication.sh
    ├── test_sessions.sh
    ├── test_locks.sh
    ├── test_perf.sh
    ├── test_space.sh
    ├── test_sql.sh
    ├── test_wait.sh
    ├── test_obj.sh
    ├── test_params.sh
    ├── test_stat.sh
    ├── test_progress.sh
    ├── test_temp.sh
    ├── test_conf.sh
    └── test_audit.sh
```

每个 test case 必须覆盖：happy path、empty path、exit code、`-v`/`-q`/`--top`、两节点差异、`--format json` 合法性。

每个功能 Task 的完成标准：**`bash test/run_tests.sh <cmd>` 全部 PASS 才 commit**。

---

## 实现优先级

| 优先级 | 内容 | 理由 |
|--------|------|------|
| P0 | **测试框架**（test/ 目录） | 所有功能的验证基础，必须先做 |
| P0 | 全局参数解析增强（core.sh） | 所有命令共用，先于功能实现 |
| P1 | `check` 6 项新增 | 巡检核心，直接补现有命令 |
| P1 | `perf` 新增 wait/io/wal/top | 扩展现有框架 |
| P1 | `locks` 新增 hold/wait/deadlock | 扩展现有框架 |
| P1 | `space` 新增 frag | 扩展现有框架 |
| P2 | `kbdiag sql` | 高频 DBA 动作 |
| P2 | `kbdiag wait` | 高频故障排查 |
| P2 | `kbdiag progress` | 高价值，纯查视图 |
| P2 | `kbdiag params` | 简单，高频 |
| P2 | `kbdiag stat` | 速率指标，DBA 必需 |
| P2 | `kbdiag obj` | 对象分析，复杂但价值高 |
| P3 | `kbdiag temp` | 补充 space 空白 |
| P3 | `kbdiag watch` | 纯 shell 循环，简单 |
| P3 | `kbdiag conf` | 配置审计 |
| P3 | `kbdiag audit` | 安全审计 |
| P4 | `kbdiag logs` | 需日志访问，较复杂 |
| P4 | `kbdiag remote` | 依赖多节点 SSH 配置 |
