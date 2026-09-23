# kbdiag 2.0 查询能力清单（草案）

**状态**：草案，待用户增删和排优先级
**日期**：2026-09-23
**参考**：`ora`（`~/Documents/oracle/ora:100-320`）、pgmetrics、pgBadger、pg_profile、pganalyze（调研见 `docs/superpowers/specs/2026-07-25-capability-gap-research.md`）；digoal/skills（https://github.com/digoal/skills/tree/main/postgresql ，下文简称 **digoal**）；KingbaseES V8 官方文档（https://help.kingbase.com.cn/v8/ ，下文简称 **KES 文档**）
**SQL 来源纪律**：参考来源里的 SQL 只拿来参考，都要按文末"SQL 引入规则"在 VM 上验证过才能进代码
**原则**：先做简单的单次查询（看一眼就知道），连环分析放在后面（见 `00-requirements-discovery.md`）。命令名一栏只是暂定名，方便讨论，形态在阶段 1 试用后再定。

**优先级**：**P1** = v0.1 候选；**P2** = v0.2；**P3** = 以后再说

---

## A. 实例与配置

| #   | 暂定名                | 回答的问题                                                                                                            | 参考来源                    | KES 数据源                                                                         | 优先级 |
| --- | ------------------ | ---------------------------------------------------------------------------------------------------------------- | ----------------------- | ------------------------------------------------------------------------------- | --- |
| A1  | `status`           | 版本、角色（主/备/单机）、启动时长、连接数/上限、库列表和大小、数据目录                                                                            | ora `version`、pgmetrics | `version()`、`sys_is_in_recovery()`、`sys_postmaster_start_time()`、`sys_database` | P1  |
| A2  | `params [pattern]` | 参数当前值、来源（默认/配置文件/ALTER SYSTEM）、是否待重启生效；可只看非默认值                                                                   | ora `params`            | `sys_settings`（`source`、`pending_restart`）                                      | P1  |
| A3  | `ext`              | 装了哪些扩展、哪些在 `shared_preload_libraries` 里                                                                          | pgmetrics               | `sys_extension`、`sys_available_extensions`                                      | P2  |
| A4  | `license`          | License 类型、到期时间、剩余天数                                                                                             | —（KES 特有）               | `get_license_validdays()` 类函数（待核实）                                              | P2  |
| A5  | `ready`            | 可观测性就绪：KWR/KSH、`log_checkpoints`、`log_lock_waits`、`log_min_duration_statement`、`track_io_timing` 开没开——出事时能不能拿到证据 | 案例 1 的教训                | `sys_settings`                                                                  | P2  |

## B. 会话与连接

| #   | 暂定名             | 回答的问题                                                                                         | 参考来源                                   | KES 数据源                                                   | 优先级 |
| --- | --------------- | --------------------------------------------------------------------------------------------- | -------------------------------------- | --------------------------------------------------------- | --- |
| B1  | `sessions`      | 当前会话列表，按状态/用户/应用/库/客户端过滤；标出长时间运行和 idle in transaction                                         | ora `sessions`、pgmetrics               | `sys_stat_activity`                                       | P1  |
| B2  | `session <pid>` | 单会话详情：完整 SQL、事务开始时间、当前等待、持有哪些锁、是否被人挡住、客户端地址                                                   | ora `fulltext`、`last_sql_hash`         | `sys_stat_activity` + `sys_locks` + `sys_blocking_pids()` | P1  |
| B3  | `conn`          | 连接数分布：按用户/应用/库/客户端 IP/状态汇总，离上限还有多远                                                            | pgBadger（连接统计）、pgmetrics               | `sys_stat_activity` 聚合                                    | P1  |
| B4  | `txn`           | 长事务、idle in transaction、最老的 `backend_xmin` 是谁；**以及未结束的两阶段提交（2PC）事务**，它们同样会压住视界，而且没有对应会话，容易被漏掉 | pgmetrics、digoal `pg-bloat-root-cause` | `sys_stat_activity`、`sys_prepared_xacts`                  | P1  |

