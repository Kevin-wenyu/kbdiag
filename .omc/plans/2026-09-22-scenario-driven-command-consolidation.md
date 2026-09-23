> **状态：superseded**（2026-09-23）：shell 版计划，已被 `2026-09-23-kbdiag-v2-phases.md` 取代；1.3 进 tag `shell-final` 后删除，勿按本文件执行。

# 场景驱动命令重构 — 执行计划（v4，已按 Critic REVISE 全面订正）

Status: **pending approval**
Source requirements doc: `docs/superpowers/specs/2026-09-22-scenario-driven-command-taxonomy-requirements.md`
Superseded: v2（局部去重，用户否决）、v3（taxonomy 重构，Critic REVISE：3 CRITICAL + 4 MAJOR，均已核实属实）

## v3 → v4 变更摘要（核实结果见下方各条）

1. **Step1 降级**：不再"合并 `perf bloat`/`space frag`"——两者谓词方向、分母、排序、输出列、JSON 结构全不同（核实：`lib/cmd_perf.sh:56-111` vs `lib/cmd_space.sh:2-51`），合并会产出错误结果。改为"只共享 SQL 片段常量"，参照 `lib/cmd_idx.sh:6-13` 的 `_IDX_UNUSED_FROM` 既有做法。
2. **转发目标改为内部函数**：旧命令不能转发到新的 `cmd_bloat`/`cmd_index`/`cmd_vacuum` 入口函数，因为 `json_begin`/`json_end`（`lib/core.sh:154-181`）是全局单缓冲区，嵌套调用会吐出两份 JSON（核实：`cmd_perf.sh:409` 已经 `json_begin "perf"`）。转发目标改成内部函数（如 `_bloat_table`），由调用方（旧命令或新命令）各自负责一次 `json_begin`/`json_end`。
3. **exit-code 显式传播**：`perf`/`advisor` 系用全局累加器 `_PERF_EXIT`（核实：`cmd_perf.sh:406-428`），`idx`/`space` 系用函数 `return 1`——两套约定，wrapper 必须显式桥接，不能假设透传。
4. **WARN 走向订正**：`warn()` 在非 JSON 模式下写 **stdout**（核实：`lib/core.sh:100-106`，只有 JSON 模式才走 stderr），风险表原描述有误，需改为显式规定 WARN 目标流。
5. **补齐未披露的内部消费者**：`cmd_diagnose.sh:391`（`_advisor_index --collect`）、`cmd_report.sh:65/68/70`（`cmd_space`/`cmd_idx`/`cmd_advisor`）、`cmd_snapshot.sh:68/69`（`cmd_wait`/`cmd_perf`）、`cmd_watch.sh:23/25`（`cmd_wait`/`cmd_perf`）全部核实存在，新增"这些命令输出不得混入 deprecation WARN"的验收标准。
6. **build.sh 改动独立成 Step**：文件拼接列表（`build.sh:20-42`）是显式白名单，新文件不加会被静默丢弃；`wait`/`idx`/`space` 的 dispatch 行（`build.sh:74/93/72`）目前不转发子命令参数，需要重写，且这一步必须先于 Step1-4 的可测试性——原计划"每个 Step 完成后局部回归"的验证顺序不成立，改为 Step0 先落地 dispatch 骨架。
7. **AC8 改为可机器判定**：原"数据一致，允许字段重命名"无法自动判断，改为迁移前后各存一份 `--format json` 输出，用 `jq` 比对 `rows` 数值字段集合，显式给出允许改名的 key 映射表。

**已拍板的两个开放问题**：`space frag` 转发后 JSON 的 `"command"` 字段保持旧值 `"space_frag"` 不变（避免破坏按 command 字段分流的外部脚本）；裸 `advisor`（跑全部四项）不打 deprecation WARN，只有 `advisor index`/`advisor vacuum` 这两个具体子命令才算 deprecated。

## Requirements Summary

见需求文档。四场景新旧命令映射（不变）：

| 场景 | 旧命令 | 新命令 |
|---|---|---|
| 表/索引膨胀 | `perf bloat`、`idx bloat`、`space frag` | `kbdiag bloat table [--early]`、`kbdiag bloat index` |
| 索引健康 | `idx unused/dup/missing`、`perf index`（废弃不迁移）、`advisor index` | `kbdiag index unused/dup/missing/advise [--fix]` |
| vacuum | `perf vacuum`、`advisor vacuum` | `kbdiag vacuum status`、`kbdiag vacuum advise [--fix]` |
| 等待事件 | `wait`(顶层)、`perf wait`、`locks wait` | `kbdiag wait detail/summary/blocking`（见"待澄清"，`detail` 定义未定） |

## 待澄清（执行前必须先定，否则 Step4 无法开工）

