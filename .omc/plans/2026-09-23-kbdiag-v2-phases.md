# kbdiag 2.0 分阶段开发计划（到 v0.1）+ 文档治理

**状态**：active（r5 终稿，2026-09-23 用户批准：§"需要用户决定" 5 项全部按推荐）
**日期**：2026-09-23 | **模式**：ralplan SHORT
**输入**：`docs/rewrite/00~04`（编号以 04 为准）、用户纠正"简单查询优先"
**取代**：r4；`docs/rewrite/01-PRD.md` Q1/§10 的 MVP（conn+lock+check+diagnose）由本计划取代，0.5 回写
**进度**：0.1~0.5、1.1 已完成（证据见 `chronicle/2026-09-23.md`）；1.1 已经 Codex 审查（REQUEST CHANGES，4 项已修）；1.2 的 L3/L4 在 node1、node2 全绿，B1 已验证；code-reviewer 两轮意见已修复，最新 code-reviewer 结果为 APPROVE；Architect lane 改由 Claude Code 侧的 OMC architect 子代理（opus，只读）执行：首轮 REQUEST CHANGES，5 项已修（disabled 行判 UNKNOWN、`--active` 只影响显示、e2e 两处空转断言、连接只读参数加 L1 测试），2 项低优先级挪到阶段 2；architect 复核 APPROVE，1.2 审查门全部通过。1.3 输出形态已于 node1 核对并获用户确认（取舍见 CLAUDE.md）；sessions 的 docs 页要等 1.3 有了 Go commit 再写。1.3 已于 2026-09-24 经用户确认执行：归档提交 6015c47 打了 tag `shell-final`，shell 版和旧文档已从 main 删除，CI 和 pre-commit 已换成 Go 版。GitHub CI 在 84c883e 上已通过；kbdiag-docs `v2` 分支的 sessions 页已提交并推送（8d59224）。阶段 2 已完成（2026-09-24）：6 条命令实现，node1、node2 的 L3/L4/L5 全绿（含 L5 第 5 格 sys_monitor），queries.md 8 项全部已验证，Codex 审查 1 条 P2 已修；证据见 `chronicle/2026-09-24.md`。阶段 3 已做到审批门前（2026-09-24）：kbdiag-docs `v2` 其余 6 页已推送（881bde3）；README 已按 Go 版重写；amd64/arm64 静态二进制都已构建（amd64 在 node1 实跑，arm64 只在 linux/arm64 容器里冒烟，没有 arm64 的 KES 可测）；L6 验收步骤写在下方阶段 3 小节。L6 经用户决定改为按 DS 场景验收、由 AI 在 VM 上执行（2026-09-24），结果见 chronicle 同日"L6 改成按场景验收"一节。用户批准补 3 个缺口（status 连接数阈值、slots 建议改指 sessions、版本号只认 v2* tag），已实现、Codex 对抗审查并复核通过，两节点 e2e 全绿（见 chronicle 同日"场景验收后的补丁"）。下一步：kbdiag-docs `v2` 同步 status/slots 页；之后 tag `v2.0.0-alpha.1`、docs 合 main，这两步都要等用户确认。本文件是唯一的计划入口，其他工具目录下的计划不作数

---

## A. RALPLAN-DR 摘要

**Principles**
1. 简单查询优先：v0.1 每条命令看一眼就有答案，不做跨维度关联
2. 准确优先于数量：每条查询都要过四关（KES 改写 → 对文档 → VM 注入实测 → 注明出处）
3. 契约先定，实现后铺：输出结构、命名、降级、退出码在写代码前定死（PRD"只增不改"）
4. 文档少而活：职责唯一；过期内容移出工作区
5. 复杂度要有当下需求撑着

**Decision Drivers**：① 测试成本按**数据源**线性增长；② 出事时的第一问是会话、锁、谁压着视界；③ 尽早让用户试用、定形态

**Viable Options**