## C. 锁

| #   | 暂定名                 | 回答的问题                    | 参考来源                            | KES 数据源                             | 优先级          |
| --- | ------------------- | ------------------------ | ------------------------------- | ----------------------------------- | ------------ |
| C1  | `locks`             | 谁在等锁、被谁直接挡住、等了多久、锁的对象和模式 | ora `wait_txlock`、`hold_txlock` | `sys_locks` + `sys_blocking_pids()` | P1           |
| C2  | `locks --table <t>` | 某张表上现在有哪些锁               | pgmetrics                       | `sys_locks` 按 relation 过滤           | P2           |
| C3  | `locks --tree`      | 多级阻塞链，一直追到源头             | ora `hold/wait_txlock` 组合       | 递归 `sys_blocking_pids()`            | P2（连环分析的第一步） |

## D. SQL

| #   | 暂定名                                      | 回答的问题                                                 | 参考来源                                          | KES 数据源                                              | 优先级 |
| --- | ---------------------------------------- | ----------------------------------------------------- | --------------------------------------------- | ---------------------------------------------------- | --- |
| D1  | `running`                                | 正在执行的 SQL，按已运行时长排序                                    | ora `execute`                                 | `sys_stat_activity`（`state='active'`）                | P1  |
| D2  | `top [--by time\|calls\|io\|temp\|rows]` | 累计 Top SQL（**明确标注是累计值及统计起点**）                         | ora `top_*`、pg_profile                        | `sys_stat_statements`                                | P1  |
| D2b | `top --interval 60s`                     | **最近** N 秒的 Top SQL：采两次样取差值，只看当下；如果两次采样之间统计被重置过，就拒绝计算 | digoal `pg-top-sql-analyze`、`pg-perf-insight` | 同上                                                   | P2  |
| D3  | `sqlstat <queryid>`                      | 单条 SQL 的执行统计：次数、平均/最大耗时、IO、临时文件                       | ora `sqlstats`                                | `sys_stat_statements`                                | P2  |
| D4  | `plan <queryid\|sql>`                    | 执行计划（只 EXPLAIN，不执行；带 ANALYZE 需显式参数）                   | ora `plan`/`eplan`                            | `EXPLAIN`                                            | P2  |
| D5  | `progress`                               | 正在进行的 VACUUM / CREATE INDEX / ANALYZE / basebackup 进度 | ora `longops`                                 | `sys_stat_progress_*`                                | P2  |
| D6  | `temp`                                   | 谁在用临时文件、用了多少                                          | ora `sql_use_temp_segment`、`tempu`            | `sys_stat_database.temp_bytes`、`sys_stat_statements` | P2  |

## E. 等待与负载

| #   | 暂定名        | 回答的问题                           | 参考来源                    | KES 数据源                                             | 优先级 |
| --- | ---------- | ------------------------------- | ----------------------- | --------------------------------------------------- | --- |
| E1  | `waits`    | 此刻各会话在等什么，按等待事件汇总               | ora `events`            | `sys_stat_activity`（`wait_event_type`/`wait_event`） | P1  |
| E2  | `ash [分钟]` | 过去 N 分钟的活跃会话历史（按时间/等待事件/SQL 汇总） | ora `ash`、`ash_sql`     | KSH（`sys_kwr.collect_ksh` 开启时），否则说明缺失               | P2  |
| E3  | `host`     | 主机 CPU/内存/负载/磁盘 IO（仅本机运行时）      | pgmetrics（system stats） | `/proc`                                             | P2  |

## F. 空间与对象

