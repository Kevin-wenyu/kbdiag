# kbdiag 企业级 DBA 工具箱设计 — 差距分析与路线图

**日期：** 2026-07-04
**状态：** 草案，待确认
**触发：** 用户反馈"功能太少，不像高级 DBA 专家的工具箱"，登录 kes-node1 实测验证

---

## 背景：实测发现了什么

登录 kes-node1（`limactl shell kes-node1`，原 CLAUDE.md 记录的 SSH 端口 57103 已失效，实际端口 60035，需要更新文档）后直接查库，发现：

1. **WAL 归档从未成功过**：`sys_stat_archiver` 显示 `archived_count=0, failed_count=12039`，`archive_command` 指向 `sys_rman archive-push`，持续失败到当前时刻。`sys_wal` 已堆积 112 个文件/1.6G（磁盘尚宽裕）。kbdiag 现有 28 个命令里，没有一个查询过 `sys_stat_archiver`——这意味着一个正在丢失 PITR 能力的实例，kbdiag 会显示"一切正常"。
2. **企业级扩展已装，kbdiag 视而不见**：`sysaudit`（审计）、`sysmac`/`sys_anon`（强制访问控制/脱敏）、`kdb_partman`（分区管理）、`kdb_schedule`（作业调度）、`walminer`（WAL 挖掘）全部已安装，但 `cmd_audit.sh` 只查 4 项最基础的东西（superuser、无密码角色、public 写权限、无主键表），其余扩展完全没有对应检查。
3. **sys_stat_statements 其实可用**（v1.11），需要确认 advisor/stat 里的"扩展不存在时跳过"分支是不是被误触发。

这三点共同指向一个结论：kbdiag 目前覆盖的是**日常巡检**（慢查询、膨胀、锁、vacuum 债务、等待事件），但一个"高级 DBA 专家的工具箱"首先要回答的是三个更根本的问题——**灾难来了能不能恢复、出了事能不能审计到人、容量什么时候会爆**——这三块 kbdiag 现在完全空白。

---

## 核心架构分叉：已否决

**决定：不引入本地状态/历史快照存储，保持纯实时查询。** kbdiag 坚持"Pure Shell、无依赖、无落盘"的项目原则；Phase 3 中依赖历史快照的能力（`trend`、执行计划回归对比）因此**取消**，不再规划。Phase 3 保留的部分收窄为"单次执行计划深挖"（`explain`），这个不需要历史存储。

---

## 优先级已重排：先消重，再谈新功能

用户反馈并明确排序：**"先把现有的功能夯实，分类做好，是不是有重复的，查询深度不够的"** —— 在做 Phase 1/2/4 任何新功能之前，先完成下面的 **Phase 0：现有 28 个命令的消重与分类审计**。这是当前唯一在执行的工作；Phase 1/2/4 仍然有效但顺延到 Phase 0 完成之后。

---

## Phase 0：现有功能消重与分类审计（已完成，2026-07-05）

**状态：本节列出的 6 项重构已全部落地并推送（commit e2b650e → 1514a9b）：**
- ✅ advisor↔idx 复用（advisor.sh 直接调用 idx.sh 的 unused/dup/missing 查询）
- ✅ 表膨胀/死元组阈值统一到 `core.sh` 的 `KB_WARN_DEAD_PCT`/`KB_FAIL_DEAD_PCT`/`KB_WARN_FRAG_PCT`，diagnose.sh 的 vacuum_debt+bloat 双重上报 bug 一并修掉
- ✅ 等待事件查询统一到 `cmd_wait.sh` 的 `_WAIT_EVENT_WHERE`（wait/perf wait/stat 三处引用）
- ✅ Top SQL 的扩展探测和 `calls > 0` 过滤统一到 `cmd_stmt.sh` 的 `_stmt_check_ext`/`_STMT_ACTIVE_WHERE`
- ✅ `conf` 默认视图收窄（不再重复 `params` 的非默认参数列表，只保留 restart-pending 信号 + `diff`）
- ✅ `--help`/README 改成三段 `[OPS]`/`[DBA]`/`[ROOT-CAUSE]`，`diagnose`/`advisor` 归位到 ROOT-CAUSE；同时补上了此前 `--help` 里完全缺失的 `idx`/`kill` 两个命令

