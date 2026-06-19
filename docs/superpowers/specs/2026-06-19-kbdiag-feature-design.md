# kbdiag Feature Design

Date: 2026-06-19

## Context

kbdiag 是 KingbaseES 命令行诊断工具，Shell 编写，以 `kingbase` 用户运行，不进交互界面直接输出实例状态。当前已有 `status / cluster / replication / sessions / locks / all` 六个命令，部署在两节点主备集群（repmgr）。

目标用户：DBA（深度数据）+ 运维值班（结论明确）。
核心场景：日常巡检 + 故障排查。
分发方式：单一可执行文件，`curl` 下载即用。

---

## Architecture

### 目录结构

```
kbdiag/
├── lib/
│   ├── core.sh              # 颜色输出、ksql_q、config、require_kingbase_user
│   ├── cmd_status.sh        # 进程、连通性、角色、运行时长
│   ├── cmd_cluster.sh       # repmgr 集群拓扑
│   ├── cmd_replication.sh   # 复制延迟 / standby 连接
│   ├── cmd_sessions.sh      # 非 idle 会话
│   ├── cmd_locks.sh         # 等待锁链
│   ├── cmd_check.sh         # NEW 健康告警阈值
│   ├── cmd_perf.sh          # NEW 性能诊断
│   └── cmd_space.sh         # NEW 存储空间
├── kbdiag.sh                # 主入口：source lib/* + dispatch
├── build.sh                 # 打包脚本
├── dist/
│   └── kbdiag               # 单文件产物（分发用）
└── README.md
```

### 构建方式

`build.sh` 按顺序 `cat` 拼接所有模块，输出到 `dist/kbdiag`：

```
shebang + set -euo pipefail
→ lib/core.sh
→ lib/cmd_status.sh
→ lib/cmd_cluster.sh
→ lib/cmd_replication.sh
→ lib/cmd_sessions.sh
→ lib/cmd_locks.sh
→ lib/cmd_check.sh
→ lib/cmd_perf.sh
→ lib/cmd_space.sh
→ dispatch（case 语句）
```

无外部依赖，纯 shell 拼接。

### 安装

```bash
curl -fsSL https://github.com/Kevin-wenyu/kbdiag/releases/latest/download/kbdiag \
  -o /tmp/kbdiag && chmod +x /tmp/kbdiag
sudo -i -u kingbase /tmp/kbdiag all
```

---

## Verbose Mode

所有命令支持 `-v` / `--verbose` 标志，控制输出层次：

- **默认（运维视角）**：结论摘要，OK/WARN/FAIL + 关键数字
- **`-v`（DBA 视角）**：结论 + 底层完整查询结果

`core.sh` 中解析全局 `VERBOSE` 变量，各 `cmd_*.sh` 用 `if [[ $VERBOSE ]]; then` 控制详细输出块。`check` 命令在 `-v` 下每项检查附带触发该状态的原始行数据。

```bash
kbdiag check          # 运维：只看 OK/WARN/FAIL
kbdiag check -v       # DBA：结论 + 底层数据
kbdiag perf slow -v   # DBA：慢查询完整信息
kbdiag all -v         # DBA：全量诊断
```

---

## Feature Spec

### P1 — `check`（健康告警）

每项输出 `[OK]` / `[WARN]` / `[FAIL]`。整体 exit code：0=全OK，1=有WARN，2=有FAIL，方便接入监控脚本或巡检报告。

| 检查项 | WARN | FAIL | 环境变量覆盖 |
|--------|------|------|-------------|
| 连接数占比 | >70% max_connections | >90% | `KB_WARN_CONN` / `KB_FAIL_CONN` |
| 复制延迟（standby） | >30s | >300s | `KB_WARN_LAG` / `KB_FAIL_LAG` |
| 长事务 | >5min | >30min | `KB_WARN_TXN` / `KB_FAIL_TXN` |
| 等待锁 | 有等待锁 | — | — |
| 归档失败 | archiver 有 failed | — | — |
| autovacuum 积压 | dead tuples >10万 | — | `KB_WARN_DEAD_TUPLES` |

standalone 模式下自动跳过复制延迟检查。

### P2 — `perf`（性能诊断）

支持子命令：`kbdiag perf [slow|bloat|vacuum|index]`，不带子命令时全部运行。

- **slow**：当前运行超过 N 秒的 SQL（默认 `KB_SLOW_THRESHOLD=5`），显示 pid/用户/时长/SQL前80字符
- **bloat**：膨胀率 >20% 的表，显示实际大小 vs 理论大小（基于 `sys_stat_user_tables`）
- **vacuum**：last_autovacuum 超过 7 天 或 dead tuple ratio >10% 的表（Top 20）
- **index**：过去 30 天从未被扫描的索引（基于 `sys_stat_user_indexes`，排除主键）

### P3 — `space`（存储空间）

- 数据目录磁盘使用率（`df -h $KB_DATA_DIR`），>80% WARN，>90% FAIL
- Top 10 最大表（含 toast + index，`pg_total_relation_size`）
- WAL 目录大小（`pg_wal/`）
- 各数据库大小（`pg_database_size`）
- 归档目录大小（若 `archive_command` 已配置）

---

## README Structure

```
# kbdiag
一句话介绍

## 安装
curl 单行安装

## 快速开始
all / check 示例

## 命令参考
命令表（含新增三个）

## 环境变量
连接参数 + 告警阈值覆盖

## 要求
kingbase 用户、KingbaseES V8R6+、repmgr 可选
```

---

## Implementation Order

1. 重构：现有命令拆入 `lib/`，写 `build.sh`，验证 `dist/kbdiag` 产物与现有行为一致
2. 实现 `cmd_check.sh`（P1）
3. 实现 `cmd_perf.sh`（P2）
4. 实现 `cmd_space.sh`（P3）
5. 更新 `dist/kbdiag`（build），更新 README