| 方案 | 数据源 | 优点 | 缺点 |
|---|---|---|---|
| ① P1 全做（19 条） | 约 13 | 巡检面最全 | 四关工作量最大；要做复制延迟注入和 repmgr 元数据调研；首次试用最晚 |
| ② 约 12 条（③ + params/conn/space/freeze/repl/cluster） | 约 10 | "出事"和"巡检"都覆盖 | 见下方风险差异 |
| ③ 核心 7 条（**推荐**） | 6 | 4 条命令共用 `sys_stat_activity`；覆盖第一问；最早试用 | v0.1 不能巡检；备库只验证角色切换和"不适用"，验证不到复制数值 |

**②和③的风险差异**：两者都要求 node2 以备库身份在线，也都要在备库上跑。区别在于：
- ③在备库上只断言"能跑，并输出正确的视角或不适用"，node2 只要处于 recovery 状态就够，复制延迟多少不影响结果。
- ②的 repl/cluster 要断言**延迟数值**和 repmgr 节点状态，因此需要可控的延迟注入、健康的 repmgrd，以及对 repmgr 元数据的调研。

**我的判断是 ③**：三个驱动都指向它；代价（备库数值、开关深度推后）用 §B 的两个样板实例部分对冲。

## B. v0.1 命令清单（7 条命令，覆盖 04 的 8 项）

| 序 | 编号 | 暂定名 | 回答的问题 | 数据源 | 备库上的行为 |
|---|---|---|---|---|---|
| 1 | B1+D1 | `sessions`（`--active` 吸收 D1） | 有哪些会话，谁跑得久，谁 idle in txn | S1 | 正常 |
| 2 | B2 | `session <pid>` | 这个会话在跑什么 SQL、在等什么、持有哪些锁、被谁挡住 | S1+S2 | 正常 |
| 3 | C1 | `locks` | 谁在等锁、直接被谁挡住（一层）、等了多久 | S2 | 正常 |
| 4 | B4 | `txn` | 长事务、idle in txn、最老的 backend_xmin、未结束的 2PC | S1+S3 | 长事务部分正常；2PC 部分 `not_applicable`，提示去主库查（0.2 实测：备库查不到主库的 2PC） |
| 5 | E1 | `waits` | 此刻各会话在等什么（汇总） | S1 | 正常 |
| 6 | A1 | `status` | 版本、role、downstreams、启动时长、连接数/上限、库大小、数据目录 | S5+S6 | 视角切换 |
| 7 | I2 | `slots` | 复制槽是否活跃、保留多少 WAL、xmin 是否压着视界 | S4 | 正常输出；WAL 保留量按角色改用 `sys_last_wal_replay_lsn()`（0.2 实测：备库上 current_wal_lsn 报错） |

**数据源（6 个）**：
- S1 `sys_stat_activity`
- S2 `sys_locks` + `sys_blocking_pids()`
- S3 `sys_prepared_xacts`
- S4 `sys_replication_slots`
- S5 实例函数：`version()`、`sys_is_in_recovery()`、`sys_postmaster_start_time()`、`sys_database` + `pg_database_size`，以及用 `current_setting()` 取单个参数值
- S6 `sys_stat_replication`，只取 `count(*)`

**status 的字段（N2）**：
- `role` **只表示恢复角色**：primary 或 standby，取自 `sys_is_in_recovery()`。
- 另加 `downstreams: n`（来自 S6），表示有几个下游。
- 不读 repmgr，也不判断 standalone。表达集群拓扑的 `topology` 字段属于 v0.2 以后（和 I3 一起做），v0.1 不做。

**读参数**：只用 `current_setting(name)` 取单个值。A2 之所以挪出，是因为 `sys_settings` 的 source 和 pending_restart 语义要单独过四关，和这里不是一回事。

**开关感知样板**：sessions 登记 `track_activities` 和 `track_activity_query_size`；开关关着时，报"开关没开"，而不是给出空的 SQL 文本。

**备库视角样板（A4）**：0.2 已在 node2 实测（2026-09-23）：txn 的 2PC 部分在备库上输出 `not_applicable`；slots 在备库上正常输出，按角色取 LSN。0.5 把这两条写进 PRD，之后按它们加断言。

**先做 `sessions`**：它的 probe 是 session、txn、waits 三条命令的地基。

