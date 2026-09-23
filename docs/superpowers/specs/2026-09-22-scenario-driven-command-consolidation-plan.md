# 场景驱动的命令整合规划（v2，已用代码证据订正）

**状态**：v1（仅凭场景文档措辞推断）已被 `oh-my-claudecode:critic` 评审推翻核心结论，本版本改用**代码级逐行比对**作为一手证据，场景引用降级为辅助信号。v1 的 §2.1（bloat 真重复）、§2.4（wait 真重复）结论均为误判，本版本订正。

**承接**：`2026-09-20-command-overlap-and-docs-audit.md` 找出 4 组重叠、6 个命令，是"命令名/子命令名字面冲突"的穷举，本身成立。本文档回答的是下一步——"重叠是不是真的功能重复，该怎么处置"，这一步必须靠代码事实，场景文档的措辞（两个命令在同一句话里被提到）**不能当作重复的证据**，这是 v1 的方法论错误，本版本吸取教训。

---

## 一、代码级比对结论（一手证据）

逐一读取涉及命令的完整 SQL/逻辑（`lib/cmd_perf.sh`、`lib/cmd_idx.sh`、`lib/cmd_advisor.sh`、`lib/cmd_wait.sh`、`lib/cmd_stat.sh`、`lib/cmd_space.sh`、`lib/cmd_locks.sh`），结论如下：

| 组 | 涉及命令 | 结论 | 证据 |
|---|---|---|---|
| 表/索引膨胀 | `perf bloat`（`lib/cmd_perf.sh:56`） vs `idx bloat`（`lib/cmd_idx.sh:120`） | **不重复** | 前者查 `sys_stat_user_tables` 的表级活跃/死元组比例（阈值 `KB_WARN_DEAD_PCT`），后者查 `sys_class.relpages` vs `reltuples` 的索引页膨胀率（硬编码 30%）。对象（表 vs 索引）、公式、阈值来源全不同 |
| 索引健康 | `perf index`（`lib/cmd_perf.sh:176`） vs `idx unused`（`lib/cmd_idx.sh:54`，用共享变量 `_IDX_UNUSED_FROM/WHERE`，定义于 `lib/cmd_idx.sh:6-13`） | **真重复，且 `perf index` 是更差的实现** | `idx unused` 的过滤条件正确用 `si.indisunique`/`si.indisprimary` 标志位排除唯一/主键索引，并要求 `sut.n_live_tup > 1000`；`perf index` 另起一段 SQL，只用 `indexrelname NOT LIKE '%_pkey'` 做名字匹配，会漏判非 pkey 命名的唯一索引，也没有表数据量下限过滤——两者对同一份数据可能给出不同的"未使用索引"清单 |
| 索引健康（汇总） | `advisor index`（`lib/cmd_advisor.sh:13`） | **不重复** | 代码注释明确写"reuses these instead of duplicating the SQL"，直接调用 `idx.sh` 定义的 `_IDX_UNUSED_FROM/WHERE`、`_IDX_DUP_FROM/WHERE`、`_IDX_MISSING_FROM/WHERE` 三组共享变量，是正当的"汇总三项检查 + 生成 `--fix` SQL（DROP/CREATE INDEX）"，符合三层命令哲学的"断层"定位 |
| Vacuum 健康 | `perf vacuum`（`lib/cmd_perf.sh:113`） vs `advisor vacuum`（`lib/cmd_advisor.sh:150`） | **不重复** | 前者判据是"距 autovacuum_vacuum_threshold 还差多少 + 是否 7 天未 vacuum"；后者判据是"死元组比例超 `KB_WARN_DEAD_PCT`"+独立的"XID age > 500M 冻结风险"，并生成 `VACUUM (ANALYZE)`/`VACUUM FREEZE ANALYZE` SQL。两套判据、两个阈值来源，`advisor` 侧多了 XID 冻结这个 `perf` 完全没有的维度 |
| 表膨胀（第三份实现） | `space frag`（`lib/cmd_space.sh:2`） | **同公式族的独立预警梯度，非代码重复** | 与 `perf bloat` 公式同源（死元组比例），但用独立阈值 `KB_WARN_FRAG_PCT`；代码注释明确写"故意比 `KB_WARN_DEAD_PCT` 更低/更早报警，是 heads-up 不是 go-vacuum-now"。这是有意的三级预警设计（`space frag`早期预警 → `perf bloat`确认 → `advisor vacuum`给处置 SQL），但 README 完全没体现这个梯度关系，DBA 从命令名看不出来 |
| 等待事件 | `wait`（`lib/cmd_wait.sh:10`） vs `perf wait`（`lib/cmd_perf.sh:226`） | **不是简单重复** | 两者共享同一个过滤条件 `_WAIT_EVENT_WHERE`（定义于 `lib/cmd_wait.sh:8`），但 `wait` 输出逐会话明细（pid/等待时长/SQL 文本），`perf wait` 输出按 `wait_event_type`/`wait_event` 聚合的统计（count+avg）。是同一份数据的明细/聚合两级视图。另外 `lib/cmd_stat.sh:50` 也在消费同一个 `_WAIT_EVENT_WHERE`，是被完全漏掉的第三个使用方 |
| 锁等待 | `locks wait`（`lib/cmd_locks.sh:2`） | **概念独立，不动** | 内容是会话级阻塞链（谁等谁、锁模式），与上面"等待事件统计"完全不是一回事，只是命令名共享"wait"这个词造成误读 |