`wait detail` 的默认输出到底是什么：现有顶层 `cmd_wait`（`lib/cmd_wait.sh:10-`）在非 `-v` 模式下输出的已经是按 type/event 聚合的汇总表（`cmd_wait.sh:69-74`），跟 `_perf_wait`（迁移目标 `wait summary`）高度重叠。两种选择：
- (A) `wait detail` 默认行为不变（仍是聚合表，`-v` 才出逐会话明细）——那么 `detail`/`summary` 几乎同质，拆分意义不大，命名需要重新想（例如 `wait sessions`/`wait stats` 更准确）
- (B) `wait detail` 改成默认输出逐会话明细——这是对现有顶层 `wait` 默认输出的行为变更，违反"旧命令转发后行为不变"的前提

**这一项必须在 Step4 开工前由用户选定，本计划不预设。**

## Acceptance Criteria

- [ ] AC0：`build.sh` 文件拼接列表新增 `lib/cmd_bloat.sh`/`lib/cmd_index.sh`/`lib/cmd_vacuum.sh`；dispatch 表 `wait`/`idx`/`space` 三行改为转发 `$SUBCMD`+`CMD_ARGS`（参照 `perf`/`advisor` 现有写法 `cmd_perf "$SUBCMD" "${CMD_ARGS[@]+"${CMD_ARGS[@]}"}"`），新增 `bloat`/`index`/`vacuum` 三个 dispatch 分支；`bash build.sh` 成功生成 `dist/kbdiag` 且旧命令行为不变（此时新命令的函数体还不存在，允许报"unknown subcommand"，只验证 dispatch 骨架不报语法错误）
- [ ] AC1：`lib/cmd_bloat.sh` 的 `bloat table`/`bloat index` 是迁移（原样搬运 `_perf_bloat`/`_idx_bloat` 逻辑），不合并阈值判定；`bloat table --early` 是新增能力，内部复用与 `_space_frag` 相同的 SQL 片段常量（新增共享变量，参照 `_IDX_UNUSED_FROM` 模式），但保留各自独立的输出渲染
- [ ] AC2：`lib/cmd_index.sh` 的 `unused`/`dup`/`missing` 迁移自 `lib/cmd_idx.sh:54/87/163`（复用既有 `_IDX_*_FROM/WHERE` 共享变量不变）；`advise [--fix]` 迁移自 `_advisor_index`（`lib/cmd_advisor.sh:13`），且必须保留 `--collect` 模式（供 `cmd_diagnose.sh:391` 调用，见 AC6）；`perf index` 不迁移，直接改为 WARN+调用 `index unused` 的内部函数
- [ ] AC3：`lib/cmd_vacuum.sh` 的 `status`/`advise [--fix]` 分别迁移 `_perf_vacuum`（`lib/cmd_perf.sh:113`）/`_advisor_vacuum`（`lib/cmd_advisor.sh:150`），逻辑原样搬运不改动
- [ ] AC4：按"待澄清"一节的用户选定方案实现 `wait` 三个子命令，`_WAIT_EVENT_WHERE` 定义（`lib/cmd_wait.sh:8`）补注释列出全部消费者（含 `lib/cmd_stat.sh:50`）
- [ ] AC5：旧命令（`perf bloat`/`idx bloat`/`space frag`/`perf index`/`idx unused`/`idx dup`/`idx missing`/`advisor index`/`advisor vacuum`/`perf vacuum`/顶层`wait`/`perf wait`/`locks wait`）改为调用对应**内部函数**（不是新的 `cmd_*` 入口，避免 `json_begin` 嵌套）；WARN 提示文本在 JSON 模式走 stderr、非 JSON 模式走 stdout（沿用 `warn()` 现有行为，不额外改路由）；退出码由各旧命令的 wrapper 显式桥接到自己原有的约定（`_PERF_EXIT` 累加或 `return 1`，不透传新函数返回值时更改约定）；`space frag` 转发后 JSON 输出的 `"command"` 字段保持 `"space_frag"` 不变；裸 `advisor`（无子命令）不打 WARN，只有 `advisor index`/`advisor vacuum` 这两个具体子命令才打
- [ ] AC6：`cmd_diagnose.sh:391`、`cmd_report.sh:65/68/70`、`cmd_snapshot.sh:68/69`、`cmd_watch.sh:23/25` 的输出在改动前后跑一遍对比，确认不出现 deprecation WARN 污染（这些是直接调用内部函数或未被标记 deprecated 的路径，不应触发 WARN）
- [ ] AC7：README 中英文按新命令名组织，旧命令名标注 deprecated + 指向新命令；`cmd_diagnose.sh` 里硬编码的旧命令名建议文案（如 `384/430/456` 行附近的 `kbdiag advisor vacuum` 字样，具体行号执行时用 `grep -n "kbdiag " lib/cmd_diagnose.sh` 核实）同步改成新命令名
- [ ] AC8：迁移前后分别对同一 VM 状态跑 `--format json`，用 `jq` 比对 `.rows[]` 的数值字段集合是否一致，允许改名的 key 需要在测试脚本里显式给出映射表（例如 `_perf_bloat` 的 `live_pct` 对应 `bloat table` 输出里的同名或重命名字段，执行时按 AC1 实际实现补全映射）

