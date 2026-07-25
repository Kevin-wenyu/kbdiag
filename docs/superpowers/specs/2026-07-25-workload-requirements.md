# `kbdiag workload` 需求说明（2026-07-25）

## 背景

用户觉得 kbdiag 能力不如 `ora`/pg_profile/pgBadger/pgmetrics/pganalyze，用 `research` 技能跑了一次调研（`docs/superpowers/specs/2026-07-25-capability-gap-research.md`），最有价值的发现是：KingbaseES 自带 AWR+ASH 等价的 `sys_kwr` 扩展（DB 侧自己维护快照仓库），以及一条更轻量的 fallback（`track_real_stats` GUC 打开后，`sys_stat_sysmetric_history` 视图开始滚动记录指标）。因为这两条数据都是 **KingbaseES 自己持久化的，不是 kbdiag 自己攒的状态**，理论上能在不违反 kbdiag "无状态/单文件/纯 shell" 架构的前提下，做出接近 `ora` 的历史工作负载报告能力。

**✅ 已确认（2026-07-25）**：`docs/superpowers/specs/2026-07-14-next-features-requirements.md` 第 15-16 行的"trend 已否决"红线，用户确认可以做——区分点是"状态由 KingbaseES 自己的扩展/视图维护，kbdiag 只读不存"，跟当时否决的"kbdiag 自己攒状态"不是一回事，不算推翻旧决定，是同一条红线下两种不同的情况。

## 已通过 `/grill-with-docs` 确认的决定

### 1. 命令名与领域术语：`workload`

新增查层命令 `kbdiag workload`，不并入现有 `perf`（`perf` 是实时快照，`workload` 是区间 diff，输出契约不同，混在一起会让 `perf` 变复杂）。

术语已写入 `CONTEXT.md`：

> **Workload**：一段时间区间内的负载变化报告（等待事件、SQL 耗时、指标趋势的 diff），而非某一时刻的快照。底层可能来自 `sys_kwr` 扩展或 `sys_stat_sysmetric_history`，命令层的叫法与具体实现解耦。

避免叫 `history`（太泛，跟日志/慢 SQL 历史混淆）或 `kwr`/`awr`（绑死具体实现）。

### 2. 数据源探测与降级顺序

1. `sys_kwr` 扩展已装（`sys_extension` 里查得到）→ 用它，`perf.kwr_report(start_id, end_id, 'text')` 拿区间 diff 报告
2. `sys_kwr` 未装但在 `sys_available_extensions` 里存在 → WARN + 提示"可选装 `sys_kwr` 获得更完整的历史工作负载报告"，kbdiag **不自己跑 `CREATE EXTENSION`**（改 schema 的权限操作，不该由诊断工具代劳）
3. `track_real_stats=on` → 退化到 `sys_stat_sysmetric_history`，粒度粗（CPU/等待类趋势），但零安装成本
4. 都不满足 → WARN + 提示怎么开（一行命令：`ALTER SYSTEM SET track_real_stats = on; SELECT sys_reload_conf();`，同样不自动执行）

这条降级顺序参照了 `lib/cmd_stmt.sh:18-28` 处理 `sys_stat_statements` 未加载时的既有惯例（WARN + 提示，不是 FAIL），保持全仓库一致。

### 3. 默认时间区间

不传 `--from/--to` 时：
- `sys_kwr` 路径：最近两个可用快照（不管间隔多长，由 DBA 自己配的采集频率决定分辨率）
- `sys_stat_sysmetric_history` 路径：固定最近 15 分钟（这条路径本来就是轻量 fallback，没有离散快照点的概念）

`--from/--to` 传入时两条路径统一按绝对时间戳过滤。

### 4. `data` 字段：原样带出 KingbaseES 自己生成的报告，不解析

`perf.kwr_report(1, 2, 'text')` 实测输出是一份 904 行的纯文本 AWR 风格报告（中文，等待事件/DB Time/Top SQL 全在里面）。决定 **`data.report` 直接存原始文本，不拆解成结构化字段**。

理由：
- `ora` 自己对 Oracle AWR/ASH 报告也是这么处理的——原样呈现，不重新解析内容
- 报告字段/格式跟着 KingbaseES 版本走，今天解析明天可能就碎
- 报告本身已经是 KingbaseES 官方算好的成品，kbdiag 的价值在"判断该不该生成、怎么生成"，不在于重新排版

`verdict` 只反映"报告生成成功与否 / 走了哪条数据源"，**不试图从报告内容里判断严重程度**（等待事件占比高不高之类的解读留给看报告的人）。

`sys_stat_sysmetric_history` 路径没有预渲染的报告文本可言（这个视图本身就是结构化行数据），按同一原则（"数据库给你什么形状就原样带出什么形状，不重新发明"）应该是 `data.metrics` 存原始行（`{metric, ts, value}` 数组），不套用 `data.report` 的字符串形状——**这条是我按 Q3 的原则自己推的，没有单独跟你确认过，如果你觉得两条路径应该统一成同一个字段形状，需要在这里改**。

## 已确认（2026-07-25，全部收尾）

### Q4. 没有快照时，`workload` 该不该自己补一个？—— 确认，按推荐方案做

可用快照 < 2 个时，`workload` 自己调一次 `perf.create_snapshot()` 补数据，`verdict` 走 WARN（不是 OK），`data` 里附一句"当前区间基于本次自动生成的快照，历史对比数据不足，建议之后再跑一次"。不新增任何默认后台调度（不擅自建 cron job）。`--no-snapshot` 关掉这个行为，退化成"没数据就直接 WARN 提示手动执行"。

### 测试环境状态 —— 保留

`kes-node1` 的两个改动（`track_real_stats=on`、装了 `sys_kwr` 并有 snapshot 1/2）保留作测试夹具；`kes-node2` 保持原状（未开启/未装），两个节点分别覆盖"已开启/已装"和"未开启/未装 → WARN 提示"两条分支，写回归测试直接用，不用来回切换环境。

## 明确不做（Option C 范围内）

- 不做 pg_profile 式 kbdiag 自建的有状态指标仓库
- 不做 pgBadger 式富 HTML 报告渲染
- 不新增任何默认开启的后台采集/调度（cron、systemd timer 等）——所有数据采集要么是 KingbaseES 自己在做，要么是 `workload` 调用时的一次性动作