| #   | 暂定名                       | 回答的问题                                                                                                           | 参考来源                                                         | KES 数据源                                              | 优先级 |
| --- | ------------------------- | --------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------ | ---------------------------------------------------- | --- |
| F1  | `space`                   | 库大小、表空间大小、磁盘剩余、WAL 目录大小                                                                                         | ora `space`                                                  | `pg_database_size`、`sys_tablespace`、`statfs`         | P1  |
| F2  | `top-objects`             | 最大的表/索引 Top N（含 TOAST）                                                                                          | ora `segsize`                                                | `pg_total_relation_size`                             | P1  |
| F3  | `table <t>`               | 单表概况：大小、行数估计、死元组、最近 vacuum/analyze、索引列表、年龄                                                                      | ora `tabstats`、`optstats`、`idxdesc`                          | `sys_stat_user_tables`、`sys_class`、`sys_index`       | P1  |
| F4  | `colstats <t>`            | 列统计：NULL 比例、distinct、相关性                                                                                        | ora `colstats`                                               | `sys_stats`                                          | P3  |
| F5  | `bloat`                   | 表/索引膨胀：默认用估算（快）；加 `--exact <t>` 对单表用 kbstattuple 精确测量（会扫全表，需显式指定）                                               | ora `tab_frag`/`index_frag`、pgmetrics、digoal `pg-find-bloat` | 估算 SQL；KES 文档里的 `kbstattuple` 扩展                     | P2  |
| F6  | `indexes --unused\|--dup` | 从未使用、重复的索引。要把 digoal 列出的误判点写进输出：统计起点（`stats_reset`、重启时间）；**备库上的扫描不计入主库**；支撑外键的索引不建议删；分区表要把所有分区加起来看；账号权限不足时结果会不全 | pganalyze、pg_profile、digoal `pg-find-unused-index`           | `sys_stat_user_indexes`、`sys_index`、`sys_constraint` | P2  |
| F7  | `seq`                     | 快用完的序列：剩余可调用次数；int/smallint 类型的序列单独标出                                                                           | pgmetrics、digoal `pg-runtime-risk`                           | `sys_sequences`                                      | P2  |
| F8  | `partitions <t>`          | 分区表的分区列表和大小                                                                                                     | —                                                            | `sys_inherits`、`kdb_partman`                         | P3  |

## G. 维护（vacuum / 冻结）

| #   | 暂定名                | 回答的问题                                                                       | 参考来源                     | KES 数据源                                                           | 优先级 |
| --- | ------------------ | --------------------------------------------------------------------------- | ------------------------ | ----------------------------------------------------------------- | --- |
| G1  | `vacuum`           | 死元组最多的表、最近 autovacuum 时间、当前在跑的 autovacuum worker                            | pgmetrics、pgBadger       | `sys_stat_user_tables`、`sys_stat_activity`                        | P1  |
| G2  | `freeze`           | 库级、表级事务年龄，离回卷还有多远                                                           | pgmetrics                | `age(datfrozenxid)`、`age(relfrozenxid)`                           | P1  |
| G2b | `freeze --buckets` | 冻结风暴风险：按年龄分桶，看有多少表、多大体量会在相近时间一起触发 freeze                                    | digoal `pg-runtime-risk` | 同上 + `pg_total_relation_size`                                     | P2  |
| G3  | `stats-age`        | 统计信息是否过时：自上次 analyze 以来的修改量占触发阈值的百分比；从未 analyze 的表；表级 reloptions 覆盖了全局阈值的情况 | digoal `pg-runtime-risk` | `sys_stat_user_tables.n_mod_since_analyze`、`sys_class.reloptions` | P2  |

## H. WAL / 检查点 / 归档

| #   | 暂定名          | 回答的问题                                                           | 参考来源                | KES 数据源                                         | 优先级 |
| --- | ------------ | --------------------------------------------------------------- | ------------------- | ----------------------------------------------- | --- |
| H1  | `wal`        | WAL 生成速率（两次采样差值）、`sys_wal` 目录大小、谁在保留 WAL（槽/`wal_keep_segments`） | pgmetrics           | `sys_current_wal_lsn()`、`sys_replication_slots` | P2  |
| H2  | `archive`    | 归档是否正常：失败次数、最后成功/失败时间                                           | pgmetrics           | `sys_stat_archiver`                             | P1  |
| H3  | `checkpoint` | 检查点频率、定时 vs 请求触发比例、bgwriter 统计                                  | pgBadger、pg_profile | `sys_stat_bgwriter`                             | P2  |