**挪到 v0.2 的 12 项**：
- A2 params：sys_settings 的语义要单独过四关
- B3 conn：它是 sessions 的聚合
- D2 top：要处理累计值的统计起点和重置
- F1/F2/F3：空间和对象类，属于巡检
- G1/G2：维护类
- H2 archive：要先排除主动关闭归档的配置
- I1/I4 repl：需要复制延迟注入
- I3 cluster：需要先调研 repmgr

## C. 地基契约（阶段 0.5 写进 PRD，只定义不实现）

1. **Report 增加 `data`（A1）**：`data: {<probe_id>: {columns[], rows[], truncated: n}}`。列表命令的主体放在 data 里，findings 只放阈值标出的行；列名属于契约。
2. **probe_id 命名（N3）**：和 finding.id 用同一套数据域前缀，写成 `<域>.<对象>`，例如 `session.activity`、`lock.list`、`txn.prepared`、`slot.list`、`inst.info`、`inst.downstreams`。每个 probe_id 都登记在 queries.md 对应的那一行。
3. **状态增加 `not_applicable`（N1；0.5 落地为 probe 级的 `data.<probe_id>.status`，旧的 `skipped[]` 并入同一字段）**：表示这个 finding 在当前角色或环境下没有意义，比如备库上的复制槽。它**不参与 verdict 计算**，也不同于 `skipped`（skipped 是想查但没能查到）。这条要回写进 PRD §3 产品原则第 3 条"不假装 OK"，和它并列。
4. **verdict 规则**：
   - 取被标出行里最高的 level；没有标出行就是 OK。
   - 关键 probe 被 skipped、同时又没有 WARN/FAIL 时，verdict 为 UNKNOWN。
   - not_applicable 不参与计算。
   - 阈值默认值取自 04 的阈值表，放在 `rule.Defaults`，可以用 flag 覆盖（1.1 定：暂不建 config 包）。
5. **退出码**：沿用 PRD §5：`0` OK、`1` WARN、`2` FAIL、`3` UNKNOWN、`64` 用法错误、`69` 连不上。
6. **部分降级（A3）**：用 `redacted[]` 表示，每条是 `{probe_id, field, reason, rows_affected}`。KES 实际遮蔽哪些列，以 0.2 实测为准。
7. **命名（A2）**：v0.1 用扁平命令。finding.id 和 probe_id 都只用数据域前缀，不带命令名；golden 文件名不属于发布契约。

## D. 文档治理

**分工**：
- `docs/` 放当前事实。
- `CLAUDE.md` 放为什么这么设计、开发约定，以及**活文档表的唯一权威副本**。
- `.omc/plans/` 放要做的事，最多 1 个 `.md` 文件。
- `chronicle/` 放发生了什么，调研和讨论也记在这里。

**活文档 5 个**（这张表 0.4 抄进 CLAUDE.md，以 CLAUDE.md 为准）

| 文档 | 唯一职责 | 来源（只迁移结构，不重写内容） |
|---|---|---|
| `README.md` | 使用者：安装、命令、示例 | 阶段 3 重写 |
| `CLAUDE.md` | 设计理由、约定、KES 坑、活文档表；外加"shell 版（冻结）"小节，只写一句话 | 0.4 改成 Go 版 |
| `docs/PRD.md` | 需求、范围、契约、版本目标、DS 场景表（压缩为编号 + 一句话 + 版本） | 01 + 00 的结论 + DS 压缩 |
| `docs/queries.md` | 查询清单（唯一的"版本"列）、probe_id、DS、SQL 出处、开关、验证状态（`未验证` / `已验证 YYYY-MM-DD`） | 04 + 03 §6 |
| `docs/engineering.md` | 选型、架构、测试 L1~L6、注入手段、运行命令 | 02 + 03 |

- PRD 的版本路线只写目标和验收，不列条目；条目以 queries.md 的"版本"列为准。
- **禁止引用 tag 路径**：活文档不能引用 tag 里的具体路径。CLAUDE.md 的冻结小节是唯一例外，而且只写**占位符**形式：`git show shell-final:<路径>`，不写具体文件名，所以 0.4 的 grep 检查不会命中它（Critic-2）。

