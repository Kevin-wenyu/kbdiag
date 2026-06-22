# diagnose 命令设计文档

**日期：** 2026-06-22
**状态：** 已批准

## 背景

kbdiag 现有 25 个命令均为独立快照，用户出事时需要逐条执行才能拼出完整图景。`diagnose` 的目标是：一条命令完成诊断，结论优先，做到 RCA（根因分析）深度，覆盖救火、巡检、上线前评估三个场景。

## 设计决策

- **复用为主**：调用现有 `_advisor_*` 内部函数（新增 `--collect` 模式），不重写查询逻辑
- **时间轴**：当前版本专注快照诊断，不引入数据持久化；唯一例外是复制延迟做 3 秒内双采样 delta，判断 lag 方向
- **两档模式**：默认快速（< 15s），`--full` 完整（约 90s）

---

## 第一节：输出格式

结论优先，每条 finding 完整 RCA 结构：

```
==> Diagnose  [快速模式，耗时 8s]

● 发现 2 个严重问题，3 个需关注，2 条信息

[CRITICAL] 长事务
  症状：3 个事务持续 > 4h
  证据：
    · pid 1234 | user: app_user | app: batch-report | IP: 192.168.1.10
      状态: idle in transaction | 时长: 4h32m
      最后 SQL: SELECT * FROM orders WHERE created_at > ...
    · pid 5678 正在阻塞 pid 2001、2002（等待行锁）
  根因假设（按可能性排序）：
    ① 批处理作业未显式 commit/rollback（高）
       依据：application_name=batch-report，SQL 是全表扫描，idle in transaction
  影响链：
    长事务 → 持有锁 → 阻塞 2 个活跃会话
    长事务 → 阻碍 autovacuum → orders 表 dead tuples 已 1.2M
  建议：
    · pid 1234、9012 满足安全终止三条件（见下），可执行：
      SELECT pg_terminate_backend(1234);
      SELECT pg_terminate_backend(9012);
    · pid 5678（active，阻塞其他）→ 先执行 kbdiag sql 5678 确认 SQL 性质

[WARN]  复制延迟
  症状：node2 延迟 45s，且仍在增大（+3s/3s）
  ...

[INFO]  慢查询：当前有 2 条运行超过 5s

加 --full 可运行完整检查（预计约 90s）
```

### 分级规则

| 级别 | 建议深度 |
|------|----------|
| CRITICAL | 给可执行 SQL，标注安全终止条件依据 |
| WARN | 给操作方向或具体诊断命令 |
| INFO | 点到为止，不展开 |

### "可安全终止"三条件

同时满足才标注为可安全处置，并在输出中展示判断依据：
1. `state = 'idle in transaction'`
2. 不在任何会话的阻塞链上（用 `pg_blocking_pids(pid)` 展开；实现时需 SSH 进 VM 验证 KingbaseES 是否支持，若无则 fallback 到 `sys_locks` JOIN 方式）
3. 无 Exclusive DDL lock（`locktype IN ('relation','extend','tuple')` AND `mode LIKE '%Exclusive%'`）

---

## 第二节：检查项范围

### 快速模式（默认，目标 < 15s）

| 检查项 | 数据来源 | RCA 深度 |
|--------|----------|----------|
| 长事务 + prepared xacts | `sys_stat_activity` + `sys_prepared_xacts` + `sys_locks` | 完整：来源/阻塞链/假设 |
| 连接数饱和 | `sys_stat_activity` + `sys_settings` | 完整：按 state/user/app 分解 |
| 等待锁（含 advisory/relation/tuple） | `sys_locks` + `sys_stat_activity` | 完整：阻塞链，锁类型分类展示 |
| 当前慢查询 | `sys_stat_activity` | 完整：SQL 摘要、wait_event |
| 复制延迟 + slot 积压 + **delta** | `sys_stat_wal_receiver` + `sys_replication_slots` | 完整：lag 方向（3s 双采样） |
| 磁盘空间 + 归档积压 | `pg_database_size` + `archive_status` + df | 简单：占比 + ready 文件数 |
| XID age 危险 | `sys_stat_user_tables` | 简单：距 wraparound 距离 |

**性能保证**：将统计类查询合并到 2-3 次 `ksql` 调用，每次 `statement_timeout=3s`，总耗时可控。

### 完整模式（`--full` 追加）

| 检查项 | 数据来源 |
|--------|----------|
| autovacuum 积压（dead tuples top 10 + 上次 vacuum 时间） | `sys_stat_user_tables` |
| 未使用/重复索引 | 复用 `_advisor_index --collect` |
| 参数建议（shared_buffers/work_mem） | 复用 `_advisor_params --collect` |
| buffer hit rate | `sys_stat_bgwriter` |
| bloat top 5 | `sys_stat_user_tables` |
| checkpoint 频率 | `sys_stat_bgwriter` |
| 临时文件累计量 | `sys_stat_database` |

---

## 第三节：技术实现

### findings 数据结构（平行数组）