## I. 复制与集群

| #   | 暂定名       | 回答的问题                                                                      | 参考来源                     | KES 数据源                                                    | 优先级                         |
| --- | --------- | -------------------------------------------------------------------------- | ------------------------ | ---------------------------------------------------------- | --------------------------- |
| I1  | `repl`    | 主库视角：备库列表、各阶段延迟（write/flush/replay）、同步/异步                                  | pgmetrics                | `sys_stat_replication`                                     | P1                          |
| I2  | `slots`   | 复制槽：是否活跃、保留了多少 WAL、`xmin` 是否压着视界                                           | pgmetrics                | `sys_replication_slots`                                    | P1                          |
| I3  | `cluster` | repmgr 集群视图：节点、角色、上游、状态                                                    | —（KES/repmgr 特有）         | repmgr 元数据表                                                | P1                          |
| I5  | `ha`      | 单点风险：同步备库数量、`synchronous_standby_names`、`synchronous_commit` 是否是 off/local | digoal `pg-runtime-risk` | `sys_stat_replication.sync_state`、`sys_settings`           | P2                          |
| I4  | 备库视角      | 接收状态、回放延迟、是否暂停回放                                                           | pgmetrics                | `sys_stat_wal_receiver`、`sys_last_xact_replay_timestamp()` | P1（并入 `repl`，在备库上运行时自动切换视角） |

## J. 历史（依赖 KWR/KSH，不开就明说）

| #   | 暂定名                    | 回答的问题                                           | 参考来源                                        | KES 数据源                                                                            | 优先级 |
| --- | ---------------------- | ----------------------------------------------- | ------------------------------------------- | ---------------------------------------------------------------------------------- | --- |
| J1  | `kwr snaps`            | 有哪些快照、时间范围                                      | ora `snap`、`awr_all_snap`                   | `perf.kwr_snapshots`                                                               | P2  |
| J2  | `kwr report --at <时间>` | 自动找覆盖某时间点的快照区间，生成 KWR 报告                        | ora `awr_*`、pg_profile                      | `perf.kwr_report()`                                                                | P2  |
| J3  | `kwr top --at <时间>`    | 某时间窗内的 Top SQL / Top 等待                         | ora `awr_sql_elaps_time`、`awr_top10_events` | `sys_kwr` 内部表（需调研）                                                                 | P3  |
| J4  | `ksh --at <时间>`        | 某时点前后每秒的会话采样：谁在跑、在等什么。开了 KSH 才有；没开就明说，并告诉用户怎么打开 | ora `ash`、KES 文档                            | KES 文档：`perf.session_history`（内存环形缓冲区）、`perf.ksh_history`（落库）、`perf.ksh_report*()` | P2  |

KES 文档确认过的接口（sys_kwr 1.8，还要在 VM 上核对）：

- `perf.kwr_snapshots`
- `perf.create_snapshot()`
- `perf.kwr_report(start_id, end_id, format, database)`
- `perf.ksh_report(start_ts, duration, slot_width, database)`
- `perf.ksh_report_by_snapshots(...)`（1.6 起支持）

参数：`sys_kwr.interval` 默认 60 分钟，`sys_kwr.history_days` 默认 8 天，`sys_kwr.collect_ksh` 默认 off。KSH 每秒采样一次；官方文档原话是开启"会有一定的性能损耗"。

## K. 日志

| #   | 暂定名                                        | 回答的问题                          | 参考来源                            | KES 数据源           | 优先级 |
| --- | ------------------------------------------ | ------------------------------ | ------------------------------- | ----------------- | --- |
| K1  | `log --since <时间>`                         | 某时间窗内的 ERROR/FATAL/PANIC，按类型汇总 | pgBadger、pganalyze Log Insights | `sys_log` 目录（仅本机） | P2  |
| K2  | `log --slow` / `--locks` / `--checkpoints` | 日志里的慢 SQL、锁等待、检查点记录            | pgBadger                        | 同上                | P2  |