**归档方式：我选进 tag**。
- 理由：archive 目录只是换了名字的僵尸文件，agent 搜索时还会把它当作现行文档；真正需要的 DS 内容已经压缩进 PRD。
- 可选项 B：保留 `docs/archive/`，配一个索引 README，不计入活文档数。

**现有文件处置**（"进 tag"的做法：归档提交 → 打 `shell-final` → 从 main 删除。未跟踪的待删文件都先进归档提交，所以没有不可恢复的删除）

| 对象 | 处置 | 需用户确认 |
|---|---|---|
| `docs/rewrite/01`、`04` | 改名为 `docs/PRD.md`、`docs/queries.md` | — |
| `docs/rewrite/00`、`02`、`03` | 并入 PRD 和 engineering 后删除，原文进 tag | 是 |
| `docs/superpowers/`（30 个已跟踪 + 2 个未跟踪的 09-22 文件）、`docs/test-report-2026-07-09.md`、`docs/dba-scenario-acceptance.md`、`docs/problem-signal-extraction-2026-09-08.md` | 进 tag | 是 |
| `docs/docs-site-setup.md`、`scripts/init-docs.py`（未跟踪） | 进 tag | 是 |
| `.omc/plans/2026-09-22-...consolidation.md`（未跟踪） | 进 tag | 是 |
| `docs/adr/`（空目录） | 删除 | 是 |
| `.claude/agents/kbdiag-shell-dev.md` **和** `kbdiag-ops-diagnostician.md`（未跟踪，都是给 shell 版用的） | **两个都在 1.3 进 tag 后删除**；Go 版需要诊断 agent 时再另建（统一 r4 L95 和 L124 的不一致） | 是 |
| `.git/hooks/pre-commit`（检查 KB_* 变量） | 1.3 替换为文档数检查 | 是 |
| `docs/agents/domain.md:8,19`、`issue-tracker.md:30`、CLAUDE.md 的 Issue tracker 和 Domain docs 两节 | 删掉关于 docs/superpowers 和 docs/adr 的说法 | — |
| `.tokensave/`、`.omx-state-locks*`、`.claude/.headroom_*`、`.claude/settings.local.json` | 加进 `.gitignore`，不删 | 是 |
| `.omc/`（目前没有被 ignore） | `.gitignore` 加 `.omc/*` 和 `!.omc/plans/` | 是 |
| `chronicle/` | 纳入 git 跟踪 | 是 |
| tag `v1.0.0` | 保留（shell 版最后一次发布）；`shell-final` 打在它之后 | — |

**防僵尸规则**（0.4 写进 CLAUDE.md）
1. 只有 5 个活文档都装不下的长期职责才允许新建文档，而且要先改 CLAUDE.md 里的活文档表。
2. `docs/` 下的文件名不带日期。
3. 活文档第一行下写 `状态: active | 最后核对: YYYY-MM-DD`。
4. 计划状态只有四种：`pending user approval` → `active` → `done` / `superseded`。done 当天把"为什么"写进 CLAUDE.md、过程写进 chronicle，然后删除计划；superseded 的计划立刻删除。
5. 可执行检查（N4），pre-commit 和 CI 都跑：`test "$(find docs -name '*.md' -not -path 'docs/agents/*' | wc -l)" -eq 3 && test "$(find .omc/plans -name '*.md' 2>/dev/null | wc -l)" -le 1`

## E. 阶段划分（executor 执行，verifier 核对证据，L6 由用户本人做）

