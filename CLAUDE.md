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

## 测试节点

- node1 (primary): `ssh -p 57103 kevin@127.0.0.1` → `sudo -i -u kingbase`
- node2 (standby): `ssh -p 57123 kevin@127.0.0.1` → `sudo -i -u kingbase`
- DB 连接：`ksql test system`（本地 socket，免密）
- 端口：54321
