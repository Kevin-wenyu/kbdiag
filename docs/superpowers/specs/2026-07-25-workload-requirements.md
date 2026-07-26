# `kbdiag workload` 需求说明（2026-07-25，定稿）

## 背景

用户觉得 kbdiag 能力不如 `ora`/pg_profile/pgBadger/pgmetrics/pganalyze，用 `research` 技能跑了一次调研（`docs/superpowers/specs/2026-07-25-capability-gap-research.md`），最有价值的发现：KingbaseES 自带 AWR+ASH 等价的 `sys_kwr` 扩展（DB 侧自己维护快照仓库），以及一条更轻量的 fallback（`track_real_stats` GUC 打开后，`sys_stat_sysmetric_history` 视图开始滚动记录指标）。这两条数据都是 **KingbaseES 自己持久化的，不是 kbdiag 自己攒的状态**，能在不违反 kbdiag「无状态/单文件/纯 shell」架构的前提下，做出接近 `ora` 的历史工作负载报告能力。

`docs/superpowers/specs/2026-07-14-next-features-requirements.md` 第 15-16 行曾否决过"trend 类"命令，本次确认可以做——区分点是"状态由 KingbaseES 自己的扩展/视图维护，kbdiag 只读不存"，跟当时否决的"kbdiag 自己攒状态"不是一回事，不算推翻旧决定。

## 命令定位

新增查层（DBA 层）命令 `kbdiag workload`，不并入现有 `perf`——`perf` 是实时快照，`workload` 是区间 diff，输出契约不同。

领域术语已写入 `CONTEXT.md`：

> **Workload**：一段时间区间内的负载变化报告（等待事件、SQL 耗时、指标趋势的 diff），而非某一时刻的快照。底层可能来自 `sys_kwr` 扩展或 `sys_stat_sysmetric_history`，命令层的叫法与具体实现解耦。

避免叫 `history`（太泛，跟日志/慢 SQL 历史混淆）或 `kwr`/`awr`（绑死具体实现）。

## 数据源探测与降级

主库上，按顺序探测：

1. `sys_kwr` 扩展已装（`sys_extension` 里查得到）→ 用它，`perf.kwr_report(start_id, end_id, 'text')` 拿区间 diff 报告
2. `sys_kwr` 未装但在 `sys_available_extensions` 里存在 → **WARN**，提示"可选装 `sys_kwr` 获得更完整的历史工作负载报告"；kbdiag **不自己跑 `CREATE EXTENSION`**（改 schema 的权限操作，不该由诊断工具代劳）
3. `track_real_stats=on` → 退化到 `sys_stat_sysmetric_history`，粒度粗（CPU/等待类趋势），零安装成本
4. 都不满足 → **WARN**，提示怎么开（`ALTER SYSTEM SET track_real_stats = on; SELECT sys_reload_conf();`），同样不自动执行

降级顺序参照 `lib/cmd_stmt.sh:18-28` 处理 `sys_stat_statements` 未加载时的既有惯例（WARN + 提示，不是 FAIL），保持全仓库一致。

### standby 节点

`perf.create_snapshot()` 是写操作，standby 只读副本写不进去。`workload` 开头先探测节点角色，复用仓库已有惯用写法（`cmd_check.sh:46`、`cmd_diagnose.sh:243` 的 `pg_is_in_recovery()`/`sys_is_in_recovery()` + `tr -d '[:space:]'`）。

- standby 上，"可用快照不足两个时自动补一个"这条**永远不触发**，等价于强制生效 `--no-snapshot`。提示语区别于主库版本：「当前节点是备库（只读），无法自动生成快照；如需完整历史对比，请到主库执行 `kbdiag workload` 触发快照，或手动到主库执行 `perf.create_snapshot()`」
- `perf.kwr_report()`、`sys_stat_sysmetric_history` 的**读取**在 standby 上没有问题——repmgr 走物理流复制，相关表随 WAL 复制到备库，只是备库自己不能再写新快照
- 步骤 4 的 `ALTER SYSTEM SET track_real_stats = on` 提示同理是写操作、且改的是要在主库生效的参数：standby 上该提示要改成「请到主库执行该命令」，不能原样给一条备库执行不了的命令

对齐 CLAUDE.md「Single-node + HA：每个检查都要在 repmgr 缺失/只读场景下优雅降级」的红线，不是新原则。

## 默认时间区间

不传 `--from/--to` 时：

- `sys_kwr` 路径：最近两个可用快照（不管间隔多长，由 DBA 自己配的采集频率决定分辨率）
- `sys_stat_sysmetric_history` 路径：固定最近 15 分钟（本来就是轻量 fallback，没有离散快照点的概念）

可用快照 < 2 个时（且未走 `--no-snapshot`、且不在 standby 上）：`workload` 自动调一次 `perf.create_snapshot()` 补数据，`verdict` 走 **WARN**，`data` 里附一句"当前区间基于本次自动生成的快照，历史对比数据不足，建议之后再跑一次"。不新增任何默认后台调度（不擅自建 cron job）。

## CLI 参数

| flag | 说明 |
|---|---|
| `--from <dur>` | 区间起点，相对时长，语法照抄 `lib/cmd_logs.sh:21-22,54-59` 的 `--since`（`1h`/`30m`/`1h30m`，"N 之前"），复用其"正则拆 h/m 累加分钟数 → `date -d`/`date -v` 双兼容转绝对 cutoff"逻辑，不新写解析器 |
| `--to <dur>` | 区间终点，语法同上；省略时默认 `now` |
| `--no-snapshot` | 关闭"不足两个快照时自动补一个"的行为，退化成"没数据就直接 WARN 提示手动执行"。standby 上该行为本来就强制关闭，此 flag 在 standby 上是空操作（不报错） |