### 阶段 0：地基（不写产品代码）
| # | 任务 | 通过标准（命令 → 证据） |
|---|---|---|
| 0.1 | 修复实验室；修之前先把 `repmgr_slot_2` 的现状写进 chronicle | `repmgr cluster show` 两节点都是 running；`select slot_name,active from sys_replication_slots` 全部是 true |
| 0.2 | Go spike S1~S6（02 §4）；另外实测三件事：①用 `kbdiag_ro`（`CREATE USER kbdiag_ro PASSWORD '…'`，不授任何监控角色）查看别人会话时，哪些列被遮蔽；②**sys_monitor 是否存在、能否 GRANT、授予后遮蔽是否解除（N5）**；③在 node2 上跑 slots 和 2PC 查询的实际行为 | 三项结果都写进 engineering.md 的结果表，结论回写 PRD 契约。**阶段 3 的 L5 第 5 格和 §G 第 4 条依赖②的结论（Critic-3）**：sys_monitor 可用，就按原样断言；不可用，就换成实测可用的等价角色，找不到等价角色则删掉这一格，并写进 chronicle |
| 0.3 | 注入脚本 `e2e/inject/<名>.sh up\|down`：持锁/等锁、idle in txn、长查询、2PC、inactive 且带 xmin 的槽、`track_activities=off` | 每个脚本手工跑一遍：up → 在 S1~S4 里能看到目标对象 → down → 看不到。输出贴进 chronicle |
| 0.4 | 文档治理（§D 用户确认后）：迁移活文档，更新 CLAUDE.md 和 docs/agents 里的引用，改 `.gitignore`。归档提交放到 1.3 | `grep -rn "docs/superpowers\|docs/adr\|dba-scenario" CLAUDE.md docs/PRD.md docs/queries.md docs/engineering.md docs/agents` 无输出（冻结小节只写占位符，不会命中） |
| 0.5 | 把 §C 契约回写 PRD：`not_applicable` 写进 §3 原则 3，§5 加入 data、redacted、probe_id、downstreams，v0.1 范围改为本计划的 §B；为 7 条命令各写一个 JSON 示例 | 以下两条都满足（第 10 条）：`for k in '"data"' '"redacted"' 'not_applicable' '"downstreams"' 'probe_id'; do grep -q "$k" docs/PRD.md \|\| echo MISSING $k; done` 无输出；`grep -c '^#### 示例：' docs/PRD.md` 等于 7，并且每个示例都能被 `jq .` 解析 |

### 阶段 1：骨架 + `sessions` + 切换点
- 1.1 骨架（conn/probe/facts/rule/report）+ `sessions`
  - **范围**：只建 `sessions` 用得到的接口，不为另外 6 条命令预铺。
  - **SQL 边界**：`session.activity` 的 SQL 在 1.1 里走完 queries.md"SQL 引入规则"的第 1、2、4 关：改写成 KES 语义；对照 KES 文档核对视图和列名；在 queries.md 的 B1 行登记出处。第 3 关（VM 实跑）在 1.1 里只做冒烟：在 node1 和 node2 上用 ksql 各跑一次，确认能执行、列齐全，输出贴进 chronicle。**B1 的验证状态保持 `未验证`**，1.2 的 L3/L4 通过后才改成 `已验证 YYYY-MM-DD`。
  - **证据分级**：`go vet ./... && go test ./...` 只证明 L1/L2（规则、渲染、契约），不能写成"KES 验证通过"；KES 行为的证据只认 VM 实跑。
  - **通过标准**：本机 `go vet`、`go test` 全绿；L2 golden 的 JSON 形状与 PRD §5.1 `sessions` 示例的字段一致；两台节点的 ksql 冒烟输出都已进 chronicle。
- 1.2 测试（L4 按命令注入）：
  - 本机跑 `go test ./...`（L1/L2）。
  - `KB_TEST_NODE=kes-node1 go test -tags vm ./e2e/...`（L3/L4），覆盖阴性、诱饵、`track_activities=off`，并用 `kbdiag_ro` 夹具断言 `redacted[]`。
  - 再用 `KB_TEST_NODE=kes-node2` 跑一遍备库。
  - L3/L4 通过后：B1 改为 `已验证 YYYY-MM-DD`；在 kbdiag-docs 的 `v2` 分支写 `sessions` 页（中英），示例取自这次 VM 实跑，并注明对应的 Go commit。
