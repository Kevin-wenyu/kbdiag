# kbdiag 场景驱动命令重构 — 需求文档

**状态**：v2，核心决策已由用户拍板（见"五、开放问题"1-4 项的确认结果），可以进入执行计划编写。
**背景**：`2026-09-22-scenario-driven-command-consolidation-plan.md`（v2）做的是局部去重（发现 4 组"疑似重叠"里只有 `perf index` 真重复）。用户认为这不够——问题不是"有没有重复代码"，是**命令的组织方式本身**：现在按"谁实现的"分层（`perf`/`idx`/`advisor`/`locks`/`space`/`wait` 六个平行入口），不是按"DBA 在想什么场景"组织。本文档是这次结构性重构的需求说明，参考 `ora`（本地 `~/Documents/oracle/ora`）的命令组织方式。

---

## 一、现状：kbdiag 命令全景（36 个顶层命令，代码穷举）

```
advisor analyze/index/params/vacuum   check[--os]        cluster[ready]     colstat
conf                                  diagnose            explain            idx bloat/dup/missing/unused
instances                             jobs                kill               license
locks wait/hold/deadlock              logs                obj                params
partition                             perf bloat/index/io/slow/top/vacuum/wait/wal
progress                              remote              replication        report
sessions                              snapshot            space[frag]        sql
stat                                  status               stmt               temp
update                                wait                watch              workload
```

其中 `perf`/`idx`/`advisor`/`locks`/`space`/`wait` 六个命令内部含子命令，是本次重构的主要对象；其余 30 个命令是单一职责的顶层命令（如 `status`/`check`/`logs`），暂不在本轮讨论范围（见"六、非目标"）。

## 二、参考模型

### 2.1 `ora`（`~/Documents/oracle/ora`，10433 行，本地文件，用户已看过）

命令组织原则：**扁平命名空间，~90 个场景动词，一个动词=一个诊断问题**，不按"这段代码属于哪个模块"分组。举例（`~/Documents/oracle/ora:100-320` 的 usage 块）：

- `hold_txlock` / `wait_txlock` —— 谁占着锁 / 谁在等锁，两个问题两个命令，不是 `lock hold`/`lock wait` 这种嵌套
- `tab_frag <owner> [frag_percent]` / `index_frag <owner> [frag_percent]` —— 表碎片/索引碎片，命令名直接是场景，不需要先选"用 perf 还是 idx"
- `top_cpu_usage` / `top_buffers_gets` / `top_executions` —— 每个"top N by X"都是独立命令，不是 `top --metric=cpu` 这种参数化
- `ash <minutes_from_now>` —— 一个动词覆盖"过去N分钟活跃会话"这整个场景，背后可能查好几张视图，DBA 不需要知道

DBA 使用 `ora` 时，思考路径是"我想知道 X"→直接敲跟 X 同名或谐音的命令，不经过"这属于哪一层/哪个模块"的中间判断。

### 2.2 `pganalyze`（`docs/superpowers/specs/2026-07-25-capability-gap-research.md` §1.5，已有调研）

组织原则：按**顾问角色**分组（Log Insights / Index Advisor / Vacuum Advisor / Schema Statistics），每个顾问内部自己决定要不要用历史数据、要不要给处置建议，用户不需要知道内部实现分层。这与 `ora` 的"纯场景动词"不同，是"场景聚类+顾问封装"，两者共同点是**用户可见的命令层从不暴露内部实现结构**。

*（若"之前说的那个工具"指的不是 pganalyze，请指出具体是哪个，本节需要订正）*

### 2.3 kbdiag 现状的问题（对照上面两个模型）

现在 `perf bloat`/`idx bloat`/`space frag` 是同一个"表膨胀"场景的三个不同入口，DBA 得先弄清楚"我该找 perf 还是 idx 还是 space"，这是纯粹的实现细节泄漏到了命令行接口上。`wait`/`perf wait`/`locks wait` 同理。

---

## 三、拟定原则（需要用户确认）

**原则 A（命名）**：命令名 = 场景问题本身，不是"实现它的模块名"。例如 `perf bloat`/`idx bloat` 二选一保留独立命令（因为对象不同，是两个真场景），但不再挂在 `perf`/`idx` 这两个人为的模块命名空间下面。

**原则 B（深度怎么表达）**：三层命令哲学（看/查/断）本身不变，但深度差异不该靠"命令属于哪个顶层分组"来体现，应该靠**同一场景内的子命令或参数**（例如 `bloat table`/`bloat table --advise` 生成处置 SQL，而不是"查表膨胀用 perf，要处置建议换去 advisor"）。