以下小节保留原始审计记录（发现时的原文，未回填"已修复"标注到每一条），作为这次重构的依据存档。checkpoint/bgwriter 重叠（0.2 倒数第二条）和 ANALYZE drift 公式重叠（0.2 倒数第三条）**尚未处理**，影响较小，未纳入本轮 6 项任务范围。

通读了 `lib/cmd_*.sh` 全部 24 个命令文件 + `core.sh` + `build.sh` 后，逐条核实（非猜测）的发现如下。

### 0.1 分类问题

**`--help` 文本与 CLAUDE.md 文档的三层架构（看/查/断）不一致。** `build.sh` 里内嵌的 USAGE heredoc 只分了 `[OPS]` 和 `[DBA]` 两组：
- `diagnose` 被放进 `[OPS]`，但它是文档里明确定义的"断"层命令（多维根因），不该和 `status`/`check` 这类"看"层命令混在一起。
- `advisor` 放在 `[DBA]` 组的最后，没有和其他"查"层命令区分开——它其实也带一点"断"层性质（给建议、可 `--fix`）。
- 建议：`--help` 输出直接改成三段 `[看/OPS]` `[查/DBA]` `[断/ROOT-CAUSE]`，`diagnose`/`advisor` 移入第三段，与 CLAUDE.md 保持一致，避免文档和实际工具"两套真相"。

**`conf` 与 `params` 默认行为近乎完全重复。** 两者都是 `SELECT name, setting, unit, boot_val ... FROM sys_settings WHERE setting <> boot_val`，字段几乎一样。`conf` 唯一的独有能力是 `diff` 子命令（SSH 跨节点参数比对）。建议：`params` 保留作为独立"查看当前非默认参数"命令，`conf` 改名或收窄为专职"跨节点参数比对"（即只保留 `diff`），消除默认视图的重复入口，避免用户困惑"到底该用哪个"。

### 0.2 重复实现（同一逻辑独立写了多份，阈值还不一致）

**表膨胀/死元组比例——6 处独立实现，6 个不同阈值：**

| 位置 | 阈值/口径 |
|---|---|
| `cmd_perf.sh` `_perf_bloat` | live_pct < 80%（即约 20% dead） |
| `cmd_space.sh` `_space_frag` | dead 比例 > 10% |
| `cmd_check.sh` 健康检查项 | 固定数量阈值（非比例） |
| `cmd_advisor.sh` `_advisor_vacuum` | 20% |
| `cmd_diagnose.sh` `_diag_vacuum_debt` | 20% |
| `cmd_diagnose.sh` `_diag_bloat` | 30%（同一个命令内部两个不同阈值） |
| `cmd_perf.sh` `_perf_vacuum` | autovacuum 公式阈值（threshold + scale_factor * n_live_tup） |

同一件事——"这张表该不该 vacuum/膨胀严重不严重"——七处给出七个不同答案，DBA 拿不同命令查同一张表可能得到矛盾结论。**建议：** 在 `core.sh` 新增一个共享函数 `_bloat_query <schema.table或全库>`，统一膨胀比例计算和分级阈值（用现有 `KB_WARN_DEAD_TUPLES` 类似的可配置常量），所有命令改为调用它。autovacuum 公式阈值（`_perf_vacuum`）可以保留作为"是否会触发 autovacuum"的独立判断，但要在输出里明确标注这是不同维度，不是同一个"膨胀率"。

**索引检查——2 处逐字复制：** `cmd_idx.sh` 的 unused-index 查询（`idx_scan = 0 AND n_live_tup > 1000`）和 missing-FK-index 查询（`contype='f' AND n_live_tup > 10000`），与 `cmd_advisor.sh` `_advisor_index()` 里的对应 SQL 几乎逐字相同。**建议：** 直接删除 `advisor.sh` 里的重复 SQL，改为调用 `idx.sh` 的 `_idx_unused`/`_idx_missing` 函数——这正是 `diagnose.sh` 已经在用的模式（`_diag_indexes()` 调用 `_advisor_index --collect`，`_diag_params()` 调用 `_advisor_params --collect`），把这个"调用而不是复制"的模式补齐到 advisor→idx 这条链路上即可，改动量很小。

