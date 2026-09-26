# 打磨剩下的 6 条命令（云会话长跑 + 本地 VM 收口）

**状态**：active（用户 2026-09-26 定：交给云会话长跑，用户按阶段异步审核）

**背景**：status 已打磨完（见 CLAUDE.md"status 打磨"一节）。剩下 `sessions`、`session <pid>`、`locks`、`txn`、`waits`、`slots` 6 条。逐条走"场景表 → 参数 → 草样 → 实现"太慢，而云会话的额度快到期，所以改成：本地先把 VM 数据一次采齐（阶段 0），云会话按本计划一条命令接一条命令地做（阶段 1–8），不停下来等审核；用户在每个检查点异步审核，有意见就在云会话里追加消息，云会话用修正提交处理。VM 验证、Codex 审查、kbdiag-docs、合进 main 都在本地做（阶段 9）。

## 0. 云会话必读（开工前）

1. 先读 `CLAUDE.md`（全部）、本计划、`chronicle/` 最新两天、`docs/PRD.md` §4/§5、`docs/queries.md`，再看 `internal/report/status.go` 和 `internal/scenario/testdata/status_*.golden`：它们是打磨的模板。
2. **云容器连不到 Lima VM**。能做的只有 L1/L2（`go vet`、`go test`、`-race`、交叉编译、`go vet -tags vm ./e2e/` 只编译）。所有 VM 相关的结论都写成"未验证"，列进该阶段的"VM 待验清单"，**不许写成已验证**。
3. **KES 行为只认两处证据**：`CLAUDE.md` 的"KingbaseES 特有行为"和 `e2e/testdata/captures/`（阶段 0 的实采）。实采里没有的行为不许按 PG 猜着写进断言；status 那次 e2e 挂在 kbdiag_ro 上，就是按 PG 猜的。拿不准时写进 VM 待验清单，e2e 断言写宽或先不写。
4. **只推云会话指定的分支**，不推 main、不打 tag、不碰 kbdiag-docs。
5. `TYPESAFE_API_KEY` 没有就跳过 Jev，在 chronicle 里注明，回本地再补。
6. 不起多 agent 工作流（ralplan/team/autopilot/ultragoal）。单 agent 主线加上 code-reviewer 审查即可。

## 1. 每条命令的固定流程（阶段 1–6 都照这个做）

1. **场景表（MECE）**：用户跑这条命令时想问什么；每个问题现在怎么回答、改成什么；不归它管的问题指向哪条命令。
2. **参数**：从场景推。没有场景对应的参数删掉（alpha 阶段直接删，不留兼容）；不加过滤参数，要筛就用 `--json` 配 jq。
3. **判定**：FAIL = 业务已经受影响；WARN = 还没受影响，但不处理会出事。只用客观线；没人会调的阈值不做成参数。现有阈值（`rule.Defaults`：`LockWaitWarnS 10`、`XactWarnS 300`、`XactFailS 1800`、`PreparedFailS 900`、`IdleInTxnWarnS 300`）逐个说明保留、改还是删，以及理由。**改或删阈值属于改方向，列进"需要用户拍板"，但先按你的建议实现。**
4. **草样**：只用 `e2e/testdata/captures/` 里的实采数据手排文本输出，至少覆盖：主库有注入、主库干净、备库、kbdiag_ro。排版规则沿用 status（时长两个最大单位，大小按 1024 进位，表格列对齐，JSON 不变形）。
5. **契约变更**：列出命令名、参数、列、JSON 字段、finding.id、probe_id、next 的变化；JSON 尽量不变。
6. **先写失败测试**：golden 手抄草样，确认它先失败；再加遮蔽、截断、空结果的 golden。按"暴力测试"规则补边界：空结果、NULL 字段、超长 SQL/应用名、中文和多字节字符、0 和负的时长、`--limit 0`、行数恰好等于 limit。
7. **实现**：专用排版放 `internal/report/<命令>.go`；rule 保持纯函数；probe 只采集。
8. **e2e**：按新契约改 `e2e/`，只编译不跑；每条新断言在 VM 待验清单里点名。
9. **文档同步**：PRD §4 表和 §5.1 示例、queries.md、README（中英两段）、CLAUDE.md 加一节"<命令> 打磨"写**为什么**。不新建文档；跑防僵尸检查。
10. **自审**：起一个 code-reviewer 审这一阶段的 diff（不能自己写完自己审），处理意见。
11. **检查点提交**：提交信息以 `polish(<命令>):` 开头；chronicle 当日文件追加一节"<命令> 打磨（云会话）"，内容照 status 那节的格式：改动、实际跑过的命令和结果、VM 待验清单、需要用户拍板的点、下一步。然后推分支，**不等审核，直接进下一阶段**。