## L. 作业与安全

| #   | 暂定名     | 回答的问题                          | 参考来源                                 | KES 数据源                         | 优先级 |
| --- | ------- | ------------------------------ | ------------------------------------ | ------------------------------- | --- |
| L1  | `jobs`  | `kdb_schedule` 作业状态、最近失败       | —（KES 特有）                            | `kdb_schedule` 表                | P3  |
| L2  | `users` | 用户/角色、超级用户列表、密码过期；应用是否在用超级用户连接 | pgmetrics、digoal `pg-security-audit` | `sys_roles`、`sys_stat_activity` | P3  |
| L3  | `lo`    | 大对象占用空间                        | digoal `pg-runtime-risk`             | `sys_largeobject_metadata`      | P3  |

**不收录**（属于审计/建议类，不是查询）：digoal 的 `pg-design-audit`（表结构设计审查）、`pg-sql-audit`（上线前 SQL 审查）、`pg-parameter-tuning-advisor`（参数调优建议）。这几项留到"断"层或以后再考虑。

**合计**：51 项，另有下文"KES 原生统计视图"一节新增的 7 项，一共 58 项；其中 P1 共 20 项（I4 并入 `repl`，实际是 19 条命令）。digoal 带来的新增项都排在 P2/P3，v0.1 范围不变；只有 B4 的范围扩大，加上了 2PC 检查。

---

## kbdiag 自己的特点（参考工具都没有，或做得不够）

1. **原生懂 KingbaseES**：`sys_` 视图、Oracle 兼容模式下的语义差异、`.s.KINGBASE` socket、repmgr、`kdb_schedule`、License、`sys_kwr`/KSH。参考工具都是为 PostgreSQL 或 Oracle 做的，拿到 KES 上要么跑不通，要么结果不准。
2. **零安装**：单个二进制文件，拷过去就能跑，备库上也能跑。ora 依赖 sqlplus，pgBadger 依赖 Perl，pg_profile 要装扩展，pganalyze 要装 agent 和 SaaS。
3. **诚实的输出**：
   - 查不了的写明原因（权限、备库、远程、扩展没开），不静默跳过
   - 累计值和当前值分开标注
   - 截断的写明"还有 M 条没显示"
   - 查到空结果时，区分"真的没有"和"开关没开"（`track_sql`、`collect_ksh` 等），并指出具体是哪个开关
4. **查询之间有"下一步"指引**：简单查询不做关联分析，但会在结果里指出下一步该查什么。例如 `sessions` 里某个会话被挡住，就提示"`kbdiag locks` 查看阻塞者"。这样保持了简单，又不孤立，也为以后的连环分析铺好了路。
5. **机器可读、可接监控**：所有命令统一 JSON 结构，退出码采用 Nagios 约定，可以直接挂到监控或巡检脚本里。
6. **中文输出**：面向国内 DBA，结果默认中文，帮助信息用英文。

## KES 原生统计视图（来自 digoal/kingbase，2026-09-23 在 VM 上实测）

digoal 的 `kingbase/` 技能大部分是把 PG 版逐个移植过来，阈值原文就写着"与 PG 版完全一致"，而且他是在 **V9R1C10** 上验证的，我们的 VM 是 V8R6C9。真正有价值的是他记下的 KES 特有内容。下面这些视图在我们的 V8R6 VM 上**都存在**：