```bash
FINDINGS_LEVEL=()      # CRITICAL / WARN / INFO
FINDINGS_CATEGORY=()   # long_txn / connections / locks / slow_queries / replication / disk / xid_age / ...
FINDINGS_BODY=()       # 完整文本块，换行安全，render 时 printf '%s\n'

_finding() {
  FINDINGS_LEVEL+=("$1")
  FINDINGS_CATEGORY+=("$2")
  FINDINGS_BODY+=("$3")
}
```

不使用分隔符拼接，避免 SQL 文本 / 文件路径中的 `|` 破坏解析。

### 函数结构（`lib/cmd_diagnose.sh`）

```
cmd_diagnose()                   # 入口，解析 --full 参数，调 render
  _diag_fast()                   # 快速检查集
    _diag_long_txn()             # 长事务 + prepared xacts
    _diag_connections()          # 连接饱和
    _diag_locks()                # 等待锁
    _diag_slow_queries()         # 慢查询
    _diag_replication()          # 复制延迟（含 3s delta）+ slot 积压
    _diag_disk()                 # 磁盘 + 归档积压
    _diag_xid_age()              # XID wraparound
  _diag_full()                   # 完整模式追加
    _diag_vacuum_debt()
    _diag_indexes()              # 调用 _advisor_index --collect
    _diag_params()               # 调用 _advisor_params --collect
    _diag_bloat()
    _diag_buffer_hit()
    _diag_checkpoint()
    _diag_temp()
  _diag_render()                 # 按 CRITICAL→WARN→INFO 排序，格式化输出
```

### advisor 函数改造

`_advisor_index` / `_advisor_params` 新增 `--collect` 模式：传入该参数时只往 FINDINGS 数组写数据，不执行任何打印。原函数（无参）行为保持不变，向后兼容。

### 复制延迟 delta

```bash
lag1=$(ksql_q "SELECT EXTRACT(EPOCH FROM write_lag)::int FROM sys_stat_replication LIMIT 1;")
sleep 3
lag2=$(ksql_q "SELECT EXTRACT(EPOCH FROM write_lag)::int FROM sys_stat_replication LIMIT 1;")
# delta = lag2 - lag1 > 0 → 在增大；< 0 → 在收敛
```

---

## 第四节：JSON 输出与工具集成

### JSON 输出

现有 `json_item` 为扁平四元组，无法表达 findings 的嵌套结构。新增专用 `json_finding()` helper：

```bash
json_finding() {
  local level="$1" category="$2"
  local body="$3"
  # 用 python3 -c "import json,sys; print(json.dumps(sys.stdin.read()))" 做安全转义
  local escaped
  escaped=$(printf '%s' "$body" | python3 -c "import json,sys; print(json.dumps(sys.stdin.read()))")
  local entry="{\"level\":\"$level\",\"category\":\"$category\",\"body\":$escaped}"
  _JSON_FINDINGS="${_JSON_FINDINGS:+${_JSON_FINDINGS},}${entry}"
}

json_diagnose_end() {
  local ts; ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  local mode="${1:-fast}"
  local critical warn info
  # count from FINDINGS_LEVEL array
  echo "{\"command\":\"diagnose\",\"mode\":\"$mode\",\"timestamp\":\"$ts\",\"summary\":{\"critical\":$critical,\"warn\":$warn,\"info\":$info},\"findings\":[${_JSON_FINDINGS}]}"
}
```

### 工具集成

| 文件 | 变更 |
|------|------|
| `lib/cmd_diagnose.sh` | 新建 |
| `lib/cmd_advisor.sh` | `_advisor_index` / `_advisor_params` 新增 `--collect` 模式 |
| `lib/core.sh` | 新增 `json_finding()` / `json_diagnose_end()` |
| `kbdiag.sh` | source + dispatch + help |
| `build.sh` | lib 列表 + dispatch + USAGE |
| `dist/kbdiag` | rebuild |

`kbdiag all` 追加 `cmd_diagnose`，暂时接受部分查询重复（all 语义是"全跑"）。

help 文本 `[OPS]` 区块追加：
```
  diagnose [--full]   RCA 根因诊断（快速 <15s；--full 完整约 90s）
```

### 测试（`test/cases/test_diagnose.sh`）

| 测试 | 覆盖点 |
|------|--------|
| exit code 0 | 正常完成 |
| 输出含 summary 行 `● 发现` | 结论优先格式 |
| 输出含 `[CRITICAL]` 或 `[WARN]` 或 `[INFO]` | findings 存在 |
| JSON 合法性 | `--format json` |
| evidence 为空时 JSON 输出 `[]` 不崩溃 | edge case |
| `--full --format json` 组合 | 两标志共存 |
| 无 repmgr 时不 exit（standalone 降级） | 单机模式 |

---

## 文件变更清单

| 文件 | 变更类型 |
|------|----------|
| `lib/core.sh` | 新增 `json_finding()` / `json_diagnose_end()` |
| `lib/cmd_diagnose.sh` | 新建（主体） |
| `lib/cmd_advisor.sh` | 改造：`--collect` 模式 |
| `kbdiag.sh` | source + dispatch |
| `build.sh` | lib 列表 + dispatch + USAGE |
| `dist/kbdiag` | rebuild |
| `test/cases/test_diagnose.sh` | 新建（7 个测试） |
