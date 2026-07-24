# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

kbdiag — KingbaseES 命令行诊断工具，Shell 编写，不进交互界面直接查实例状态。支持单机和主备集群（repmgr）。

## Development Environment

### Test VMs (Lima)

SSH 端口由 Lima 每次启动动态分配，不要硬编码——用 `limactl list` 确认当前端口，或直接用 `limactl shell <name>` 免密登录（推荐，不受端口/host key 变化影响）。

| Node | Role | Shell | Internal IP |
|------|------|-----|-------------|
| kes-node1 | primary | `limactl shell kes-node1` | 192.168.105.10 |
| kes-node2 | standby | `limactl shell kes-node2` | 192.168.105.11 |

若需要 raw SSH（如脚本里 `ssh -p PORT ...`），先跑 `limactl list --format '{{.Name}} {{.SSHLocalPort}}'` 拿到当前端口，端口变化后旧的 known_hosts 记录会导致 host key 校验失败，需要 `ssh-keygen -R '[127.0.0.1]:<old_port>'` 清理。

### KingbaseES

- Port: 54321
- OS user: `kingbase`
- Connect: `ksql test system` (local socket, no password)
- Cluster manager: repmgr
- Replication user: esrep

### Login sequence

```bash
ssh -p 57103 kevin@127.0.0.1   # node1 (primary)
# or
ssh -p 57123 kevin@127.0.0.1   # node2 (standby)

sudo -i
su - kingbase
ksql test system                # connect to DB
```

### Check cluster status

```bash
su - kingbase
repmgr cluster show
```

## Architecture

Pure Shell scripts — no build step, no dependencies. Scripts run directly on the DB host as the `kingbase` OS user.

Design rules:
- **Non-interactive**: never open `psql`/`ksql` interactive sessions; use `ksql -c "..."` or `ksql -f file.sql`
- **Single-node + HA**: every check must degrade gracefully when repmgr is absent (standalone mode)
- **No sudo assumed**: scripts run as `kingbase`; escalation must be explicit and documented

### 三层命令设计哲学

kbdiag 的命令按深度分三层，新命令设计前先确认它属于哪一层：

| 层 | 名称 | 定位 | 深度要求 | 当前命令 |
|----|------|------|----------|----------|
| 看 | OPS 层 | 快速事实查询，给一个确定答案 | 无需深度，准确即可 | `status` `check` `cluster` `params` `license` `space` `version` |
| 查 | DBA 层 | 单项精确深查，可独立使用，也可验证"断"层结论 | 深度，单维度 | `obj` `colstat` `locks` `perf` `sql` `wait` `slow` `bloat` `vacuum` |
| 断 | 根因层 | 多维数据关联，输出完整诊断链（症状→证据→根因→建议） | 最深，多维综合 | `diagnose` `advisor` |

**设计原则：**
- 看层：不展开，不分析，用户要的是事实
- 查层：单命令回答一个具体问题，输出结构化可机读；DBA 用它来验证"断层"的结论
- 断层：输出是链路，不是 finding 列表——每个结论必须能追溯到证据，并指向"查层"验证路径

## Git / Deploy

- Remote: `https://github.com/Kevin-wenyu/kbdiag.git`
- Branch strategy: push directly to `main`

## KingbaseES 特有行为（v2 开发中发现）

- 布尔值返回 `true`/`false`（非 PostgreSQL 的 `t`/`f`）：比较前必须 `tr -d '[:space:]'`
- 系统视图前缀 `sys_`：`sys_stat_activity`、`sys_locks`、`sys_stat_replication` 等
- Size 函数保留 `pg_` 前缀：`pg_relation_size`、`pg_total_relation_size`、`pg_database_size`
- WAL 目录：`$KB_DATA_DIR/sys_wal`（非 `pg_wal`），代码中需回退兼容
- `pg_is_wal_receiver_up()` 不存在，用 `sys_stat_wal_receiver.status` 代替
- `||` 拼接把 NULL 当空串（Oracle 模式）：`NULL || '/'` 得 `'/'` 而非 NULL，不能靠 `coalesce(a||b, fallback)` 兜底，要用 CASE 显式判断
- `cmd_check || exit $?` — 非零 exit code 需显式传播（`set -e` 在 case 分支行为不一致）
- `cmd_check || true` — 在 all 命令中允许 WARN 不中断流程

## Shell 脚本陷阱（set -e / set -u）

- 算术展开在 `set -e` 下会短路退出：`((x++))` 结果为 0 时被判定为失败触发 exit，改用 `x=$((x+1))` 或 `: $((x++))`
- exit trap 里不要引用可能未定义的局部变量——`set -u` 下会因 unbound variable 报错；trap 内用到的变量需提前赋默认值
- 每次修复这类问题都要配一个回归测试，防止再犯