| 视图                                                                     | 内容                                       | VM 行数          | 开关（GUC）                             |
| ---------------------------------------------------------------------- | ---------------------------------------- | -------------- | ----------------------------------- |
| `sys_stat_sql`、`sys_stat_sqltime`/`sqlio`/`sqlwait`/`sqlcount`         | SQL 画像，db_time/cpu/wait 分开计，含 top 等待事件   | 0              | `track_sql`（VM 上是 off）              |
| `sys_stat_wait`、`sys_stat_waitaccum`                                   | 实例级累计等待事件（相当于 Oracle 的 `V$SYSTEM_EVENT`） | 0              | `track_sql`（digoal 说法，待核实）          |
| `sys_stat_dbtime`、`sys_stat_dmlcount`                                  | DB Time 总量、DML 分类计数                      | 0              | `track_sql`                         |
| `sys_stat_instevent`/`instlock`/`instio`、`sys_stat_msgaccum`           | 实例级事件/锁/IO 聚合                            | instevent 18 行 | `track_instance`（VM 上是 off 却有数据，待查） |
| `sys_stat_metric`、`sys_stat_metric_history`                            | 指标时序（相当于 `V$SYSMETRIC_HISTORY`）          | 322 行          | 待查（KWR 没开也有数据）                      |
| `sys_stat_wal_buffer`                                                  | WAL buffer 写盘状况                          | 1              | —                                   |
| `sys_stat_transaction`、`sys_stat_shmem`、`sys_stat_progress_checkpoint` | 事务、共享内存、检查点进度                            | 有数据            | —                                   |
| `perf.kwr_snap_idx_suggest`                                            | **KWR 自带的索引建议**                          | —              | `sys_kwr.enable`                    |

VM 上还装了这些扩展：

- `sys_freespacemap`（查看 FSM，和案例 1 直接相关）
- `walminer`（WAL 解析）
- `sys_hm`（健康检查，目前只看到数据文件检查两项）
- `kdb_partman`、`kdb_schedule`

**由此在清单里新增的项**：

| #   | 暂定名                | 回答的问题                                                                       | 数据源                                  | 优先级      |
| --- | ------------------ | --------------------------------------------------------------------------- | ------------------------------------ | -------- |
| E4  | `waits --system`   | 实例级累计等待分布；采两次样取差值，就能看到"最近 N 秒系统在等什么"。**不依赖 KSH**                            | `sys_stat_wait`、`sys_stat_instevent` | P2       |
| D7  | `top --native`     | 用 KES 原生 SQL 画像（db_time 拆成 CPU 和等待）做 Top SQL，比 `sys_stat_statements` 多了等待维度 | `sys_stat_sql*`                      | P2       |
| J5  | `metric [--since]` | 系统指标时序（QPS/TPS/命中率等）                                                        | `sys_stat_metric_history`            | P2       |
| J6  | `kwr idx-suggest`  | 读 KWR 已经算好的索引建议                                                             | `perf.kwr_snap_idx_suggest`          | P3       |
| F9  | `fsm <index>`      | 索引/表的 FSM 空闲页分布（案例 1 取证用）                                                   | `sys_freespacemap`                   | P3       |
| H4  | `walminer` 封装      | 按 LSN/时间范围解析 WAL                                                            | `walminer`                           | P3       |
| L4  | `hm`               | 调用 `sys_hm` 做数据文件检查                                                         | `sys_hm`                             | P3，需要先调研 |

**由此得出的设计原则：感知开关**。KES 的很多视图是否有数据，取决于 `track_sql`、`track_instance`、`sys_kwr.enable`、`sys_kwr.collect_ksh`、`log_*` 这些开关。kbdiag 查到空结果时，**必须区分"真的没有"和"开关没开"**，并报出具体是哪个开关。每个 probe 都要登记自己依赖的开关，A5 `ready` 就是把这张表完整输出一遍。

**KES 语义坑（digoal 实测得出；标了"待核实"的要在我们 V8R6 上确认）**：

