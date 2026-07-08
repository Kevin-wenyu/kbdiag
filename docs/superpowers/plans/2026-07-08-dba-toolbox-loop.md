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
| 1 | `backup` 命令:归档状态、last archived/failed、archive_command 有效性、备份工具检测(sys_rman) | OPS/DBA | ✅ 2026-07-08 迭代 1 |
| 2 | `check`/`diagnose` 集成归档失败信号(archiver failed_count、WAL 堆积根因链) | ROOT-CAUSE | ✅ 2026-07-08 迭代 2 |
| 3 | `explain` 命令:给定 SQL/queryid 的计划分析(Phase 3) | DBA | 待做 |
| 4 | `jobs`(kdb_schedule 定时任务检查)(Phase 4) | DBA | 待做 |
| 5 | `partition`(分区表健康)(Phase 4) | DBA | 待做 |
| 6 | audit 补充:连接来源白名单(sys_hba.conf 分析) | DBA | 待做 |
| 7 | 次要消重遗留:checkpoint/bgwriter 数据源、ANALYZE drift 公式 | 工程 | 待做 |
| 8 | diagnose 回归套件存量失败排查(基线 18 过/15 败,与本轮改动无关,疑似环境相关) | 工程 | 待做 |

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

### 迭代 1 — 2026-07-08:`backup` 命令(任务 #1)

- **VM 实测先行**,发现测试环境归档正在真实失败:`archived_count=0`、
  `failed_count=27771` 且持续增长,archive_command 走 sys_rman
  (`--config /rman/kbbr_repo/sys_rman.conf`)。这个活故障直接成为检查逻辑的
  真实验证用例——比构造 mock 可信得多。
- 新增 `lib/cmd_backup.sh`,5 个检查:归档配置(archive_mode/wal_level/
  archive_command)、归档器裁决(failing/recovered/ok/idle,时间戳比较放在
  SQL 服务端做)、积压 WAL(`archive_status/*.ready` 计数,阈值
  `KB_WARN_WAL_READY=10`/`KB_FAIL_WAL_READY=100`,sys_wal→pg_wal 回退)、
  sys_rman 二进制+config 存在性(支持 `--config=X` 和 `--config X` 两种写法)、
  非活动复制槽(静默保留 WAL 撑爆磁盘的隐患)。
- 归位 OPS 层(USAGE/README 已加行);`fail()` 只打印不改 exit code,
  与 audit 同款语义(门控用 `check`,不用 backup)。
- 实测输出:归档器 FAILING(27783 次)、49 个 .ready 积压(WARN 档)、
  sys_rman 二进制和 config 都在——说明故障在更深层(stanza/repo),
  fail 信息里给出的"手工跑 archive_command 验证"指引正确。
- 门禁:build/syntax/shellcheck 过;回归 7/7(含 standby 优雅降级);
  perf 门禁 backup 1084ms / 预算 15s,全套 PASSED。
- 下一轮:任务 #2 —— `check`/`diagnose` 集成归档失败信号
  (check 加 archiver 项,diagnose 加 WAL 堆积→归档失败根因链)。

### 迭代 2 — 2026-07-08:check/diagnose 归档信号集成(任务 #2)

- `cmd_backup.sh` 抽出共享函数 `_backup_archiver_row()`(裁决 SQL 单点维护),
  三处消费:backup 自身、check 第 15 项、diagnose 根因链——延续 Phase 0 的
  "shared function reuse" 模式。
- `check` 新增第 15 项 WAL archiving:failing+积压≥`KB_FAIL_WAL_READY` → FAIL;
  failing → WARN;off → ok(是否该开归档由 backup 命令负责提示,check 不重复告警)。
  USAGE/README 的 "14-item" 同步改 15。
- `diagnose` 新增 `_diag_archiver`:症状(.ready 积压)→ 证据(失败计数/最后
  失败段/最后成功)→ 根因(archive_command 原文)→ 建议(kbdiag backup 验证、
  手工执行归档命令、sys_rman 排查路径)。实机渲染正确,`%p` 经 `printf %b`
  参数位安全。
- **基线对比流程实际用上了**:diagnose 套件 14 败初看吓人,git stash 基线
  证实改动前就是 15 败——存量环境问题,与本轮无关(净效果 +2 过/-1 败),
  已入队任务 #8 排查。教训:stash pop 会被重建的 dist/kbdiag 挡住,
  先 `git checkout -- dist/kbdiag` 再 pop。
- 门禁:check 套件 13/13(含新用例)、backup 7/7、diagnose 无回归;
  perf check 1873ms / diagnose 706ms / backup 1031ms,全部在预算内。
- 下一轮:任务 #3 `explain` 命令(Phase 3)或任务 #8 存量失败排查——
  倾向先做 #8,测试套件不干净会污染后续所有迭代的判断。

### 迭代 2.5 — 2026-07-08:CI 热修(用户报障)

- GitHub CI 自 07-06 起每次 push 都红:CI 的 apt 版 shellcheck 对三处
  `A && B || C`(cmd_conf.sh / cmd_logs.sh / cmd_stat.sh)报 SC2015 且判失败,
  本地 0.11.0 不报——**本地 shellcheck 过 ≠ CI 过,版本行为有差**。
- 三处改写为 if 语句(语义不变,顺带去掉 `|| true` 拐杖),conf 8/8、
  logs 10/10、stat 15/15 回归全过,push 后 CI 转绿(run 28954029732)。
- 教训入库:门禁清单里"shellcheck 过"应以 CI 的版本为准;后续可考虑
  本地跑 `shellcheck -S info` 对齐严格度。
