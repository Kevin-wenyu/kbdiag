# stmt 命令 + diagnose 集成设计文档

**日期：** 2026-06-22  
**状态：** 已批准

## 背景

kbdiag 采用"看/查/断"三层设计哲学（见 CLAUDE.md）。`sys_stat_statements` 是 KingbaseES 的 SQL 历史统计视图，包含所有执行过的 SQL 的累积性能数据（调用次数、耗时、IO 消耗等）。

现有 `diagnose` 的 `_diag_slow_queries` 只看 `sys_stat_activity`（当前正在运行的慢 SQL），没有历史维度，无法判断"这次慢是偶发还是一贯如此"。本设计补全这条缺失的链路。

## 设计决策

- **查层**：新增 `kbdiag stmt` 命令，独立可用，AWR 风格多维 Top N 展示
- **断层**：`diagnose` 内部集成，两处：增强现有 slow_queries 检查 + 新增历史负担 Top SQL 检查
- **时间窗口**：使用累积数据（不引入持久化快照），输出里展示 `stats_reset` 时间戳
- **EXPLAIN**：不自动执行，输出可填参数的模板命令，避免占位符导致的误导性计划

---

## 第一节：`kbdiag stmt`（查层）

### 无参数：AWR 风格多维 Top N

```
==> SQL 历史统计  [数据覆盖: 2026-06-01 09:00 → 现在, 共 21 天]

── Top 10 by 平均耗时 ──────────────────────────────────────────────
 # │ queryid          │ calls │ mean_ms  │ max_ms   │ %total │ SQL 摘要
 1 │ 3f7a1b2c8d4e9f0a │ 1,203 │ 8,312.4  │ 45,001.0 │  38.2% │ SELECT o.*, c.name FROM orders o JOIN ...
 2 │ ...

── Top 10 by 总耗时 (calls × mean) ──────────────────────────────────
 # │ queryid          │ calls     │ total_s  │ mean_ms │ %total │ SQL 摘要
 1 │ a1b2c3d4e5f60718 │ 2,847,391 │ 14,237.1 │    5.0  │  65.4% │ SELECT id, status FROM orders WHERE ...

── Top 10 by IO（cache miss，shared_blks_read）────────────────────
 # │ queryid          │ calls  │ blks_read │ hit_rate │ SQL 摘要
 1 │ ...

── Top 10 by 调用频率 ──────────────────────────────────────────────
 # │ queryid          │ calls     │ mean_ms │ SQL 摘要
 1 │ ...
```

字段说明：
- `%total`：该 SQL 占所有 SQL 累计总耗时的百分比
- `SQL 摘要`：`sys_stat_statements.query` 截断至 60 字符

### 带参数：`kbdiag stmt <queryid>` 下钻

```
==> SQL 详情  queryid: 3f7a1b2c8d4e9f0a

── 执行统计 ──────────────────────────────────────────────────────────
调用次数    : 1,203
平均耗时    : 8,312.4 ms
最小 / 最大 : 120.3 ms / 45,001.0 ms
标准差      : 3,847.2 ms    ← 高标准差表明执行时间不稳定
规划时间    : 15.2 ms (avg)

── IO 分解 ───────────────────────────────────────────────────────────
shared_blks_hit   : 48,203,912   (cache hit)
shared_blks_read  : 12,047,301   (disk read)
Buffer 命中率     : 80.0%        ← 低于正常水平
temp_blks_read    : 0
temp_blks_written : 0

── SQL 全文 ──────────────────────────────────────────────────────────
SELECT o.id, o.status, o.created_at, c.name, c.email
FROM orders o
JOIN customers c ON c.id = o.customer_id
WHERE o.created_at > $1
  AND o.status = $2
ORDER BY o.created_at DESC;

── 验证路径 ──────────────────────────────────────────────────────────
执行计划（填入实际参数后运行）:
  EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
  SELECT o.id, o.status, o.created_at, c.name, c.email
  FROM orders o
  JOIN customers c ON c.id = o.customer_id
  WHERE o.created_at > '2026-06-01'   -- 替换为实际值
    AND o.status = 'pending'           -- 替换为实际值
  ORDER BY o.created_at DESC;

注意: 执行计划基于实际参数，上方 SQL 中 $1/$2 为占位符，需替换为真实值后再分析
```

### JSON 输出（`--format json`）

无参数时：
```json
{
  "command": "stmt",
  "stats_reset": "2026-06-01T09:00:00Z",
  "top_by_mean": [...],
  "top_by_total": [...],
  "top_by_io": [...],
  "top_by_calls": [...]
}
```

带 queryid 时：
```json
{
  "command": "stmt",
  "queryid": "3f7a1b2c8d4e9f0a",
  "stats_reset": "2026-06-01T09:00:00Z",
  "calls": 1203,
  "mean_exec_time": 8312.4,
  "min_exec_time": 120.3,
  "max_exec_time": 45001.0,
  "stddev_exec_time": 3847.2,
  "mean_plan_time": 15.2,
  "shared_blks_hit": 48203912,
  "shared_blks_read": 12047301,
  "hit_ratio": 80.0,
  "temp_blks_read": 0,
  "temp_blks_written": 0,
  "query": "SELECT o.id ..."
}
```

---

## 第二节：diagnose 集成（断层）

### 2A：增强 `_diag_slow_queries`