**等待事件分布——3 处独立实现：** `cmd_wait.sh`（顶层命令）、`cmd_perf.sh` `_perf_wait`、`cmd_stat.sh` 的等待事件小节，三份 SQL 基本是同一个 `GROUP BY wait_event_type, wait_event` 查询的变体。**建议：** 抽成 `core.sh` 或新建 `lib/_shared_wait.sh` 里的一个函数，三处调用。

**Top SQL（AWR 风格慢查询排行）——3 处独立实现：** `cmd_stmt.sh`（专职命令）、`cmd_diagnose.sh` `_diag_stmt_top`、`cmd_stat.sh` 的 Top SQL 小节。三处都查 `sys_stat_statements`，字段选择和排序逻辑高度相似。**建议：** 同上，抽成共享函数；`diagnose`/`stat` 调用 `stmt.sh` 暴露的一个 `_stmt_top --collect` 风格函数，而不是各自重新写 SQL。

**checkpoint/bgwriter 统计——2 处重叠：** `cmd_perf.sh` `_perf_wal` 和 `cmd_stat.sh` 的 checkpoint 小节都查 `sys_stat_bgwriter`，字段重叠但不完全一致。影响较小，可以合并成一个函数，两处调用不同的展示粒度。

**ANALYZE 陈旧度/drift 概念——2 处口径不同但公式相同：** `cmd_advisor.sh` `_advisor_analyze()`（全库扫描，找 `n_live_tup` 相对 `reltuples` 偏移 > 10% 或超过 7 天未 analyze 的表）和 `cmd_colstat.sh`（单表深挖时展示的 drift_pct）用的是同一个公式 `(n_live_tup - reltuples) * 100.0 / reltuples`，只是一个是全库巡检、一个是单表深挖。不算严重重复（服务场景不同），但建议把这个 drift 公式本身抽成 `core.sh` 里的一个共享 SQL 片段/函数，避免以后两处各自改出偏差。

**当前活动视图——3 处部分重叠：** `sessions`（非 idle 会话列表）、`sql`（无参数时的全会话汇总）、`wait -v` 的详细模式，三者对"当前正在跑什么"这件事有重叠但侧重点不同（sessions 偏会话列表，sql 偏 SQL 文本，wait 偏等待事件）。这个重叠是合理的"同一份活动数据、不同切面"，不建议合并，但建议在 `--help` 里加一句说明三者的区别，减少用户选择困难。

### 0.3 查询深度不够的命令

**`audit`（57 行，只有 4 项检查）：** superuser 角色列表、有登录权限但无密码的角色、public schema 上的 PUBLIC 写权限、无主键的表。这四项都是通用 PostgreSQL 级别的基础检查，**完全没有覆盖已经安装在测试环境里的 KingbaseES 企业级安全扩展**：`sysaudit`（审计策略是否启用、覆盖了哪些对象）、`sysmac`/`sys_anon`（强制访问控制/敏感列脱敏是否配置）。对于一个标榜支持 KingbaseES 特有能力的工具来说，`audit` 是当前查询深度最浅、同时又最该加深的命令——这个可以留到 Phase 2 再做，但值得在这里点出来，优先级应该提到 Phase 2 列表最前面。

**`conf`（39 行）：** 除 `diff` 子命令外，默认视图和 `params` 重复（见 0.1），本身没有独立的深度问题，是分类问题不是深度问题。

其余"查"层命令（`idx`、`colstat`、`obj`、`locks`、`stmt`）经通读代码确认深度是够的，暂无需要补强的地方。

### 0.4 一个已经做对的模式，应当推广

`diagnose.sh` 的 `_diag_indexes()` 调用 `_advisor_index --collect`、`_diag_params()` 调用 `_advisor_params --collect`——共享同一份查询逻辑，用 `--collect` 标志切换"直接展示"和"返回结果给调用者"两种模式，而不是复制 SQL。这是消除 0.2 里列出的重复实现的现成模板，直接套用到 advisor↔idx、diagnose/stat↔stmt、wait 相关的三个命令上即可，不需要发明新模式。