- 1.3 **切换点**（四件事一起做）：
  1. 归档提交，打 `shell-final`。
  2. **删除 shell 版（Critic-1）**：`lib/`、`dist/`、`build.sh`、`kbdiag.sh`、`scripts/deploy.sh`、`scripts/setup-dev.sh`、`scripts/hosts.example`，以及**整个 `test/`**（`test/cases/`、`test/lib/`、`test/setup/`、`test/unit/`、`run_tests.sh`、`perf_check.sh`），还有两个 `.claude/agents`。
  3. Go 版测试统一放在 `e2e/`，注入脚本从一开始就建在 `e2e/inject/`，不会被这次删除波及；需要参考旧注入 SQL（如 `make_lock.sql`）时，从 tag 里取。
  4. `ci.yml` 改成跑 `go vet`、`go test ./...` 和文档数检查，并替换 pre-commit。
- 1.3 完成时，在 chronicle 记一句（N5）：**从 1.3 到阶段 3 之间，main 上只有 `sessions` 可用，其余命令陆续加入；需要完整旧功能请用 `v1.0.0`**。
- **通过标准**：
  - 1.2 的 go test 全绿，日志附在提交说明里；CI 绿；文档数检查通过。
  - `git ls-files lib dist test build.sh kbdiag.sh` 无输出。
  - 用户确认形态，**确认项**：命令名、列名和列顺序、默认排序、默认行数上限、截断提示、`--json` 字段名、finding.id 前缀、**probe_id 命名（N3）**。结论写进 CLAUDE.md。

### 阶段 2：另外 6 条
- 顺序：`session` → `locks` → `txn` → `waits` → `status` → `slots`。每条都跑 1.2 的两条命令，主库和备库各跑一次。
- 1.2 architect 审查挪过来的两项：
  - `conn.Identify` 失败时（连上了，但查实例信息超时或报错）现在返回 69"连不上"，改成 UNKNOWN(3)，写 stderr（`cmd/kbdiag/main.go`）。
  - 按角色判 `not_applicable`（如备库上的 txn 2PC）：写第一个备库相关的命令时，让 probe 函数接收 `facts.Context`，由它自己返回 `not_applicable`。这属于"能不能采"，不算业务判断。`rule.Merge` 等到多 probe 命令（status）真用到时再加。
- **通过标准**：全绿；queries.md 里 8 项都是 `已验证 YYYY-MM-DD`，并登记了 probe_id 和出处；其余 50 项保持 `未验证`；每条命令在 kbdiag-docs `v2` 分支都有一页，写法同 1.2。

### 阶段 3：v0.1 验收
- L5 降级矩阵，每格一条断言：①主库；②备库；③远程 TCP；④`kbdiag_ro` 不授监控角色；⑤`kbdiag_ro` 授予 sys_monitor（或 0.2 实测出的等价角色；如果不存在，这一格按 0.2 结论删除）。
- README 重写；用 `GOOS=linux GOARCH=amd64|arm64 CGO_ENABLED=0 go build` 出两个二进制，拷到 node1 上直接跑。
- 我出验收步骤，用户本人在 Lima 上走一遍（L6），签字记进 chronicle。
- 发布 `v2.0.0-alpha.1` 时，把 kbdiag-docs 的 `v2` 分支合进它的 main（在这之前线上站点保持 v1 手册）。

#### L6 验收步骤（用户本人执行，AI 不代签）

在 Mac 的仓库根目录执行；整个过程开着 `caffeinate -dimsu`（Mac 休眠会冻住 VM，时间类判定会失真）。每步写下实际退出码，和"预期"对不上就停下记进 chronicle。

0. **准备**：`caffeinate -dimsu &`；然后
   `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(git rev-parse --short HEAD)" -o /tmp/kbdiag ./cmd/kbdiag`，
   两台都部署：`for n in kes-node1 kes-node2; do limactl copy /tmp/kbdiag $n:/tmp/kbdiag && limactl shell $n sudo install -o kingbase -m 755 /tmp/kbdiag /home/kingbase/kbdiag; done`。
   再定义两个函数（bash/zsh 都能用）：`k1() { limactl shell kes-node1 sudo -iu kingbase "$@"; }; k2() { limactl shell kes-node2 sudo -iu kingbase "$@"; }`。
   预期：`k1 ./kbdiag --version` 打印当前 commit。