## 2. 阶段

| 阶段 | 在哪 | 内容 | 检查点（用户异步审核的东西） |
|---|---|---|---|
| 0 | 本地 | 在 node1/node2 上实采剩下 6 条命令的 text 和 JSON，存进 `e2e/testdata/captures/`（见 §3） | 采集清单齐全 |
| 1 | 云 | **sessions**：场景表和草样已经写好（附录 A），直接从第 6 步开始 | 附录 A 的两处改方向：默认不列后台进程、删 `--active` |
| 2 | 云 | **locks** | 场景表、草样、`LockWaitWarnS 10` 怎么处理 |
| 3 | 云 | **session \<pid\>** | 同上；和 locks 的挡路者信息怎么分工 |
| 4 | 云 | **txn** | 同上；`XactWarnS`/`XactFailS`/`PreparedFailS` 怎么处理；和 sessions 的 idle in txn 重叠（附录 A.3 已比过） |
| 5 | 云 | **waits** | 同上 |
| 6 | 云 | **slots** | 同上；下一步从"备库上看 sessions"改成 `kbdiag status`（附录 A.5） |
| 7 | 云 | **跨命令收口**：所有 `verify:` 指向的命令和参数都存在；各命令的 next 互相一致；README 和 PRD 的命令表一致；L2 加一个测试，扫描 rule 里所有 Next.Command，断言命令和参数能被 cobra 解析 | 一份跨命令一致性清单 |
| 8 | 云 | **加固和余量**：（a）给 `internal/report` 的表格和时长/大小格式化补边界测试和 `go test -fuzz` 的 fuzz 目标（本地跑 30 秒不崩即可）；（b）清点 7 条命令的 VM 待验清单，合成一份给阶段 9 用的核对表，写进 chronicle；（c）起草 `v2.0.0-alpha.2` 的发布说明，写进 chronicle，不打 tag | 核对表、发布说明草稿 |
| 9 | 本地 | fetch 云分支 → 每条命令两节点跑 e2e（`-count=1`）→ 修 → 把 VM 实跑的文本输出贴给用户 → Codex 审查 → Jev 分诊 → 修 → kbdiag-docs 各命令页（用 VM 实跑输出，注明 commit）→ 用户说"合并"才合进 main 并推 kbdiag-docs `v2` | 每条命令的 VM 输出；Codex 意见处理表 |

阶段 1–6 可以不按顺序等审核，但**要按顺序做**：后面的命令会引用前面命令的 next。用户对前一阶段提了意见，就先处理意见再继续当前阶段。

## 3. 阶段 0：本地实采清单

用当前 main 编出的 linux/amd64 二进制，两个节点都采，每条都存 text 和 `--json`，文件名 `<命令>_<节点>_<场景>.{txt,json}`，文件末尾记退出码。

| 命令 | 场景 |
|---|---|
| sessions | 干净；idle_txn + long_query；kbdiag_ro；track_off；conn（连接占满） |
| session \<pid\> | idle_txn 的 pid；lock 注入里挡路者和等待者的 pid；不存在的 pid；kbdiag_ro 看别人的 pid |
| locks | 干净；lock；prepared_waiter（挡路者是 2PC）；kbdiag_ro |
| txn | 干净；idle_txn；long_query；prepared；主库和备库上的 2PC；kbdiag_ro |
| waits | 干净；lock；long_query；kbdiag_ro |
| slots | 干净；slot；standby_slot；kbdiag_ro |

另外用 ksql 以 system 和 kbdiag_ro 各查一遍 `sys_locks`、`sys_prepared_xacts`、`sys_replication_slots` 在注入下的原始行，存成 `ksql_<视图>_<节点>_<用户>.txt`，给云会话核对"哪些列被遮蔽"。

采完删掉 VM 上的临时二进制、撤掉所有注入，在 chronicle 里记采集时间和 commit。

**这些文件的寿命**：阶段 9 结束、计划 done 时，被 golden 或 e2e 引用的留下，其余删掉，不留僵尸。

## 4. 审批门槛（压缩、交接、云会话都不能自己跨过）