- `round(double precision, int)` 没有这个重载，需要显式 `::numeric`（待核实）
- 装了 `sys_squeeze` 后会出现它自己的逻辑复制槽，不能当孤儿槽报警
- 默认库是 `kingbase`，没有 `postgres` 库
- `sys_monitor` 等价于 `pg_monitor`
- 统计无用索引、膨胀时，要排除内置 schema：`sys_catalog`、`sys_hm`、`sysaudit`、`sysmac`、`src_restrict`、`xlog_record_read`、`anon`、`dbms_job`、`dbms_scheduler`、`kdb_schedule`、`perf`
- 二级分区要查 `sys_catalog.sys_subpartition_table`，`sys_partitioned_table` 里只有一级分区
- PG12 内核里对应的参数是 `wal_keep_segments`，没有 `wal_keep_size`
- 连接池可能是 KBProxy
- digoal 说 V9 上 `max_prepared_transactions` 默认是 0；我们 VM 上是 100
- 我们 VM 上 `log_min_duration_statement=1000`，`log_checkpoints` 和 `log_lock_waits` 都是 on；digoal 说默认都是关的。**客户环境不能假设这些已经打开**

## SQL 引入规则（参考来源 → 代码）

参考来源里的 SQL（digoal、ora、pgmetrics、KES 文档示例）**不能直接照抄**。每条都要过下面几关：

1. **改写成 KES 语义**：
   - 视图改用 `sys_` 前缀
   - 布尔值是 `true`/`false`
   - Oracle 模式下 `NULL || 'x'` 的结果是 `'x'`，空串会被当成 NULL（`ora_input_emptystr_isnull=on`）
   - `COALESCE`/`NULLIF` 做兜底的地方逐个检查
   - 本机时间格式、`IntervalStyle=kingbase` 会影响时长的输出
2. **核对 KES 文档**：确认视图/函数在 V8R6 上存在、列名相同（比如 `sys_prepared_xacts`、`sys_stat_statements` 的列名）；有差异就记下来
3. **在 VM 上实跑**：先在 Lima 集群上跑通，再用故障注入造出目标状态（L3 取值断言 + L4 场景测试），确认数值正确，而不是"能跑出结果"就算过
4. **记录出处**：probe 代码注释里写明来源（URL/文件），以及 VM 验证日期

## 默认阈值参考（来自 digoal `pg-runtime-risk/references/thresholds.md`，还要按 KES 默认参数校准）

| 指标                         | 关注             | 警告           | 严重                                                            |
| -------------------------- | -------------- | ------------ | ------------------------------------------------------------- |
| 连接使用率                      | > 65%          | > 80%        | > 90%，或已逼近 `max_connections - superuser_reserved_connections` |
| idle in transaction 占连接数比例 | —              | > 20%        | —                                                             |
| 物理复制回放延迟                   | > 1 分钟 / 100MB | > 5 分钟 / 1GB | > 30 分钟 / 10GB                                                |
| 复制槽未激活                     | —              | —            | `active=false`                                                |
| 库年龄 `age(datfrozenxid)`    | > 10 亿         | > 15 亿       | > 20 亿                                                        |
| 序列剩余可调用次数                  | < 10 万         | < 1 万        | < 1000                                                        |
| 2PC 事务存在时长                 | —              | —            | > 15 分钟                                                       |

归档检查要先排除主动配置，再判断异常：`archive_mode=off`、`archive_command` 为空或是 `/bin/true`、还在 `archive_timeout` 窗口内。这一条直接吸收进 H2。

## digoal 的"诊断输出契约"（留给断层参考）

digoal 所有诊断类技能都遵循同一份输出契约（`_shared/diagnostic-output-contract.md`），必须包含 10 个部分：结论、推演过程、证据链、证据权威性、适用边界、前提条件、前提变化后的新结论、置信度、分级建议动作、未覆盖的点。

- **对 v0.1 简单查询来说太重**，不采用。
- **以后做"断"层时可以借鉴**，尤其是"证据权威性分级"（实测、推断、用户描述）和"前提不成立时结论怎么变"这两点，和我们"不假装 OK"的原则一致。

## 需要你决定

1. 清单增删：有没有你常查、这里漏掉的
2. P1 这 19 条命令作为 v0.1 是否合适（我的判断：19 条偏多，但每项都是简单查询，平均每项工作量小；真正的成本在测试。如果想更快交付，可以先砍掉 F2、D2、H2 这类可以稍后补的）