---

## 差距分层清单（按 看/查/断 三层架构归类，Phase 0 完成后再排期）

### Phase 1：灾备可信度（已实测到真实故障）

| 命令 | 层 | 内容 |
|------|-----|------|
| `kbdiag backup` (新) | 查 | `sys_stat_archiver` 归档成功/失败次数、最后归档时间、失败率；rman/pgBackRest 仓库最近一次全量/增量备份时间戳；RPO 是否超标（距上次成功备份 > 阈值报警） |
| `status`/`check` 加一行 | 看 | 归档失败时在快速体检里直接标红，不用等 DBA 单独去查 |
| `diagnose` 加一节 | 断 | 归档失败 → 关联到"WAL 目录增长速率"和"磁盘剩余空间"，给出根因链和建议 |

### Phase 2：安全合规深度（利用已装的企业扩展，`audit` 是当前查询深度最浅的命令，优先做）

| 命令 | 层 | 内容 | 状态 |
|------|-----|------|------|
| `audit` 扩展 | 查 | sysaudit 是否启用、审计规则数；sysmac 策略/enforcement 数；sys_anon 脱敏策略数；SSL 是否启用；sepapower 三权分立状态；密码过期策略（`VALID UNTIL`） | ✅ 已完成并推送（commit c72b5e3，2026-07-06） |
| `audit --report` (可选) | 查 | 解析 sysaudit 日志，输出近 N 天的异常登录/权限变更事件 | 未做——sysaudit 日志表对 DBA(system) 角色 permission-denied（三权分立架构限制，非配置问题），需要 sso/sao 角色才能读，暂不可行 |

**已验证的关键行为**（VM 实测，V8R6）：sysaudit 的规则/日志表在 C 函数层面对 `system` 超级用户硬性拒绝，与 GRANT/ACL、`sepapower.separate_power_grant` 开关状态均无关；sys_anon 的策略视图是 schema 级 ACL 拒绝（`permission denied for schema anon`）；两者的 permission-denied 都被 audit 命令报告为"合规"而非"检查失败"。sysmac 则可以被 `system` 直接查询。连接来源白名单（pg_hba 等价物）未纳入本轮，留待后续。

### Phase 3：性能深度（架构分叉已否决，收窄为单次深挖，不做趋势/回归）

| 命令 | 层 | 内容 |
|------|-----|------|
| `kbdiag explain <sql\|pid>` (新) | 查 | 捕获执行计划（`EXPLAIN (ANALYZE, BUFFERS)`），标注全表扫描/未走索引/估算行数偏差过大 |
| ~~`trend`~~ | — | **取消**（需要历史快照存储，与"纯无状态"架构冲突，已否决） |
| ~~计划回归对比~~ | — | **取消**（同上，需要历史快照） |

### Phase 4：运维自动化与集群深度

| 命令 | 层 | 内容 |
|------|-----|------|
| `audit` 或新 `jobs` 命令 | 查 | kdb_schedule 里失败/超时的作业 |
| `perf partition` 或新 `partition` 命令 | 查 | kdb_partman 分区健康：未来分区是否已预创建、维护任务是否失败、分区裁剪是否生效 |
| `cluster` 增强 | 断 | failover 演练检查清单（witness 节点、quorum、repmgr 配置一致性），而不只是当前 topology |

---

## 已确认决定

1. **架构分叉**：否决。不引入本地状态存储，保持纯实时查询；Phase 3 的 `trend`/计划回归能力取消。
2. **优先级**：不是"先做 Phase 1 新功能"，而是**先做 Phase 0 消重与分类审计**（本文档已完成，见上）。Phase 1/2/4 顺延到 Phase 0 之后，届时 Phase 2（`audit` 深度不足）优先于 Phase 1/4。
3. **CLAUDE.md 的 Lima SSH 端口**：已更新（改为 `limactl shell` 作为主要连接方式，注明端口动态分配、需要 `limactl list` 现查）。