- 合进 main、推 main、打 tag、推 kbdiag-docs：只有用户明确说了才做。
- 附录 A 和各阶段"需要用户拍板"的点：云会话先按建议实现，用户否了就改；不能因为已经实现了就当成用户同意。
- 暂停的 walreceiver 要不要用 `wal_receiver_timeout` 判：用户还没决定，本计划不碰。
- `inst.upstream` WARN 的注入：已定为已知限制，本计划不写注入脚本。

## 5. 进度

- 2026-09-26：sessions 两节点实采，写出场景表和草样（附录 A）
- 2026-09-26：用户决定交给云会话长跑；计划改成覆盖剩下 6 条命令
- 2026-09-26：**阶段 0 完成**。19:40–19:45 用 `v2.0.0-alpha.1-9-gdf9425f` 在两节点实采，109 组 text/JSON 加 ksql 原始行，共 219 个文件，在 `e2e/testdata/captures/`。清单和实采中的发现见 chronicle 同日"阶段 0：实采"一节，云会话从阶段 1 开始

## 附录 A：sessions 的场景表和草样（阶段 1 的输入）

### A.1 场景表（MECE）

用户跑 sessions 时想问的：

| # | 问题 | 现在怎么回答 | 改成 |
|---|---|---|---|
| S1 | 连接是谁占的（status 连接 FAIL 指过来的） | 自己在 15 列的表里数 | 默认给一段汇总：按 用户 / 库 / 应用 / 客户端 分组计数，数量从多到少 |
| S2 | 现在谁在干活、干了多久 | 表里混着 idle 和后台进程 | 默认只列不是 idle 的客户端会话（active、idle in transaction 等），按事务时长从长到短 |
| S3 | 谁开着事务不干活 | WARN `session.idle_in_txn` | 不变（见 §3） |
| S4 | 要看全部，包括 idle 会话和后台进程 | 默认就是全部 | `--all` |

**不在 sessions 里**（给出指向）：谁挡了谁 → `locks` / `session <pid>`；谁压着 vacuum 视界（xid/xmin）→ `txn`；大家在等什么 → `waits`；备库在不在收 WAL → `status`。

### A.2 参数（从场景推）

| 参数 | 场景 | 处理 |
|---|---|---|
| `--all` | S4 | 新增：列出全部会话，包括 idle 会话和后台进程 |
| `--limit N` | 会话很多时 | 保留，默认 50，只影响列表，判定和汇总覆盖全部 |
| `--idle-in-txn-warn S` | S3 | 保留，默认 300 |
| `--active` | — | **删除**：默认列表已经只看在干活的，它只多筛掉 idle in transaction，而后者正是要看的 |

不加 `--user`/`--app` 之类的过滤：汇总已经按这些分组了，要进一步筛用 `--json` 配 jq。

### A.3 判定

只保留一条：`session.idle_in_txn` **WARN**，事务开着却 idle 超过 300 秒（`--idle-in-txn-warn`）。

**为什么 300 秒可以保留**：这和 status 删掉的"连接数 80%"不一样。80% 连接对有的应用就是常态；idle in transaction 放着 5 分钟，无论应用怎么设计都是毛病（连接池泄漏、代码漏了 commit），而且它持有的锁和视界会越来越伤人。300 秒只是过滤噪声的下限，不是容量线。实验环境 `idle_in_transaction_session_timeout` 为 0（没设），所以没有服务器端的客观线可用。参数保留，因为 DBA 手工在 ksql 里开事务改数据是合理的例外。

**和 txn 的重叠**（逐行比过代码）：`rule.longTxn` 对所有会话判 `xact_age_s ≥ 300`。idle in transaction 的会话 `state_change` 不早于 `xact_start`，所以 sessions 报了，txn 在默认阈值下也一定报。两条 finding 的问题不一样：sessions 说"它闲着"，txn 说"事务太长"，各自命令能独立回答，所以保留。

### A.4 草样

用 19:26 的实采数据手排。

node1 主库，注入 idle_txn 和 long_query（阈值用 `--idle-in-txn-warn 1`，让 WARN 出现）：