1. **status**：`k1 ./kbdiag status; echo $?`、`k2 ./kbdiag status; echo $?`。预期：第一行角色分别是 primary / standby；downstreams 分别是 1 / 0；退出码都是 0。和 `repmgr cluster show` 对得上。
2. **sessions**：`KB_TEST_NODE=kes-node1 e2e/inject/idle_txn.sh up`，等 10 秒，`k1 ./kbdiag sessions --idle-in-txn-warn 5; echo $?`。预期：WARN，`session.idle_in_txn` 点名 application_name 为 `kbdiag_inj_idle_txn` 的那个 PID，退出码 1。不带参数再跑一次：OK，退出码 0（没到 300 秒）。然后 `idle_txn.sh down`。
3. **locks + session + waits**：`KB_TEST_NODE=kes-node1 e2e/inject/lock.sh up`，等 15 秒。
   - `k1 ./kbdiag locks; echo $?`：WARN，`lock.waiting` 写"会话 W 等 public.kbdiag_inj_lock … 被 H 挡住"，退出码 1。
   - `k1 ./kbdiag session H`：lock.list 里有 H 的 AccessExclusiveLock 和 W 那条 `granted=false`；H 的 state 是 idle in transaction。
   - `k1 ./kbdiag waits`：有一行 `Lock relation active 1 [W]`，退出码 0。
   - `k1 ./kbdiag locks --json | python3 -m json.tool | grep -A3 blocker_pids`：能看到 `[H]`。
   - 在备库上重复一次（`KB_TEST_NODE=kes-node2`，用的是 advisory 锁）：`k2 ./kbdiag locks` 报 WARN，locktype 为 advisory。
   - 两台都 `lock.sh down`，再跑 `locks`：OK，退出码 0。
4. **txn**：`KB_TEST_NODE=kes-node1 e2e/inject/prepared.sh up`，
   - `k1 ./kbdiag txn; echo $?`：txn.prepared 里有 `kbdiag_inj_2pc`，OK，退出码 0（没到 900 秒）；
   - `k1 ./kbdiag txn --prepared-fail 5; echo $?`：FAIL，`fix: ROLLBACK PREPARED 'kbdiag_inj_2pc'`，退出码 2；
   - `k2 ./kbdiag txn; echo $?`：`txn.prepared: not_applicable`，退出码 0；
   - `prepared.sh down`。
5. **slots**：`KB_TEST_NODE=kes-node1 e2e/inject/slot.sh up`（约 30~40 秒），`k1 ./kbdiag slots; echo $?`：FAIL，`slot.inactive` 点名 `repmgr_slot_2` 并带 xmin，退出码 2。`slot.sh down` 后再跑：OK，退出码 0。
6. **权限**：`k1 bash -c 'PGPASSWORD=kbdiag_ro_T3st ./kbdiag sessions --host 127.0.0.1 -U kbdiag_ro; echo $?'`：第一行是 `kbdiag_ro@remote`，有 `redacted:` 行，UNKNOWN，退出码 3。`waits` 同样是 UNKNOWN/3；`status` 仍是 OK/0。
7. **错误码**：`k1 ./kbdiag session; echo $?` → 64；`k1 ./kbdiag status --port 1; echo $?` → 69，并打印连接失败原因。
8. **收尾**：两台都检查 `select count(*) from sys_stat_activity where application_name like 'kbdiag_inj%'` 为 0，`sys_prepared_xacts` 为空，`repmgr_slot_2` 为 active。
9. **签字**：在 `chronicle/<当天>.md` 写"L6 验收：通过/不通过，执行人，日期，所用 commit"，不通过的步骤写实际输出。之后才进入 tag `v2.0.0-alpha.1` 和 docs 合 main（这两步也要用户点头）。

## F. 风险

