# DBA 专家工具箱重构 — 循环迭代工程记录

> 目标:把 kbdiag 从"诊断脚本集合"推进到"DBA 专家工具箱"。
> 工作方式:自适应循环(/loop),每轮一个可独立验证的增量,门禁全过才提交。
> 本文件是过程记录(key log),每轮迭代追加一节。

## 工程实现方式

### 每轮迭代的固定流程

1. **选任务**:从下方任务队列取最上面一个未完成项,一轮只做一件事。
2. **实现**:改 `lib/cmd_*.sh`,新命令同时更新 `build.sh` USAGE + dispatch + README。
3. **验证门禁**(顺序执行,任一失败停下修复,不进入下一步):
   - `bash build.sh` — 重新打包
   - `bash -n dist/kbdiag` — 语法检查
   - `shellcheck lib/*.sh` — 静态检查(pre-commit hook 会再兜一次)
   - `bash test/run_tests.sh <suite>` — 回归测试(新行为先补测试用例)
   - `bash test/perf_check.sh` — 性能测试(21 命令 wall-clock 预算门禁)
   - VM 实机跑一遍新/改命令,肉眼确认输出合理(重点看权限拒绝、空数据等边界)
4. **落地**:commit(说明 why)+ push。
5. **记录**:本文件追加迭代小节;roadmap 记忆同步更新。
6. **调度**:ScheduleWakeup 进入下一轮。

### 安全边界

- 门禁不过不 push;循环中不做破坏性操作(force push、删表、VM 重建);
  每轮增量可独立回滚;批次跑 5~8 轮后主动汇报再继续。

### 回归测试

`test/run_tests.sh` — 28 个用例文件,自动发现 `test_*` 函数,SSH 编排双节点真实
KingbaseES 实例。新功能必须带用例;改动已有行为必须先改用例再改实现。

### 性能测试

`test/perf_check.sh`(本轮新增)— 在 VM 内计时(排除 SSH 开销),每个命令一个
宽松预算(约观测基线的 3 倍以上),抓的是数量级回归(新增 N+1 查询、漏 LIMIT),
不是毫秒级噪声。排除项:`kill`(有副作用)、`update`(走网络)、`watch`(循环)、
`remote`(需节点参数)、`obj`/`colstat`(需对象参数)、`all`(聚合)。

## 任务队列(按 spec 优先级)

| # | 任务 | 层 | 状态 |
|---|------|-----|------|
| 0 | 性能测试 harness + 基线 | 工程 | ✅ 2026-07-08 本轮 |
| 1 | `backup` 命令:归档状态、last archived/failed、archive_command 有效性、备份工具检测(sys_rman) | OPS/DBA | 待做 |
| 2 | `check`/`diagnose` 集成归档失败信号(archiver failed_count、WAL 堆积根因链) | ROOT-CAUSE | 待做 |
| 3 | `explain` 命令:给定 SQL/queryid 的计划分析(Phase 3) | DBA | 待做 |
| 4 | `jobs`(kdb_schedule 定时任务检查)(Phase 4) | DBA | 待做 |
| 5 | `partition`(分区表健康)(Phase 4) | DBA | 待做 |
| 6 | audit 补充:连接来源白名单(sys_hba.conf 分析) | DBA | 待做 |
| 7 | 次要消重遗留:checkpoint/bgwriter 数据源、ANALYZE drift 公式 | 工程 | 待做 |

## 性能基线(2026-07-08,kes-node1,V8R6)

| command | measured (ms) | budget (s) |
|---------|--------------:|-----------:|
| `status` | 620 | 10 |
| `license` | 444 | 10 |
| `cluster` | 536 | 15 |
| `replication` | 360 | 10 |
| `check` | 2601 | 30 |
| `space` | 483 | 15 |
| `params` | 261 | 10 |
| `sessions` | 406 | 10 |
| `locks` | 307 | 10 |
| `perf` | 1309 | 30 |
| `wait` | 371 | 10 |
| `progress` | 464 | 10 |
| `stat` | 10126 | 20 |
| `stmt` | 827 | 15 |
| `temp` | 485 | 10 |
| `idx` | 1011 | 20 |
| `conf` | 262 | 10 |
| `audit` | 1909 | 20 |
| `logs` | 1319 | 30 |
| `diagnose` | 1263 | 30 |
| `advisor` | 1780 | 30 |

注:`stat` 10.1s 是差值采样(sleep 采样窗口)的固有耗时,非回归。

## 迭代日志

### 迭代 0 — 2026-07-08:工程骨架

- 新增 `test/perf_check.sh`:VM 内计时的性能预算门禁,21 命令全部通过,基线见上表。
- 踩坑:循环体内 `ssh` 会吃掉喂给 `while read` 的 heredoc stdin,导致只跑第一个命令
  就退出——`</dev/null` 修复。这与 audit 迭代发现的 `set -e` 裸赋值坑同属
  "shell 静默行为"类,已入过程记录供后续排查参考。
- 本文件建立,作为循环过程的 key log。
- 下一轮:任务 #1(`backup` 命令)。老规矩:先上 VM 实测 sys_stat_archiver、
  archive_command、sys_rman 存在性等真实行为,再写检查逻辑。