## 代码结构

- `lib/core.sh` — 配置、全局参数解析、颜色、输出函数、JSON helpers
- `lib/cmd_*.sh` — 各命令实现，每文件一个 `cmd_*` 函数
- `build.sh` — cat 拼接所有 lib/*.sh + dispatch → `dist/kbdiag`
- `dist/kbdiag` — 单文件分发产物，不手动编辑
- `test/` — 测试框架，本地 Mac 运行，通过 SSH 编排 VM

## 开发工作流

```bash
# 修改 lib/cmd_xxx.sh
bash build.sh                          # 重新打包
bash test/run_tests.sh xxx             # 自动验证
git add lib/cmd_xxx.sh dist/kbdiag && git commit
```

## Agent 技能工作流（mattpocock/skills）

项目已装 [mattpocock/skills](https://github.com/mattpocock/skills) 插件（`claude plugin install mattpocock-skills@mattpocock --scope project`，装在本仓库范围，见 `.claude/settings.json` 的 `enabledPlugins`）。按 kbdiag 的开发阶段对应使用：

| 阶段 | 技能 | 用途 | 触发方式 |
|------|------|------|----------|
| 想法模糊、要对齐需求 | `/grill-with-docs` | 连环追问直到需求树的每个分支都定下来，顺带维护 `CONTEXT.md`/ADR | 用户显式输入 |
| 调研能力缺口（对标 ora/pg_profile/pgBadger/pgmetrics/pganalyze 之类） | `research` | 后台 agent 查一手资料，产出带引用的 Markdown 落库 | 模型可主动用，也可用户触发 |
| 结论定型，要写规格 | `/to-spec` | 把对话变成 spec，发布到 issue tracker（本仓库落 `docs/superpowers/specs/`，见下方 setup） | 用户显式输入 |
| spec 拆成可执行任务 | `/to-tickets` | 拆成 tracer-bullet 票据，标出阻塞关系 | 用户显式输入 |
| 单次会话装不下的大工作量 | `/wayfinder` | 把大任务铺成票据地图，一次解一张，直到路线清晰 | 用户显式输入 |
| 写代码/修 bug 的实现阶段 | `implement`（内部驱动 `tdd`，收尾用 `code-review`） | 按 spec/票据实现，红绿重构一次一个切片 | 模型可主动用 |
| 难缠的 bug / 性能回归 | `diagnosing-bugs` | 复现→最小化→假设→插桩→修复→回归测试，和本文件"Shell 脚本陷阱"里"每次修复都配回归测试"的约定一致 | 模型可主动用 |
| 提交前审查 | `code-review` | 双轴审查：Standards（代码规范+坏味道）+ Spec（是否忠实实现原始需求），并行子 agent | 模型可主动用 |
| 术语/领域模型打磨（比如三层命令哲学的边界） | `domain-modeling` | 挑战术语、用场景压测，同步更新 `CONTEXT.md`/ADR | 模型可主动用 |
| 架构体检 | `/improve-codebase-architecture` | 扫描代码找深化点，出 HTML 报告，选一个再连环追问 | 用户显式输入 |
| 新命令/新输出格式不确定怎么设计 | `prototype` | 先搭一次性原型验证设计问题，不进正式代码 | 模型可主动用 |
| 合并冲突（少见，因为直推 main） | `resolving-merge-conflicts` | 按 hunk 溯源两边意图解决，不用 `--abort` | 模型可主动用 |
| 会话太长要交接 | `/handoff` | 把当前对话压成交接文档，配合本项目已有的 memory 系统跨会话续接 | 用户显式输入 |
| 不确定该用哪个技能 | `/ask-matt` | 路由到合适的技能/流程 | 用户显式输入 |

**必做的一次性初始化**：首次使用 `to-spec`/`to-tickets`/`wayfinder`/`triage` 前，需要用户自己敲 `/setup-matt-pocock-skills`（该技能标了 `disable-model-invocation`，我不能代为触发）配置本仓库的 issue tracker 落点、triage 标签、领域文档布局。目前 kbdiag 用 `docs/superpowers/specs/` 存 spec，setup 时按此对齐，不要让它改去开 GitHub Issues。

**命名冲突提醒**：本环境同时装了另一套 superpowers 技能框架，也有一个叫 `grilling` 的技能。写 `/grill-with-docs`（engineering 分类，带工程语境）而不是裸写 "grilling"，避免歧义。

## 测试节点

- node1 (primary): `ssh -p 57103 kevin@127.0.0.1` → `sudo -i -u kingbase`
- node2 (standby): `ssh -p 57123 kevin@127.0.0.1` → `sudo -i -u kingbase`
- DB 连接：`ksql test system`（本地 socket，免密）
- 端口：54321