| 风险 | 应对 |
|---|---|
| Go spike 不过 | 单项不过，按 02 §4 的绕过方案处理；核心项（socket 或 SCRAM）不过，就停在阶段 0，按 02 §2 重新选型 |
| node2 修不好 | 阶段 1~2 先只在 node1 上验证，备库那一格标 `待补`；验收时必须补上备库格，兜底办法是 `repmgr standby clone` |
| DS 引用断链 | 0.4 先把 DS 压缩进 PRD 并跑 grep 检查，1.3 才删原文 |
| 遮蔽行为或 sys_monitor 和 PG 不一样 | 0.2 实测后再定 `redacted[]` 语义和 L5 第 5 格 |
| 过渡期 main 上功能变少 | 在 chronicle 和 README 里写明，要用完整功能请用 `v1.0.0` |
| 实验室状态漂移 | 跑 vm 测试前先检查节点角色和槽状态，不健康就拒绝运行 |

## G. 验收标准（v0.1）
1. 7 条命令在 node1 和 node2 上的退出码都属于 PRD §5 的定义，没有 panic；退出码 64 和 69 各有一条测试。
2. 每条命令的注入场景都能断言到被注入的对象；阴性场景不报；诱饵不进结果。
3. txn 能报出注入的 2PC，slots 能报出注入的 inactive 槽和它的 xmin；备库上两者的输出**符合 0.2 实测后写进 PRD 的行为**；`not_applicable` 不改变 verdict。
4. 关闭 `track_activities` 时，sessions 报出开关名；`kbdiag_ro` 的输出里有 `redacted[]`；是否会随授予 sys_monitor（或等价角色）而消失，按 0.2 的结论断言。
5. 所有命令都有 `--json`，输出含 `data`，`context.role` 在所有命令里都有，status 的 `data["inst.downstreams"]` 有 `downstreams` 列（字段路径以 PRD §5.1 为准），并通过 L2 golden。
6. queries.md 里 8 项已验证，并登记了 probe_id 和出处；文档数检查在 CI 和 pre-commit 里都通过。
7. 用户本人按验收步骤走完。

## 需要用户决定
1. v0.1 采用方案③（7 条命令）
2. §D 处置表里标"是"的各项，逐项确认
3. 归档方式：进 tag（推荐），还是可选项 B `docs/archive/`
4. Go 版的 tag 命名：建议 `v2.0.0-alpha.1`，对内仍叫 v0.1（因为已有 `v1.0.0`）
5. 命名：v0.1 用扁平命令（推荐），还是 PRD §4 的场景族

## ADR

- **Decision**：
  - kbdiag 2.0 的 v0.1 采用方案③：7 条单次查询命令（sessions、session、locks、txn、waits、status、slots），用到 6 个数据源。
  - 输出契约在阶段 0 写进 PRD，包括 data、probe_id、redacted、not_applicable、role/downstreams 和退出码。
  - 活文档收敛到 5 个；旧文档和 shell 源码在 1.3 切换点一起进 `shell-final` tag。
- **Drivers**：
  - 测试成本按数据源线性增长。
  - 出事时的第一问是会话、锁、谁压着视界。
  - 要尽早让用户试用、定下形态。
  - 用户明确要求不要有僵尸文档。
- **Alternatives considered**：
  - ① P1 全做（19 条）：成本最高，而且要先做复制延迟注入和 repmgr 调研。
  - ② 12 条：多出来的是巡检类命令，外加复制延迟数值和 repmgr 状态断言的风险。
  - 归档用 `docs/archive/`：留给用户作为可选项 B。
  - 命令按场景族命名：推到 v0.2 再评估。
- **Why chosen**：
  - ③ 同时满足三个驱动。
  - 备库和开关这两个难点各做了一个样板，不至于全部推到 v0.2。
  - 进 tag 让工作区里不留僵尸文件，需要的 DS 内容已经压缩进 PRD。
- **Consequences**：
  - v0.1 不能用来巡检，也不输出 standalone 和 topology。
  - 从 1.3 到 v0.1 验收之间，main 上只有部分命令可用。
  - 备库和 sys_monitor 相关的断言要等 0.2 实测后才定。
  - v0.2 首批要补 5 条巡检命令和 2 条复制命令。
- **Follow-ups**：
  - 0.2 的实测结论（遮蔽、sys_monitor、备库行为）回写 PRD 和本计划 §G。
  - v0.1 验收后排 v0.2 的顺序。
  - 评估场景族命名、standalone/topology 判定。
  - 本计划 done 当天按规则 4 处理。
