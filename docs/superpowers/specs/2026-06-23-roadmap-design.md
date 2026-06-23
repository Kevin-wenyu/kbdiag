# kbdiag 路线图设计 — 平台 → 质量 → 功能

**日期：** 2026-06-23  
**状态：** 已批准  
**方案：** C — 先建平台安全网，再补质量，再扩功能

---

## 背景

kbdiag 是一个纯 Shell 编写的 KingbaseES 命令行诊断工具，共 28 个命令。v2/v3/v1.4 设计文档中的功能已全部实现。本路线图针对完整度审计发现的三个剩余缺口：

1. **无安全网** — 无 CI、无 git 守卫、无提交前静态检查
2. **测试覆盖不均** — 各命令 2~28 个测试不等；根因层仅有烟测
3. **两个薄弱命令** — watch（20 行）和 stat（40 行）远未达到设计潜力

**已确认约束：** GitHub Actions CI 无法运行集成测试（Lima VM 在本机，runner 无法 SSH）。CI 范围 = shellcheck + build 验证。集成测试仍需本地手动运行。

---

## 第一阶段：平台层

### 1a. git 安全守卫（Claude Code hook）

使用 `git-guardrails-claude-code` 技能，在 Claude Code 会话中拦截危险 git 操作：

- 拦截命令：`git push --force`、`git reset --hard`、`git clean -f`、`git branch -D`
- 作用范围：项目级 `.claude/settings.json`
- 原因：目前直推 main，无分支保护，一次误操作即丢历史

### 1b. GitHub Actions CI

文件：`.github/workflows/ci.yml`

触发条件：push 和 pull_request 到 main

流水线作业：
1. **shellcheck** — 在 ubuntu-latest 上运行 `shellcheck lib/*.sh`
2. **build** — 运行 `bash build.sh`，验证 `dist/kbdiag` 存在且可执行
3. **smoke** — 运行 `dist/kbdiag --version` 和 `dist/kbdiag --help`（无需数据库）

不在范围内：集成测试（依赖本地 Lima VM）。

### 1c. shellcheck pre-commit hook

文件：`.git/hooks/pre-commit`（本地安装，不提交）  
安装脚本：`scripts/setup-dev.sh`（提交到仓库，供团队复现）

行为：每次 `git commit` 前运行 `shellcheck lib/*.sh`。有错误则中断提交。警告允许保留（必要时用行内注解抑制 SC2034 等）。

---

## 第二阶段：质量层

### 2a. diagnose — 逐项断言测试

现状：16 个 `_diag_*` 子检查，只有 9 个烟测。  
目标：每个子检查至少一个测试验证输出结构，而不仅仅是退出码。

需补测的子检查：

| 子检查 | 新增断言 |
|--------|---------|
| `_diag_long_txn` | 存在 finding 或 clean 标签 |
| `_diag_connections` | 包含连接数数字 |
| `_diag_locks` | 包含锁数量 |
| `_diag_replication` | 包含延迟值或单机标签 |
| `_diag_disk` | 包含磁盘使用百分比 |
| `_diag_xid_age` | 包含 XID age 数字 |
| `_diag_vacuum_debt` | 包含死元组数 |
| `_diag_bloat` | 包含膨胀百分比或 clean |
| `_diag_buffer_hit` | 包含命中率百分比 |
| `_diag_checkpoint` | 包含 checkpoint 间隔 |
| `_diag_temp` | 包含临时文件大小 |
| `_diag_params` | 至少一条参数 finding |
| `_diag_indexes` | 包含 finding 或 clean |
| `_diag_slow_queries` | 包含 finding 或"无慢查询" |
| `_diag_stmt_top` | 扩展不存在时包含跳过提示 |

测试文件：`test/cases/test_diagnose.sh`（在现有文件追加）

### 2b. advisor --fix SQL 语法验证

现状：3 个测试仅检查 SQL 关键词存在。  
目标：验证生成的 SQL 可被解析（通过 `ksql -c` 执行或 EXPLAIN 验证语法合法性）。

新增测试：
- `test_advisor_fix_index_sql_executable` — 验证生成的 CREATE INDEX SQL 可解析
- `test_advisor_fix_vacuum_sql_parseable`
- `test_advisor_fix_analyze_sql_parseable`

### 2c. 薄弱测试套件补强

| 命令 | 现有 | 目标 | 新增测试重点 |
|------|------|------|-------------|
| sessions | 3 | 8 | 状态过滤、长查询检测、idle-in-txn 计数 |
| replication | 4 | 8 | 主库显示备库数量、备库显示延迟、verbose 列 |
| watch | 4 | 6 | watch locks 运行、watch wait 运行 |
| remote | 4 | 6 | --all 语法、单节点语法 |
| update | 2 | 4 | 离线错误提示、版本输出格式 |

### 2d. /tmp 硬编码修复

`lib/cmd_logs.sh:47` 直接使用 `/tmp`。改为 `${TMPDIR:-/tmp}` 以尊重环境配置。

---

## 第三阶段：功能层

### 3a. watch — 多子命令

现状：`watch` 只封装了 `status`（20 行，5 秒间隔）。  
设计：扩展为支持 `watch [status|sessions|locks|wait]`，带清屏刷新。

子命令：
- `watch status`（当前行为，默认）
- `watch sessions` — 实时刷新活跃会话
- `watch locks` — 实时刷新锁等待
- `watch wait` — 实时刷新等待事件汇总

接口：`kbdiag watch [子命令] [间隔秒数]`  
默认间隔：5 秒。刷新间清屏（status 已实现此逻辑）。

### 3b. stat — AWR 维度扩展

现状：`stat` 采集两个快照，计算约 6 个指标的差值（40 行）。  
设计：扩展至 DBA 期望的标准 AWR 维度。

新增指标：
- Top SQL 按总耗时排序（需 sys_stat_statements，不可用时跳过）
- 等待事件差值（区间内突增的等待类型）
- 锁获取次数差值
- 临时文件生成量差值
- Checkpoint 频率

输出：每个维度一个结构化段落，沿用现有列头风格。

---

## 技能使用对照

| 阶段 | 任务 | 技能 |
|------|------|------|
| 1a | git 守卫 | `git-guardrails-claude-code` |
| 1b–1c | CI + pre-commit | `setup-pre-commit`（参考模式）+ 手写 shell |
| 2–3 | 具体实现 | `implement` |

---

## 验收标准

- **第一阶段**：`shellcheck lib/*.sh` 零错误通过；CI badge 在 main 上常绿；Claude Code 内 force-push 被拦截
- **第二阶段**：所有命令 ≥ 6 个测试；diagnose 有逐项断言；lib/ 下零 shellcheck 错误
- **第三阶段**：`watch` 支持 4 个子命令；`stat` 覆盖 5 个 AWR 维度；新代码全部通过 CI