```
sessions  WARN  (KingbaseES V008R006C009B0014, primary, system@local, 2026-09-26T19:26:45+08:00)

[WARN] session.idle_in_txn  会话 795860 处于 idle in transaction 已 1 秒
  verify: kbdiag session 795860  # 看它持有哪些锁、有没有挡住别人

connected: 5 client sessions (not counting kbdiag), 1 walsender, 7 background
  count  user    database  application            client
  2      esrep   esrep     internal_rwcmgr        192.168.105.10
  1      esrep   esrep     internal_rwcmgr        192.168.105.11
  1      system  test      kbdiag_inj_idle_txn    local
  1      system  test      kbdiag_inj_long_query  local

not idle: 2
  pid     user    database  application            client  state                xact  query  wait             sql
  795860  system  test      kbdiag_inj_idle_txn    local   idle in transaction  3s    2s     -                select txid_current() as kbdiag_last, pg_sleep(1);
  795975  system  test      kbdiag_inj_long_query  local   active               0s    0s     Timeout:PgSleep  select pg_sleep(3600);
```

node1 主库，没有注入：

```
sessions  OK  (KingbaseES V008R006C009B0014, primary, system@local, 2026-09-26T19:26:39+08:00)

connected: 3 client sessions (not counting kbdiag), 1 walsender, 7 background
  count  user   database  application      client
  2      esrep  esrep     internal_rwcmgr  192.168.105.10
  1      esrep  esrep     internal_rwcmgr  192.168.105.11

not idle: 0
```

node2 备库：

```
sessions  OK  (KingbaseES V008R006C009B0014, standby, system@local, 2026-09-26T19:26:40+08:00)

connected: 2 client sessions (not counting kbdiag), 4 background
  count  user   database  application      client
  2      esrep  esrep     internal_rwcmgr  192.168.105.11

not idle: 0
```

`kbdiag_ro`（没有监控角色）连 node1，两个注入都在：别人会话的 state、类型、时间都被遮蔽，分不出是在干活、idle 还是后台进程。所以被遮蔽的会话单独算一类（hidden），照样进汇总，不假装它们是 idle。后台进程也混在里面，因为类型同样看不到：


```
sessions  UNKNOWN  (KingbaseES V008R006C009B0014, primary, kbdiag_ro@remote, 2026-09-26T19:26:46+08:00)

connected: 1 walsender, 12 hidden
  count  user    database  application            client
  3      esrep   esrep     internal_rwcmgr        ?
  1      -       -         auto vacuum            ?
  1      -       -         background flush       ?
  1      -       -         check pointer          ?
  1      -       -         wal flush              ?
  1      system  -         logical replication    ?
  1      system  -         sys_ksh collector      ?
  1      system  kingbase  -                      ?
  1      system  test      kbdiag_inj_idle_txn    ?
  1      system  test      kbdiag_inj_long_query  ?

not idle: 0 visible, 12 hidden
redacted: 12 rows of session.activity hide state, backend_type, client_addr, ages, wait, query (insufficient_privilege; grant sys_monitor)
```

排版规则和 status 一样：
- 时长用两个最大单位（`3s`、`6m 12s`、`2h 5m`）
- wait 只在 active 时显示，合成 `类型:事件`
- client 为空（走 socket）时写 `local`
- sql 截到终端宽度

`backend_xid`、`backend_xmin`、`state_age_s` 不进文本（视界归 txn，idle 时长写在 finding 里），JSON 里照旧都有。

### A.5 契约变更

- **推翻 2026-09-23 用户确认过的一条**："默认结果包含后台进程"改成默认不列，只在汇总里计数，`--all` 才列出来。理由：在 node1 上它们占了 11 行里的 7 行，却不回答任何一个场景
- 删 `--active`（alpha 阶段，直接删，不留兼容）
- **JSON 不变**：`session.activity` 的行还是全部会话，字段不变，只受 `--limit` 影响。汇总和"not idle"只是文本排版，不新增 probe
- `redacted[]` 在 JSON 里不变；文本里合成一行
- 连带修改：
  - status 的 `inst.connections` 下一步从 `kbdiag sessions --limit 0` 改成 `kbdiag sessions`：汇总已经回答了"谁占的"
  - slots 的下一步从"备库上看 sessions 里有没有 walreceiver"改成 `kbdiag status`：walreceiver 是后台进程，默认不再列出；status 的 `inst.upstream` 本来就回答这个问题
- 要同步的文档：PRD §4 表和 §5.1 sessions 示例、queries.md（开关列）、README、CLAUDE.md 的 sessions 输出形态一节、kbdiag-docs 的 sessions 页（代码进 main 之后）