## Implementation Steps

### Step 0 — dispatch 骨架（前置，AC0）
- 改 `build.sh` 文件拼接列表和 dispatch 表，新命令函数体先留空/占位
- 这一步完成后才具备"改一个场景、测一个场景"的能力，后续 Step1-4 的局部回归才成立

### Step 1 — `bloat` 场景族（AC1，按订正后的"不合并"方案）
- `lib/cmd_bloat.sh` 的 `bloat table`/`bloat index` 原样迁移 `_perf_bloat`/`_idx_bloat`
- `--early` 走独立的共享 SQL 常量方案，不复用 `bloat table` 默认分支的渲染代码
- 旧命令 `perf bloat`/`idx bloat`/`space frag` 的 wrapper：调用新内部函数 + 显式桥接各自原有的 exit-code 约定 + `space frag` 保留 `"command":"space_frag"`

### Step 2 — `index` 场景族（AC2）
- 迁移 `unused`/`dup`/`missing`，共享变量位置不变
- `advise` 迁移时保留 `--collect` 模式（`cmd_diagnose.sh:391` 依赖）
- `perf index` 直接废弃，wrapper 转发到 `index unused` 内部函数

### Step 3 — `vacuum` 场景族（AC3）
- 原样迁移，不改动逻辑

### Step 4 — `wait` 场景族（AC4，依赖"待澄清"一节先由用户选定方案）
- 按选定方案实现三个子命令
- 补 `_WAIT_EVENT_WHERE` 消费者注释

### Step 5 — 内部消费者回归确认（AC6）
- 对 `cmd_diagnose.sh`/`cmd_report.sh`/`cmd_snapshot.sh`/`cmd_watch.sh` 逐一跑一遍，确认输出无 WARN 污染、行为不变

### Step 6 — README / 帮助文本同步（AC7）
- 中英文 README、`cmd_diagnose.sh` 内嵌文案

### Step 7 — 测试（AC8）
- 新增 `test_bloat.sh`/`test_index.sh`/`test_vacuum.sh`，扩展 `test_wait.sh`
- 每个新命令的测试包含 jq 数值字段比对断言
- 旧命令 deprecation WARN 单独断言（含"裸 advisor 不应出现 WARN"的反向断言）

## Risks and Mitigations

| 风险 | 缓解 |
|---|---|
| Step0 的 dispatch 骨架改动本身可能引入 shell 语法错误，影响全部现有命令（不只是四个试点场景） | Step0 完成后立即跑一次全量回归（不是局部），确认现有 34 个命令行为零变化，再开始 Step1 |
| wrapper 里 exit-code 桥接写错，导致 `--exit-code` 语义悄悄失效 | AC5 的每个 wrapper 都要求单独的 `--exit-code` 断言（新增，不在 v3 风险表里），覆盖"聚合场景"（裸 `perf`/`idx` 跑全部子项）和"单项场景"两种路径 |
| jq 版本/可用性在测试 VM 上不确定 | Step7 开工前先确认 kes-node1/kes-node2 或本地测试执行环境有 `jq`，没有则改用 `python3 -c` 或现有 `awk` 方案等价比对 |
| "待澄清"一节的 `wait detail` 方案选择会影响 Step4 的实现细节和 AC4 的具体验收标准 | Step4 排在 Step1-3 之后，若"待澄清"迟迟未决，先完成 Step0-3+5-7（其余三个场景不受影响），Step4 单独排期 |

## Verification Steps

1. Step0 完成后：全量回归（不是局部），确认现有命令零变化
2. Step1-4 各自完成后：局部回归 + VM 上人工对比新旧命令同一状态输出（数值一致）
3. Step5 完成后：`diagnose`/`report`/`snapshot`/`watch` 输出逐一检查无 WARN 污染
4. Step7 完成后：全量回归，记录本次实际通过数量到 chronicle
5. 全部完成后：按 CLAUDE.md「验收纪律」由用户本人在 VM 上照验收步骤实操确认

## Open Items

- **阻塞 Step4**：`wait detail` 默认行为方案（A/B，见"待澄清"）需要用户选定
- 不阻塞：需求文档里"之前说的那个工具"是否为 pganalyze、三层命令哲学章节是否重写、`report`/`diagnose`/`advisor`(analyze/params) 本轮是否扩大范围

## Changelog

- v4（本次）：按 Critic REVISE（3 CRITICAL + 4 MAJOR，全部核实属实）全面订正，Step1 降级为不合并、转发目标改为内部函数、exit-code 显式桥接、补齐 4 个未披露的内部消费者、build.sh 改动独立成 Step0、AC8 改为 jq 可判定断言；新增"wait detail 默认行为"这一阻塞性待澄清项
- v3：taxonomy 重构版（局部去重升级为场景化命令族）