不新增：

- `--json` —— 全仓库统一用全局 `--format json`（`core.sh:128`），复用即可
- `--exit-code` —— 已是全局 flag（`core.sh:123`），复用即可
- `--database` —— 本轮不做多库切分，`kwr_report()` 的 `database` 参数按其默认值（整实例）传，不开放成 flag；如后续需要单库范围报告，另起需求

参数校验（usage error，`_exit=2`，对齐 `cmd_check.sh:12` 的 `0=OK/1=WARN/2=FAIL` 惯例）：

- 只给 `--to` 不给 `--from`：报错「`--to` 必须搭配 `--from` 使用」
- 换算后起点比终点新（如 `--from 30m --to 2h`）：报错「`--from` 必须早于 `--to`」

## 输出契约

### verdict / exit code

沿用仓库既有的 `_exit=0/1/2`（OK/WARN/FAIL）惯例（见 `cmd_check.sh:12`），`--exit-code` 时才把它当进程退出码用，默认恒 0（保证 `all` 命令里 `cmd_workload || true` 式的组合不中断，见 CLAUDE.md「KingbaseES 特有行为」一节）。

- **OK（0）**：报告从真实数据源生成成功（`sys_kwr` 有 ≥2 快照，或 `sys_stat_sysmetric_history` 有数据），未触发任何降级分支
- **WARN（1）**：报告生成成功，但走了降级路径——扩展未装/GUC 未开的提示分支、自动补快照分支、standby 强制 `--no-snapshot` 分支
- **FAIL（2）**：`--from/--to` 参数校验失败，或数据源查询本身报错（非"无数据"情形，比如 `kwr_report()` 调用抛异常）

`verdict` **只反映"报告生成成功与否 / 走了哪条数据源"**，不试图从报告内容里判断严重程度（等待事件占比高不高之类的解读留给看报告的人）。

### data 字段

`perf.kwr_report(1, 2, 'text')` 实测输出是一份 904 行的纯文本 AWR 风格报告（中文，等待事件/DB Time/Top SQL 全在里面）。`data.report` 直接存原始文本，不拆解成结构化字段：

- `ora` 自己对 Oracle AWR/ASH 报告也是这么处理的——原样呈现，不重新解析内容
- 报告字段/格式跟着 KingbaseES 版本走，今天解析明天可能就碎
- 报告本身已经是 KingbaseES 官方算好的成品，kbdiag 的价值在"判断该不该生成、怎么生成"，不在于重新排版

`sys_stat_sysmetric_history` 路径没有预渲染的报告文本（这个视图本身就是结构化行数据），按同一原则（"数据库给你什么形状就原样带出什么形状，不重新发明"），用 `data.metrics` 存原始行（`{metric, ts, value}` 数组），不套用 `data.report` 的字符串形状——两条路径的 `data` 字段形状不统一，是有意为之。

## 明确不做

- 不做 pg_profile 式 kbdiag 自建的有状态指标仓库
- 不做 pgBadger 式富 HTML 报告渲染
- 不新增任何默认开启的后台采集/调度（cron、systemd timer 等）——所有数据采集要么是 KingbaseES 自己在做，要么是 `workload` 调用时的一次性动作
- 不做多库切分（`--database` flag）
- 不做 HypoPG 式 what-if 索引建议（调研文档 shortlist 里的独立项，跟 `workload` 无关，另起需求）
- 不做 `logs` 的直方图增强、`snapshot` 的 JSON 收敛（调研文档 shortlist 剩余两项，同上，另起需求）

## 测试夹具

2026-07-26 上机复核，纠正一处此前的假设错误：`kes-node2` 是 `kes-node1` 的物理流复制 standby，`sys_kwr` 一旦在主库装上，扩展本身及 `perf.kwr_snapshots` 数据都会随 WAL 复制到备库——`kes-node2` 并不处于"未装"状态，读到的是和主库一样的 snapshot 1/2。真正节点本地化的只有 `track_real_stats`（主库 `on`、备库 `off`），但探测顺序里 `sys_kwr` 已装的优先级更高，这个差异永远不会在备库上被走到。

`kes-node1`：已开 `track_real_stats=on`，已装 `sys_kwr` 并有 snapshot 1/2——覆盖"已开启/已装"分支。
`kes-node2`：`sys_kwr` 同样已装（复制而来），`track_real_stats=off`——不覆盖"未装/未开启"分支，只用来覆盖 standby 角色下 Q5 的强制 `--no-snapshot` 行为（读 `kwr_report()` 本身仍正常）。

"扩展可选装但未装"和"两条数据源都不满足"这两条 WARN 分支，在当前两节点集群里没有真实夹具可测——`sys_kwr` 装上后无法在不破坏 `kes-node1` 夹具的前提下构造出"未装"的节点。2026-07-26 决定：不为此单开第三个实例，这两条分支只做代码走查，不进黑盒回归套件，测试里按 `test_logs.sh` 已有的"夹具缺失 → 优雅 `_pass`"惯例写空跑桩函数。

## 仍未定（不阻塞 `/to-spec`，实现时顺手定）

- `kddm_*` advisor 函数族（`kddm_index_advisor`/`kddm_guc_advisor`/`kddm_checkpoint_advisor` 等，随 `sys_kwr` 扩展一起装）跟 `workload` 无关，但可能是现有 `advisor` 命令（断层）的证据源升级点——具体返回格式没探过，留到 `workload` 落地后单独起一轮调研
