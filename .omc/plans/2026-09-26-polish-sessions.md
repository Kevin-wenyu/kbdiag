# 打磨 sessions 的输出

**状态**：pending user approval

**背景**：第二条打磨的命令，沿用 status 定下的全工具约定（级别、客观线、文本给人看、JSON 给脚本）。现状是在 node1 上一张 15 列的表：干净主库 11 行里 7 行是后台进程，没回答任何问题；想知道"连接是谁占的"要自己数。2026-09-26 19:26 在两个节点上实采（有注入和没注入的都采了），数据见 chronicle 同日"sessions 打磨：场景表和草样"一节。

## 1. 场景表（MECE）

用户跑 sessions 时想问的：

| # | 问题 | 现在怎么回答 | 改成 |
|---|---|---|---|
| S1 | 连接是谁占的（status 连接 FAIL 指过来的） | 自己在 15 列的表里数 | 默认给一段汇总：按 用户 / 库 / 应用 / 客户端 分组计数，数量从多到少 |
| S2 | 现在谁在干活、干了多久 | 表里混着 idle 和后台进程 | 默认只列不是 idle 的客户端会话（active、idle in transaction 等），按事务时长从长到短 |
| S3 | 谁开着事务不干活 | WARN `session.idle_in_txn` | 不变（见 §3） |
| S4 | 要看全部，包括 idle 会话和后台进程 | 默认就是全部 | `--all` |

**不在 sessions 里**（给出指向）：谁挡了谁 → `locks` / `session <pid>`；谁压着 vacuum 视界（xid/xmin）→ `txn`；大家在等什么 → `waits`；备库在不在收 WAL → `status`。

## 2. 参数（从场景推）

| 参数 | 场景 | 处理 |
|---|---|---|
| `--all` | S4 | 新增：列出全部会话，包括 idle 会话和后台进程 |
| `--limit N` | 会话很多时 | 保留，默认 50，只影响列表，判定和汇总覆盖全部 |
| `--idle-in-txn-warn S` | S3 | 保留，默认 300 |
| `--active` | — | **删除**：默认列表已经只看在干活的，它只多筛掉 idle in transaction，而后者正是要看的 |

不加 `--user`/`--app` 之类的过滤：汇总已经按这些分组了，要进一步筛用 `--json` 配 jq。

## 3. 判定

只保留一条：`session.idle_in_txn` **WARN**，事务开着却 idle 超过 300 秒（`--idle-in-txn-warn`）。

**为什么 300 秒可以保留**：这和 status 删掉的"连接数 80%"不一样。80% 连接对有的应用就是常态；idle in transaction 放着 5 分钟，无论应用怎么设计都是毛病（连接池泄漏、代码漏了 commit），而且它持有的锁和视界会越来越伤人。300 秒只是过滤噪声的下限，不是容量线。实验环境 `idle_in_transaction_session_timeout` 为 0（没设），所以没有服务器端的客观线可用。参数保留，因为 DBA 手工在 ksql 里开事务改数据是合理的例外。

**和 txn 的重叠**（逐行比过代码）：`rule.longTxn` 对所有会话判 `xact_age_s ≥ 300`。idle in transaction 的会话 `state_change` 不早于 `xact_start`，所以 sessions 报了，txn 在默认阈值下也一定报。两条 finding 的问题不一样：sessions 说"它闲着"，txn 说"事务太长"，各自命令能独立回答，所以保留。

## 4. 草样

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

## 5. 契约变更

- **推翻 2026-09-23 用户确认过的一条**："默认结果包含后台进程"改成默认不列，只在汇总里计数，`--all` 才列出来。理由：在 node1 上它们占了 11 行里的 7 行，却不回答任何一个场景
- 删 `--active`（alpha 阶段，直接删，不留兼容）
- **JSON 不变**：`session.activity` 的行还是全部会话，字段不变，只受 `--limit` 影响。汇总和"not idle"只是文本排版，不新增 probe
- `redacted[]` 在 JSON 里不变；文本里合成一行
- 连带修改：
  - status 的 `inst.connections` 下一步从 `kbdiag sessions --limit 0` 改成 `kbdiag sessions`：汇总已经回答了"谁占的"
  - slots 的下一步从"备库上看 sessions 里有没有 walreceiver"改成 `kbdiag status`：walreceiver 是后台进程，默认不再列出；status 的 `inst.upstream` 本来就回答这个问题
- 要同步的文档：PRD §4 表和 §5.1 sessions 示例、queries.md（开关列）、README、CLAUDE.md 的 sessions 输出形态一节、kbdiag-docs 的 sessions 页（代码进 main 之后）

## 6. 确认后的步骤

1. L1/L2：先写失败测试。三份 golden 手抄 §4 的草样；再加遮蔽和 `--all` 的 golden
2. 实现：`internal/report/sessions.go`，照 status 的模板；删 `--active`；改 status 和 slots 的 next
3. e2e：两节点都跑，`-count=1`
4. Codex 审查 → 用户看 VM 实跑输出 → kbdiag-docs 的 sessions 页 → 合进 main

## 7. 进度

- 2026-09-26：两节点实采，写出场景表和草样，等用户批准