**原则 C（哪些该合并成一个场景）**：`perf bloat`(表)、`idx bloat`(索引)、`space frag`(早期预警) 这三个虽然对象不完全一样，但对 DBA 来说是"膨胀"这一个连续的场景光谱，应该收进一个命令族里用子命令/flag 区分对象和严重程度，而不是三个不相关的顶层命令。

以上三条是本文档的核心假设，**需要用户逐条确认或否决**，见五、开放问题。

---

## 四、试点：膨胀/索引/vacuum/等待事件四个场景的新命令形态（已按"场景族+子命令"风格定稿，供执行计划直接引用）

| 场景 | 现状（4-6 个入口分散在 3 个模块） | 新形态 |
|---|---|---|
| 表/索引膨胀 | `perf bloat`、`idx bloat`、`space frag` | `kbdiag bloat table [--early]`、`kbdiag bloat index`（`--early` 对应原 `space frag` 的早期预警阈值 `KB_WARN_FRAG_PCT`） |
| 索引健康 | `idx unused/dup/missing`、`perf index`（废弃）、`advisor index` | `kbdiag index unused`、`kbdiag index dup`、`kbdiag index missing`、`kbdiag index advise [--fix]` |
| vacuum | `perf vacuum`、`advisor vacuum` | `kbdiag vacuum status`、`kbdiag vacuum advise [--fix]` |
| 等待事件 | `wait`(顶层)、`perf wait`、`locks wait`、`cmd_stat.sh` 内联查询 | `kbdiag wait detail`（原顶层 `wait`）、`kbdiag wait summary`（原 `perf wait`）、`kbdiag wait blocking`（原 `locks wait`） |

旧命令名全部保留一个版本周期的 deprecated 别名（见五.3），指向对应新命令。

## 五、开放问题

**已拍板（2026-09-22）**：

1. **范围**：✅ 先做试点——只重构四、已审计清楚的四个场景（膨胀/索引/vacuum/等待事件），不做全量 36 命令重构。全量重构留到试点验证命名风格可行后再决定要不要扩大。
2. **命名风格**：✅ "场景族+子命令"（如 `bloat table`），不采用 `ora` 式纯扁平动词。理由：对现有 `lib/cmd_*.sh` 一文件一 dispatch 的结构改动最小，且深度信息（状态查询 vs 处置建议）能自然放进子命令而不是编出一堆独立顶层命令。
3. **兼容策略**：✅ 旧命令名（`perf bloat`/`idx bloat`/`perf index`/`advisor index`/`perf vacuum`/`advisor vacuum`/`wait`顶层/`perf wait`/`locks wait`）保留一个版本周期的 deprecated 别名，输出 WARN 提示改用新名，下一个大版本移除。

**仍待确认（不阻塞试点开发，执行中可以补）**：

4. **"之前说的那个工具"**：本文档暂按 `pganalyze`（§2.2）理解，用户尚未逐字确认，若有出入请指正，执行期间发现理解错误可以随时纠正参考模型章节。
5. **三层命令哲学章节是否要重写**：本文档假设深度（看/查/断）依然存在，只是不再体现为顶层命令分组，改为场景族内的子命令（如 `advise` 子命令=断层）。试点完成、命名风格验证可行后，一并回来确认 CLAUDE.md 的"三层命令设计哲学"章节要不要同步改写。
6. **`report`/`diagnose`/`advisor`（汇总类命令）角色**：试点只动 `perf`/`idx`/`advisor` 三个命令里和四个试点场景相关的子命令，`advisor` 命令本身（`analyze`/`params` 子命令）、`report`、`diagnose` 暂不动，等试点结束再评估。

---

## 六、非目标（本轮不涉及）

- `status`/`check`/`logs`/`cluster`/`snapshot` 等 30 个单一职责顶层命令——它们本身没有"多入口同一场景"的问题，不在这次重构范围
- 新增诊断能力（如之前讨论的 `sys_hypo` what-if 索引顾问）——那是独立的 spec，不和这次命令重构混在一起

---

## 七、下一步

本文档定稿依赖用户回答"五、开放问题"的 1-6 项。定稿后才进入 `.omc/plans/` 的执行计划编写，当前 `.omc/plans/2026-09-22-scenario-driven-command-consolidation.md`（局部去重版）作废，等这份需求文档定稿后重写。