当前 `_diag_slow_queries` 检测 `sys_stat_activity` 中运行超过 `KB_SLOW_THRESHOLD` 秒的活跃 SQL。增强：对每条发现的慢 SQL，去 `sys_stat_statements` 查其历史均值，给出"偶发 vs 一贯"判断。

判断规则：
- 当前耗时 > 历史均值 × 3 → **性能突降（异常）**，可能有外部因素（锁、统计信息变化、数据量激增）
- 当前耗时 ≈ 历史均值 → **一贯慢（设计问题）**，SQL 本身效率低

输出示例（断层 finding body）：
```
症状: 2 条 SQL 当前执行超过 5s
  · queryid=3f7a1b2c  当前: 8.2s  历史均值: 0.3s  stddev: 0.1s  → 性能突降（异常）
    SQL: SELECT o.*, c.name FROM orders o JOIN customers c ...
  · queryid=a1b2c3d4  当前: 6.1s  历史均值: 5.9s  stddev: 0.8s  → 一贯慢（设计问题）
    SQL: SELECT id, status FROM orders WHERE created_at > $1 ...
根因假设:
  · queryid=3f7a1b2c: 历史均值仅 0.3s，突然变慢，可能原因：统计信息过期、锁等待、数据量激增
验证路径:
  · kbdiag stmt 3f7a1b2c8d4e9f0a
  · kbdiag obj <table>   （确认统计信息更新时间）
```

如果当前没有活跃慢查询，跳过 sys_stat_statements 查询（不额外增加耗时）。

### 2B：新增 `_diag_stmt_top`（完整模式 `--full`）

从 `sys_stat_statements` 捞历史总负担 Top 3，不依赖当前活跃会话，作为 INFO 级别展示。

```
[INFO] 历史 SQL 负担 Top 3（累计总耗时，stats_reset: 2026-06-01）
  · queryid=a1b2c3d4  总耗时: 14,237s (65%)  calls: 2.8M  mean: 5ms
    SELECT id, status FROM orders WHERE created_at > $1 ...
  · queryid=3f7a1b2c  总耗时: 8,614s (38%)   calls: 1,203  mean: 8,312ms
    SELECT o.*, c.name FROM orders o JOIN ...
  验证: kbdiag stmt（查看完整 AWR 报告）
```

---

## 第三节：技术实现

### 关键查询

**Top SQL（多维，无参数）：**
```sql
-- Top by mean_exec_time
SELECT queryid::text, calls, mean_exec_time, max_exec_time,
       total_exec_time,
       round(100.0 * total_exec_time / sum(total_exec_time) OVER (), 1) AS pct_total,
       left(query, 60) AS query_summary
FROM sys_stat_statements
WHERE calls > 0
ORDER BY mean_exec_time DESC
LIMIT 10;
```

**queryid 下钻：**
```sql
SELECT calls, mean_exec_time, min_exec_time, max_exec_time, stddev_exec_time,
       mean_plan_time,
       shared_blks_hit, shared_blks_read,
       round(100.0 * shared_blks_hit / NULLIF(shared_blks_hit + shared_blks_read, 0), 1) AS hit_ratio,
       temp_blks_read, temp_blks_written,
       query
FROM sys_stat_statements
WHERE queryid::text = '<queryid>';
```

**slow_queries 关联（diagnose 内部）：**
```sql
-- 对 sys_stat_activity 发现的 queryid 列表，批量查历史均值
SELECT queryid::text, calls, mean_exec_time, stddev_exec_time
FROM sys_stat_statements
WHERE queryid::text = ANY(ARRAY[...]);
```

**stats_reset 时间戳：**
```sql
SELECT stats_reset FROM sys_stat_bgwriter;
```

### sys_stat_statements 可用性检查

若扩展未安装，`stmt` 命令需优雅降级：
```
[WARN] sys_stat_statements 扩展未启用
  启用方式: shared_preload_libraries = 'sys_stat_statements'  (需重启)
```

`diagnose` 集成部分：找不到视图时跳过，不报错，不影响其他检查项。

### 文件变更

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `lib/cmd_stmt.sh` | 新建 | `cmd_stmt()` 函数，无参/有参两个路径 |
| `lib/cmd_diagnose.sh` | 修改 | `_diag_slow_queries` 增加历史均值关联；新增 `_diag_stmt_top` |
| `kbdiag.sh` | 修改 | source + dispatch |
| `build.sh` | 修改 | lib 列表 + dispatch + USAGE |
| `dist/kbdiag` | rebuild | 单文件产物 |
| `test/cases/test_stmt.sh` | 新建 | 7 个测试 |

### 测试覆盖（`test/cases/test_stmt.sh`）

| 测试 | 覆盖点 |
|------|--------|
| `test_stmt_exit_zero` | 无参数正常退出 |
| `test_stmt_has_four_sections` | 输出包含四个 Top N 区块 |
| `test_stmt_has_stats_reset` | 展示 stats_reset 时间戳 |
| `test_stmt_queryid_exit_zero` | 带 queryid 参数正常退出 |
| `test_stmt_queryid_has_explain_template` | 输出含验证路径/EXPLAIN 模板 |
| `test_stmt_json_valid` | `--format json` 输出合法 JSON |
| `test_stmt_no_extension_graceful` | 扩展未启用时不 crash，输出可读错误 |
