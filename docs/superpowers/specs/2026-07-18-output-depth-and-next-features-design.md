# kbdiag 输出契约改造规格说明书：判定摘要 + 数据区

日期：2026-07-18（v2，经 grilling 会话重定向后重写）
状态：sessions 样板间待用户验收
前置：2026-07-14 需求分析；本文件 v1（四功能版）已作废，候选功能见文末

## 背景与方向

用户核心诉求（原话）：**"每个命令都有清晰的检查结果、深度。"** 两个既有问题：

1. 查层命令只列数据不下结论——工具看见问题（如挂起 1 分钟的事务）却不吭声（实测演示确认）
2. 34 个命令中仅 8 个响应 `-v`，结论背后的数据看不到

## 决策记录（grilling 会话结论）

| # | 决策 | 依据 |
|---|------|------|
| D1 | 34 命令统一"判定摘要（带数值）+ 数据区"结构 | 用户方向原话 |
| D2 | exit 语义按成熟惯例分型：判定型命令（check/cluster ready/diagnose/report）保持 0/1/2；查层数据命令默认恒 0；新增全局 `--exit-code` 后 WARN/FAIL 才反映到 exit | Nagios 插件规范 / `kubectl get` / `git diff --exit-code` 三类先例；零破坏现有脚本 |
| D3 | 阈值 = 内置默认 + `KB_WARN_*` 环境变量覆盖 | 既有惯例（如 KB_WARN_SWAPPINESS） |
| D4 | 样板间先行：sessions 按新契约完整改造，用户验收四形态输出后再分批铺开 | 防止方向性返工 |
| D5 | baseline / check --upgrade / metrics 移出本轮，存候选节 | 工作量饱和；输出改造可能改变对它们的需求判断 |
| D6 | 一次查询两次呈现：结论与明细来自同一份捕获数据，`-v` 不增加 DB 往返 | 对抗审查意见 |

## 输出契约

### 结构（text 格式）

```
== <标题> ==
[OK|WARN|FAIL|INFO] <判定项>: <结论> (<实测值>, threshold <阈值>)   ← 判定摘要区，1–4 行
<空行>
<数据表>                                                            ← 数据区，受 -n 限制
```

### 旗标矩阵

| 旗标 | 判定摘要 | 数据区 |
|------|----------|--------|
| （默认） | 全部 | 精简列 |
| `-v` | 全部 | 全列 + 扩展明细块 |
| `-q` | 仅 WARN/FAIL | 不输出 |
| `--format json` | json_item 全部 | 全列（恒全量，不受 -v 影响） |
| `--exit-code` | 不变 | 不变；exit = 最差判定（0/1/2） |

### 规则

- 判定行禁止裸结论：必须内联实测值；有阈值的必须带阈值
- 数据区与判定来自同一次查询的捕获结果（每判定项最多一条 SQL）
- 纯呈现型命令（见清单中标"呈现"）可以只有 INFO 行 + 数据区，不强造判定
- 操作型命令（kill/update/snapshot）exit 语义不变（操作成败）

## 查层命令判定清单（草案，逐行待审）

| 命令 | 判定项与默认阈值 | 级别 |
|------|------------------|------|
| sessions | idle-in-txn 时长 > `KB_WARN_IDLE_TXN`(300s)；Lock 等待会话 > 0；连接占比 > `KB_WARN_CONN`(80%) | WARN |
| locks | 存在等待链 WARN；等待 > 60s WARN；检出死锁 FAIL | WARN/FAIL |
| perf slow | 运行中查询 > `KB_WARN_SLOW`(60s) | WARN |
| perf bloat | 膨胀率 > 30% | WARN |
| perf vacuum | dead tuple 比 > 20% 或 last vacuum > 7d | WARN |
| perf io | 缓冲命中率 < 90% | WARN |
| perf wait / wal / top | 呈现（Lock 类等待 > 0 时 WARN） | — |
| stmt | 呈现（历史统计无普适阈值） | — |
| sql | 呈现 | — |
| explain | 现有红旗（seq scan/嵌套循环/外部排序）即 WARN | WARN |
| wait | Lock 类等待 > 0 | WARN |
| progress | 呈现 | — |
| jobs | broken job FAIL；近期失败运行 WARN | WARN/FAIL |
| partition | 缺 DEFAULT 分区 / 大小倾斜 > 10x / 孤儿分区 | WARN |
| stat | 命中率 < 90%；rollback 占比 > 10% | WARN |
| obj | 呈现 + last analyze > 7d WARN | WARN |
| colstat / temp | 呈现（KingbaseES 无会话级实时 temp 列——实施中发现旧版该功能从未工作过，已改为库级累计 + 语句级 Top） | — |
| idx | 重复索引 / 膨胀 > 30% / 疑似缺失 WARN；unused 仅 INFO | WARN |
| conf | pending_restart > 0；diff 跨节点不一致 | WARN |
| audit / logs | 既有判定性质保持，补数值 | 既有 |
| kill / snapshot | 操作型，exit 不变 | — |
| remote / watch | 透传被包装命令 | — |

看层命令：已有判定保持，逐行补实测值（规则同上）；断层命令：证据链每节点补完整源数据。

## 样板间：sessions

改造为新契约的第一个完整实现，验收四形态：

1. 默认：判定摘要（3 项）+ 精简表（PID/USER/STATE/DURATION/WAIT/QUERY-60）
2. `-v`：+ APP/ADDR/事务时长/等待类型，QUERY-120
3. `-q`：仅 WARN/FAIL 行
4. `--exit-code`：制造 idle-in-txn 后 exit=1，清理后 exit=0

core.sh 本批只加 `--exit-code` 旗标；`detail_*` helper API 从验收通过的样板中提炼（先有实物再定接口）。

## 铺开批次（样板验收后）

1. 批 1 查层剩余 20 个（含 helper 提炼与回填 sessions）
2. 批 2 看层 10 个（补数值 + `-v` 明细）
3. 批 3 断层 + 横切 4 个
每批全量回归 + 测试断言明细内容（列名与值，非仅存在性）与 exit 语义。

## 测试要求

- 每命令：默认判定行含数值断言、`-v` 明细内容断言、json `detail` 结构断言
- `--exit-code`：至少 sessions/locks 各一条 WARN 场景 exit=1 断言；无旗标时相同场景 exit=0 断言
- 现有 41 处 exit 相关断言逐批核对

## 候选功能（未排期，设计要点保留）

对抗审查成果，捡起时直接用：

- **baseline save/compare**：TSV 目录存储（绕开 bash 解析 JSON）；meta 记录 hostname/角色/system_identifier/stats_reset 做身份校验与计数器重置防护；±50% 变化标记需两侧 calls≥20 且 mean_time≥1ms
- **check --upgrade**：通用风险清单不绑版本；盘点项恒 INFO 不入 exit；help 声明 "passing ≠ 版本兼容"
- **metrics**：stdout Prometheus 文本 + textfile collector；导原始 counter（`_total`）不导预算命中率；`--out` 原子写；失败时输出 `kbdiag_scrape_error 1` 最小集
- **cluster verify**：撤销；cluster ready 补明细后重新评估