---

## 二、处置结论（订正版）

### 2.1 bloat（`perf bloat` / `idx bloat`）—— 保留两者，不合并

不是重复。处置：README/`--help` 明确写清楚"`perf bloat` 查表膨胀，`idx bloat` 查索引膨胀"，两者是不同对象的诊断，不是同一诊断的两个入口。

### 2.2 索引健康（`idx unused`/`idx dup`/`idx missing` / `perf index` / `advisor index`）—— `perf index` 废弃，其余保留

真正需要处理的重叠只有这一处：`perf index` 与 `idx unused` 判定同一件事，且 `perf index` 的过滤条件有真实缺陷（名字匹配 vs 标志位判断）。

**处置**：`perf index` 标记 deprecated，输出改为调用 `_idx_unused` 或直接提示"请使用 `kbdiag idx unused`"，下一个大版本移除 `perf index` 子命令。`idx dup`/`idx missing`/`advisor index` 不动——三者定位清晰（`idx` 系列是查层明细，`advisor index` 是断层汇总+fix SQL），且 `advisor index` 已经在正确复用 `idx.sh` 的共享 SQL 片段，不存在重复代码。

`space frag` 独立处理，见 2.4。

### 2.3 vacuum（`perf vacuum` / `advisor vacuum`）—— 保留两者，改描述

不是重复。处置：README 和 `--help` 明确写"`perf vacuum` 查距离 autovacuum 阈值还有多远/是否过期，`advisor vacuum` 查死元组比例+XID 冻结风险并给处置 SQL"，对应查/断两层。不改代码。

### 2.4 表膨胀预警梯度（`space frag` / `perf bloat` / `advisor vacuum` 的膨胀检查）—— 保留三者，文档说明梯度关系

三者共享"死元组比例"这个公式族但阈值独立、用途不同（早期预警 / 确认 / 处置建议），是有意设计不是重复。处置：README 新增一段"表膨胀三级预警"说明，按 `KB_WARN_FRAG_PCT` < `KB_WARN_DEAD_PCT` 的顺序说清楚"先看 `space frag` 发现苗头 → `perf bloat` 确认程度 → `advisor vacuum` 拿处置 SQL"这条使用路径。不改代码，只解决"文档没有场景说明"这条原始指控。

### 2.5 等待事件（`wait` / `perf wait` / `cmd_stat.sh` 内联查询）—— 保留，文档消歧；`locks wait` 不动

不是简单重复，是明细/聚合两级视图 + 一个未被文档提及的第三消费者。处置：不合并（两种视图都有独立使用场景：明细用于"揪出具体卡住的会话"，聚合用于"总体等待画像"）。README 需要：①把"聚合等待事件统计"和"会话级阻塞链"（`locks wait`）用不同措辞区分；②在 `wait`/`perf wait` 描述里点出彼此的关系（明细 vs 聚合），并提一句 `cmd_stat.sh` 里也用到同一份等待事件数据；③代码层面把 `lib/cmd_stat.sh:50` 补进 `_WAIT_EVENT_WHERE` 的"已知消费者"注释列表（在 `lib/cmd_wait.sh:8` 定义处），避免未来改这个过滤条件时漏掉。

---

## 三、v1 方法论教训（保留记录，不删除，作为后续工作的前车之鉴）

v1 用"同一条场景文档里两个命令名被 `/` 并列提到"作为"真重复"的判据，实际只是文档作者写作时顺手举了两个相关命令，不代表两者功能等价。教训：**场景引用可以做初筛（缩小要比对代码的范围），但不能替代代码级比对下结论**。后续任何"命令 A 和命令 B 是否重复"的判断，必须以代码逐行比对为准，场景文档只用于回答"这个命令对应哪些真实故障场景"这个独立问题（§四）。

---

## 四、待处理事项（索引/空间类"零场景引用"问题，未被本轮推翻）

`idx unused`/`idx dup`/`idx missing`/`perf index`/`advisor index`/`space frag` 在 `docs/dba-scenario-acceptance.md` 的 24 个 DS 场景里零引用，这条发现本身没被代码比对推翻（代码比对回答的是"是否重复"，不是"是否对应真实场景"）。但 critic 指出 `dba-scenario-acceptance.md` 本身是未经用户审阅的草稿，且 §三只覆盖故障类场景（连接/锁/性能/vacuum/HA），索引调优不是"故障"，零引用可能只是该文档的覆盖范围问题，不能反推这些命令没用。

**处置**：不再用"零场景引用→废弃候选"的推论。先请用户对 `dba-scenario-acceptance.md` 做一次审阅确认（它现在还只是草稿），审阅时顺带判断是否需要补充"索引/空间调优"这一类非故障场景；索引/空间相关命令的去留，等场景文档定稿后再看，本文档不预设结论。
