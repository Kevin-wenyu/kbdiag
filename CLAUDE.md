# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

kbdiag 2.0 — KingbaseES 命令行诊断工具，Go 重写（单个静态二进制，直连线协议），不进交互界面直接查实例状态。支持单机和主备集群（repmgr）。v0.1 只做简单的单次查询（会话、锁、事务、等待、实例概况、复制槽），连环分析放到后面。

当前进度看 `.omc/plans/` 里唯一的计划文件（目录为空表示没有进行中的阶段）；发生过什么看 `chronicle/`。v0.1 已于 2026-09-24 发布为 `v2.0.0-alpha.1`。

## 活文档（本表是唯一权威副本）

| 文档 | 唯一职责 |
|---|---|
| `README.md` | 使用者：安装、命令、示例 |
| `CLAUDE.md` | 设计理由、开发约定、KES 坑、本表 |
| `docs/PRD.md` | 需求、范围、输出契约、版本目标和验收、DS 场景表 |
| `docs/queries.md` | 查询清单（唯一的"版本"列）、probe_id、DS、SQL 出处、开关、验证状态 |
| `docs/engineering.md` | 选型、架构、测试分层、故障注入、运行命令 |

`docs/agents/` 是给 agent 技能读的配置说明，不算活文档。配套的用户手册在独立仓库 `kbdiag-docs`（Hugo 站点，中英双语），归它自己的仓库管，也不算本仓库的活文档：v2 的页面写在它的 `v2` 分支上，每条命令过了 VM 测试再写，示例用 VM 实跑结果并注明 Go commit。文档和代码保持同步（用户 2026-09-24 定）：代码推到 kbdiag main 后，对应的文档页也推到 kbdiag-docs 的 `v2` 分支，不在本地攒着；推 `v2` 不会触发站点部署（两个部署流程都只认 main）。每次发布时把 `v2` 合进它的 main（`v2.0.0-alpha.1` 起线上站点就是 v2 手册），所以 `v2` 上可以先放还没发布的命令页。`AGENTS.md` 是 Codex 的入口，只指向本文件，不算活文档。

**共同开发**：本项目由 Claude Code 和 Codex 共同开发（2026-09-23 用户确认），本文件是两边共用的规则来源。两边共用同一个计划入口（`.omc/plans/`）和同一份 chronicle，这样一方写的计划另一方一定能看到，也不会各留一份过期计划。一方审查另一方的产出，就是这个项目的对抗审查。

**上下文压缩与交接必须无损**：不得把压缩后的对话摘要当作项目状态或完成证据。阶段、范围、审批门槛以唯一计划为准；实际改动以工作区和 `git status` 为准；验证结果、已知缺口、决定和下一步以 chronicle 为准。执行 `/compact`、交接或跨会话续做前，先把尚未落盘的重要信息追加到当日 chronicle：当前阶段及结论、改动文件、实际运行的命令和结果（含环境/节点）、未验证项和已知缺口、未提交状态、下一安全动作、任何需要用户批准的门槛。已有事实不能只留在聊天或摘要里，也不能把未验证说成已验证。恢复上下文后，先读本文件、唯一计划的进度段和 chronicle 最新相关条目，再检查 `git status` 与实际文件；遇到摘要、记忆与文件矛盾时，以当前文件证据为准并记录纠正。禁止因压缩或交接自动跨过计划里的审批门槛；尤其 1.3 的切换、tag 和推送必须等用户明确确认。

**分工**：`docs/` 放当前事实；`CLAUDE.md` 放为什么这么设计；`.omc/plans/` 放要做的事；`chronicle/` 放发生了什么（调研和讨论也记在这里）。

### 防僵尸规则

1. 只有 5 个活文档都装不下的长期职责才允许新建文档，而且要先改上面的活文档表。
2. `docs/` 下的文件名不带日期。
3. 活文档标题下写 `状态: active | 最后核对: YYYY-MM-DD`。
4. 计划状态只有四种：`pending user approval` → `active` → `done` / `superseded`。done 当天把"为什么"写进本文件、过程写进 chronicle，然后删除计划；superseded 的计划立刻删除。`.omc/plans/` 最多 1 个 `.md`。
5. 活文档不引用 tag 里的具体路径（旧内容需要时从 tag 取，见文末"shell 版（冻结）"）。
6. 可执行检查（pre-commit 和 CI 都跑）：

```bash
test "$(find docs -name '*.md' -not -path 'docs/agents/*' | wc -l)" -eq 3 && test "$(find .omc/plans -name '*.md' 2>/dev/null | wc -l)" -le 1
```

## Architecture

详见 `docs/engineering.md` §4。要点：`cmd/kbdiag`（cobra，只做参数解析和组装）→ `internal/{conn,probe,facts,rule,scenario,report}`，单向依赖。

设计理由：
- **暂不建 `config` 包**（1.1 定）：v0.1 只有一个阈值，默认值放在 `rule.Defaults`，只用 flag 覆盖。等阈值多了、或者真有环境变量覆盖的需求再建（复杂度惩罚）。
- **`rule` 是纯函数，不 import `conn`/`probe`**：判定逻辑不需要 mock 数据库就能单测。旧版 mock SQL 返回值测出来的只是"我假设的数据库行为"，是测试不可信的根源。
- **`probe` 只采集不判断**：它的正确性只能靠真实 KES 验证（L3），所以不在它里面放任何业务逻辑。
- **直连线协议，不调 `ksql` 子进程**：旧 shell 版的引号地狱和 `set -e` 陷阱是重写的直接原因。
- **默认只读**：连接开 `default_transaction_read_only`，设 `lock_timeout`；诊断路径的代码里不出现 `pg_terminate_backend`（只读事务拦不住它）。
- **不假装 OK**：没采到（`skipped`/`error`）不能当成空结果判 OK；角色上不适用的用 `not_applicable`，不参与 verdict。
- **不假设 sudo**：以 `kingbase` 用户运行；每项检查在没有 repmgr 时都要能降级。
- **输出全部英文，没有 `--lang`**（用户 2026-09-26 定，取代 PRD 原先"finding 默认中文、`--lang en` 可选"）：help、列名、状态值、JSON 字段本来就是英文，只有 finding 的 symptom/note 和 probe 的 reason 是中文，同一屏里混排不合理；两套文案还要两套测试。代码注释里引用的 KES 手册章节名（如"动态性能视图"）不是输出，保留原文方便查手册。

### v0.1 范围（2026-09-23 定）

- **7 条单次查询命令，不做 P1 全量 19 条**：测试成本按数据源线性增长，7 条只用 6 个数据源；出事时第一问是会话、锁、谁压着视界，先回答这些，尽早让用户试用、定下形态。复制延迟和 repmgr 状态要先做注入和调研，推到后面。
- **扁平命令名，不按场景族分组**：场景族命名推到 v0.2 再评估。
- **tag 叫 `v2.0.0-alpha.1`**：`v1.0.0` 已被 shell 版占用；对内仍叫 v0.1。
- **L6 按 DS 场景验收、由 AI 在 VM 上执行**（2026-09-24 用户定）：用户关心工具满足了哪些场景，不关心实现步骤。

### 阶段 2 的取舍（2026-09-24）

- **每个等锁会话各出一条 `lock.waiting`**，不按挡路者合并：v0.1 只做单次查询，合并成阻塞链是 v0.2 以后 `locks --tree` 的事。
- **`wait_s` 用 `now()-state_change` 近似**：V8R6 的 `sys_locks` 没有 `waitstart`；这是上限，宁可多报几秒也不漏报。
- **2PC 挡路者 pid 是 0**：`sys_blocking_pids()` 对 prepared 事务返回 0，finding 里写成"未提交的两阶段事务"并指向 `kbdiag txn`，而不是让人去 `session 0`。
- **连上了但 Identify 失败给 UNKNOWN(3)**，不给 69：69 只表示连不上，否则监控脚本会把"库卡住了"误读成"网络断了"。
- **status 的连接数 = `sum(sys_stat_database.numbackends)`**：只数连到库的后端，和 `max_connections` 可比；后台进程不占这个额度。
- **status 看不到库大小（无 CONNECT 权限）不算 UNKNOWN**：大小不参与判定，只记进 `redacted[]`。
- **probe 通过 `facts.Context` 自己返回 `not_applicable`**（备库上的 2PC）：这是"能不能采"，不是业务判断，不违反"probe 只采集不判断"。

### 场景验收后的补丁（2026-09-24）

- **status 判连接数，分母是 `max_connections - superuser_reserved_connections`**：用满这部分时业务已经连不上、只剩超级用户能进，这就是 FAIL。按 `max_connections` 算会让"业务已经连不上"只显示 97%。（原来的 80% WARN 已在 2026-09-26 删除，见下节）
- **slots 的下一步指向备库上的 `kbdiag sessions`，不指向 `status`**：当时 status 没有 WAL 接收状态。status 打磨加了 `inst.upstream`、sessions 默认不再列后台进程之后，2026-09-26 改指 `kbdiag status`（见"sessions 打磨"）。
- **README 构建用 `git describe --match 'v2*'`**：不加的话会取到 shell 版的 `shell-final` tag，版本号像 `shell-final-5-g…`。

### status 打磨（2026-09-26）

- **只判两条，没有参数**：FAIL 只给"普通用户已经连不上"（已用 ≥ 可用），WARN 只给"备库没在收 WAL"（没有接收进程，或状态不是 `streaming`）。80% 的 WARN 和 `--conn-warn`/`--conn-fail` 删掉：多少算快满因应用而异，没有客观线；没人会调的阈值不做成参数。
- **`last_msg_age_s` 超过 `wal_receiver_timeout` 才判**（用户 2026-09-27 选 B，取代原来的"只展示不判"）：walreceiver 被暂停（SIGSTOP）或卡在内核里时状态仍是 `streaming`，只有 last_msg 在涨。线用服务器自己的：正常的接收进程在超时的一半时就向主库要回复（空闲的主库也会回），满了就断开重连，所以只有卡住的进程会越过它；实验环境是 30s，空闲时实测 8 秒前。和"没有接收进程""不是 streaming"同一个 id、同一个 WARN，evidence 多 `last_msg_age_s`、`wal_receiver_timeout_s`。`wal_receiver_timeout` 为 0（关闭）或读不到时不判。`slot.sh` 正好造出这种情况，所以这一支有 L4（未在 VM 上跑过）。已知的误报窗口：PG12 的接收进程在连接之前就把收到时间设成当时，空闲主库上第一条消息要等它自己在超时一半时要回复，所以连接本身慢于超时一半（多主机 conninfo 第一台不应、DNS 慢）或时间线切换后，可能短暂报一次，下一次就消失；不加余量，因为余量没有客观来源（阶段审查指出）。
- **`inst.upstream` 的 WARN 没在 KES 上验证就合并**（用户 2026-09-26 定）：能可靠造出"备库没在收 WAL"的办法都要动 sudo 或集群网络，比如改 `primary_conninfo` 要重启、会被 kbha 拉起，在主库上杀 walsender 后 5 秒就重连。判定本身只是"查询没返回行"和一次字符串比较，L1/L2 已经覆盖。等 slots 或复制延迟打磨需要复制中断注入时再补 L4。文档页如实写明这条 finding 是从源码摘的，不是实采。
- **`inst.disk` 是"一个 probe 一条 SQL"的例外**（v0.2 起还有 `space.disk`，见 space 小节）：KES 没有查磁盘剩余空间的函数，而磁盘满是库挂掉最常见的原因之一，status 又是第一个跑的命令，所以直接对 `data_directory` 做 statfs。只有确定跑在数据库主机上才读：走 socket，或者 host 是 localhost/127.0.0.1/::1 并且目录在本机能 stat（端口可能被转发到别的机器，所以只看 host 不够）；否则 `not_applicable`。它只展示、不参与 verdict，所以没读到也不会让结论变成 UNKNOWN。
- **`inst.upstream` 在主库上由 probe 自己报 `not_applicable`**：和备库上的 2PC 一样，是"能不能采"，不是业务判断。
- **downstreams 的 `sync_state` 不翻译**：repmgr 下实测是 `quorum`，不是 `sync`/`async`，翻译会丢信息。
- **status 有专用的文本排版**（`internal/report/status.go`）：单行数据用键值、大小按 1024 进位（和 `pg_size_pretty` 一致）、时长留两个最大单位、各段按问题的先后排（备库上游在前，主库下游在前）。JSON 不变形，保留字节和秒。后面 6 条命令打磨时照这个模板。

### locks 打磨（2026-09-26）

场景表：L1 有没有人在等锁；L2 谁是罪魁（挡的人最多）、它持有什么；L3 每个等锁的会话等什么、等多久、被谁直接挡住；L4 挡路者在干什么 → `kbdiag session <pid>`（finding 的 next）。不归 locks：多级链 → v0.2 `locks --tree`；长事务 → `txn`；大家在等什么事件 → `waits`。

- **文本先列 blockers，再列 waiting**：出事时第一问是"谁挡的"，而一个挡路者常挡住几十个会话，逐行看 waiting 数不出来。blockers 按挡住的会话数排，列出它在这些会话想要的对象上持有的锁；它自己也在排队时写 `(queued ahead for ...)`：`sys_blocking_pids()` 会把排在前面、锁模式冲突的等待者也算作挡路者。2PC 挡路者（pid 0）写成 `2PC`。
- **waiting 按等待时长排**，最久的在前；看不到时长（遮蔽、untracked）写 `?`。`--limit` 只裁 waiting 列表。
- **JSON 不变**：lock.list 仍是等锁的行加挡路者在同一对象上的锁。
- **`lock.waiting` 保留 WARN、10 秒和 `--lock-wait-warn`**（用户 2026-09-26 确认）：没有服务器端的客观线（`lock_timeout` 由各应用自己设，kbdiag 看不到；`deadlock_timeout` 是死锁检测的间隔，不是"等太久"）。等锁 10 秒对 OLTP 来说已经是事故，对批处理可能正常，所以不升 FAIL；10 秒是滤掉行锁瞬时争用的下限，不是容量线，和 idle in transaction 的 300 秒同一个道理。参数保留，因为批处理库会想调高。
- advisory 等没有 relation 的锁只按锁类型比对象（lock.list 没带 objid），同一挡路者的几个 advisory 锁会一起列在 holds 里。几个 2PC 同时挡路时合成一个 `2PC` 挡路者，holds 列出所有 pid 为 NULL 的持锁行。
- **挡路者去重**：`sys_blocking_pids()` 对每个挡路的 2PC 都报一个 0，并行查询时同一个 pid 也会出现多次；文本、finding 和 evidence 的 `blocker_pids` 都按 `Lock.Blockers()` 去重，JSON 的 `blocked_by` 保留原值。
- **blocks 数的是进程**：并行查询的 worker 有自己的 pid 和锁行，一条等锁的并行查询会被算成几个；lock.list 没有 leader pid 可以归并，已知限制。

### session \<pid\> 打磨（2026-09-26）

场景表：P1 这个会话是谁（用户、库、应用、客户端、类型）；P2 在干什么、干了多久、完整 SQL；P3 在等什么锁、被谁挡住；P4 挡住了谁；P5 持有哪些锁。不归它：压着视界多严重 → `txn`；全局谁挡得最多 → `locks`；终止会话 → 不做（只读）。

- **文本是键值块加 sql 块加三段**（waiting for / blocking / holds），段标题带数量；JSON 除了下面去掉的 next 以外不变。sql 不截断、保留原来的换行：这是唯一能看到完整 SQL 的命令。idle 的会话标题写 `last sql`（那条语句已经结束），state 看不到或 untracked 时仍写 `sql`。
- **看挡路者时 WARN 保留**（阶段 0 的发现）：`session <挡路者>` 带出等待者的 `lock.waiting`，它说的正是"这个会话挡住了别人 N 秒"，是挡路者自己的问题，所以保留 WARN（用户 2026-09-26 确认）。
- **指向自己的 next 去掉**：finding 的 next 如果是 `kbdiag session <当前 pid>`（挡路者看到的 lock.waiting、idle in transaction），读者已经在看它了；去掉后可以没有 next。
- **holds 不列自己的 virtualxid 和 transactionid**，除非有人在等同类型的锁：每个事务都持有这两把，列出来只是噪声；xid 在上面的键值块里。重复的行（子事务的多个 transactionid、同一张表的多个 tuple 锁）只列一次。
- **行锁写明类型**：有 relation 但不是表锁的（tuple、page）写成 `public.t (tuple)`，否则 `public.t ExclusiveLock` 会被读成表锁；locks 同样。
- **转义也覆盖 finding 行和格式字符**：finding 的症状、next、原因里引用的表名和 gid 同样转义；除 C0/C1 控制字符外，bidi 覆盖字符（U+202E、U+2066–2069）和 U+2028/2029 也显示成转义，防止完整 SQL 块被重新排序。
- 参数不变（`--lock-wait-warn`、`--idle-in-txn-warn`），判定复用 sessions 和 locks 的规则。

### txn 打磨（2026-09-26）

场景表：T1 谁压着 vacuum 视界（最老的 xid/xmin，谁持有）；T2 哪些事务开着、开了多久、在干什么；T3 有没有遗留的 2PC；T4 备库上 2PC 看不到 → `not_applicable`，去主库。不归 txn：idle in transaction 闲多久 → `sessions`；锁 → `locks`；复制槽的 xmin → `slots`。

- **长事务和 2PC 都只报 WARN**（用户 2026-09-26 确认）：按"FAIL = 业务已经受影响"，它们的危害是压住视界（表膨胀、2PC 还占着锁），是以后的事；它们挡住的会话由 locks 的 `lock.waiting` 报。删 `txn.long` 的 1800 秒 FAIL 和 `--xact-fail`；`txn.prepared` 从 FAIL 改 WARN，`--prepared-fail` 改名 `--prepared-warn`（alpha 阶段直接改，不留兼容）。
- **300 秒和 900 秒保留，参数保留**：没有服务器端的客观线；300 秒会碰上正常的批处理，所以参数留给批处理库调高。保留参数还有一个原因：e2e 要靠把阈值缩到 1 秒来造出 finding（engineering.md §6.5 A）。
- **文本先给 oldest xid**：`oldest xid: 6170  (801150 xid, 801314 xmin)`，按 2^32 取模比较（和服务器一样），列出持有这个值的所有会话和 2PC。再列开着的事务（有 xact_start、xid 或 xmin 的），最后是 2PC。
- **被遮蔽的会话照样列**：KES 对非监控账号不遮蔽 backend_xid/backend_xmin（阶段 0 实采），所以有 xid/xmin 的遮蔽会话仍然能说"它在事务里、压着多少"，其余列写 `?`；两者都没有的只计数（`N, M sessions hidden`：多半是 idle 会话，不能说成 M 个事务）。
- **oldest xid 不在缺数据时下结论**：activity 或 2PC 有一个没采到（备库 2PC 的 `not_applicable` 除外），就注明"可能有更老的"，一个持有者都没有时写 `unknown`。
- **gid 含控制字符时不给 ROLLBACK 语句**：终端上显示的是转义后的文本，照抄执行匹配不到；改成 `verify: kbdiag txn --json` 去取原始 gid。
- 和 sessions 的 `session.idle_in_txn` 在默认阈值下必然同时报（附录 A.3），两条回答的问题不同，都保留。

### waits 打磨（2026-09-26）

场景表：W1 此刻在干活的会话在等什么；W2 有没有大量会话堆在同一个等待事件上；W3 是哪些会话。不归 waits：谁挡的 → `locks`；单个会话详情 → `session <pid>`；历史等待 → KSH/KWR（v0.1 不做）。

- **只列在干活的组**：idle 会话的 `Client:ClientRead` 和后台进程的 `Activity:*` 占了原来输出的大半，却不回答"卡在哪"。`Activity` 类等待按 PG/KES 的定义是进程在主循环里空闲（没东西可发的 walsender、两次 checkpoint 之间的 checkpointer、KES 的 KSH 进程），不管 state 写什么都归后台；state 为空且没有等待事件的也归后台。**后台进程卡在真正的等待上（checkpointer 等 `IO:DataFileSync`、备库 startup 等 `BufferPin`）照样列出**，state 写 `(background)`：这正是 waits 要回答的"卡在哪"。它们之外合成一行 `not shown: N idle, M background, K hidden (state unknown), J untracked (state unknown)`：遮蔽和 untracked 的会话不知道是不是 idle，所以不算进 not idle。walsender 追赶时等的是 `IO:WALRead` 之类，会出现在列表里，空闲的主库上时有时无，属正常。
- **排序**：会话数多的在前（堆积最显眼），同数时 active 在前；active 却没有等待事件的写 `(running)`（在 CPU 上或这段代码没埋点）。pids 最多列 10 个，其余写 `... (+N)`，JSON 全有。
- **没有判定，没有参数**：等待事件本身没有客观线（同样 10 个会话等 IO，对一个库是事故，对另一个库是常态）；锁等太久由 locks 报。看不全（遮蔽、untracked）照旧 UNKNOWN。
- **后台进程在跑、没有等待事件时也归后台**（阶段 9 发现，用户 2026-09-26 选 A）：KES 的 `ksh writer` 定期跑 `metric_update_timer()`，被赶上时 state 是 active、没有等待事件，原来被当成客户端会话列成 `(running)`。`wait.summary` 加一个不进 JSON 的内部字段，列出每组里后台进程（不是 client backend 或 parallel worker，且没被遮蔽）的 pid，文本据此把它们移进 background。选内部字段而不是按后台/客户端拆组：拆组会改 JSON 的行。parallel worker 替客户端干活，不算后台。后台进程卡在真正的等待上仍照样列出。
- JSON 不变。

### slots 打磨（2026-09-26）

场景表：R1 有哪些槽、有没有人在消费；R2 每个槽保留了多少 WAL（磁盘）；R3 槽的 xmin / catalog_xmin 是否压着视界；R4 不活跃时下游还在不在 → `kbdiag status`（到下游上跑）。不归 slots：复制延迟数值 → v0.2；备库在不在收 WAL → `status`；会话压着的视界 → `txn`。

- **`slot.inactive` 从 FAIL 改 WARN**（用户 2026-09-26 确认）：槽不活跃时业务照常，危害（WAL 撑满磁盘、表膨胀、没有跟得上的备库）是以后的事，按"FAIL = 业务已经受影响"是 WARN。没有阈值、没有参数：不活跃本身就是客观线。repmgr 重启备库的那几秒也会报，属实。
- **WAL 量用人读的单位**：symptom 原来一律按 MB 取整，备库上 45 kB 写成"保留 0 MB WAL"；改用 `internal/units`（和 pg_size_pretty 一致），文本和 finding 共用。`internal/units` 是新包：rule 要用，又不能 import report。
- **逻辑槽的 catalog_xmin 写进 symptom 和 evidence**：它压着系统表的 vacuum。实验环境 wal_level=replica，建不了逻辑槽，只有 L1/L2。catalog xmin 列只在有槽带它时出现。
- **next 说"到这个槽的下游节点上运行"**：备库上也可以有槽（级联），原来的"在备库上运行"对它不对。
- 文本按不活跃在前、保留 WAL 多的在前排；JSON 行仍按槽名排。

### v0.2 共用约定（2026-09-27）

各节先写一行场景表和"不归它"的边界（阶段 1 的完整版在计划附录 B，计划 done 时随计划删除），再写每条命令的"为什么"，结构同 v0.1 各节。

- **纯展示的命令**（没有判定的：top-objects、top、wal 等；space 有了一条 FAIL 之后仍按这个规矩算 UNKNOWN）任何一个 probe 没采到、或有列看不到，verdict 就是 UNKNOWN（`rule.Display`）：OK 只表示"都采到了"，不表示数值好。有判定的命令照旧，只有判定的输入没采到才 UNKNOWN，只展示的 probe（像 `inst.disk`）不影响。
- 新命令的列表默认 20 行；列契约登记在 PRD §5.2（不再逐条写 JSON 示例）。
- 云会话写的部分只过了 L1/L2，VM 验证在计划的阶段 13。
- **PRD §5.2 由测试强制**（阶段 18）：每个 golden 和 fuzz 生成的 v0.2 报告，probe 的列、finding 的 evidence 字段都要和 §5.2 登记的完全一致；改列就得先改 PRD。fuzz 同时查服务器来的字符串都已转义，首行的 version 和 user 就是它找出来的漏网之鱼。
- **文本里提到的每条 `kbdiag …` 都要存在**（`TestMentionedCommandsExist` 扫 rule/scenario/report 的全部字符串，不只是 next 的 Command）：提示写错了读者照着敲就是用法错误。

### space（2026-09-27）

场景表：SP1 磁盘还剩多少、是哪块盘在满；SP2 哪个库最大；SP3 表空间各多大、在哪；SP4 WAL 目录多大、有没有超出配置该有的量。不归 space：表和索引 → `top-objects`；单表 → `table`；谁保留了 WAL → `slots`、`archive`、`wal`；数据目录那块盘的一行摘要 → `status`。

- **只判一条 FAIL：数据目录或 WAL 目录所在文件系统的可用空间不到一个 WAL 段**（`space.disk_full`，用户 2026-09-27 定）：WAL 所在盘上建不出新段，只能复用旧段，旧段用完实例就停；数据目录所在盘上表、事务状态文件、临时文件马上长不了（报 ERROR，实例不停）。symptom 按这个盘上有什么分别写（阶段审查指出原来一律说"实例会停"不对）。严格说是"马上就会受影响"，按用户定报 FAIL：离失败只差一个段，没有余地再观察。剩多少算少的其余情况因库而异，只展示。只看数据目录和 WAL：表空间所在盘满了只影响那些表，不让实例停，只展示。段大小来自 `space.wal`，kbdiag_ro 采不到，这时不判（本来就是 UNKNOWN）。每个文件系统一条，和文本一样按 `st_dev` 合并；avail 是非 root 可用的量，kingbase 用户就只能用这么多。L6 造不出来（要真把数据盘填满），只有 L1/L2。status 的 `inst.disk` 仍只展示，没改。**远程运行是 UNKNOWN，不是 OK**（审查 2026-10-07）：盘读不到，唯一的判定没做，说 OK 就是假装；status 不受影响，因为它的 `inst.disk` 本来就不参与判定。**某个表空间 statfs 失败（没挂载、目录删了）时保留数据目录和 WAL 两行照样判**：probe 记 error（JSON 照规矩不给行），能判出 FAIL 就是 FAIL，否则 UNKNOWN；原来整条 probe 报错，盘满了反而只给 UNKNOWN。
- **`space.disk` 是第二个 statfs 例外**：space 的问题就是"哪块盘在满"，而 `sys_wal` 常是指向另一块盘的符号链接，表空间也可以在别的盘上；只看数据目录（`inst.disk`）答不了。按目录的设备号（`st_dev`，跟随符号链接；statfs 的 fsid 在有些文件系统上是 0，靠不住）把同一文件系统上的目录合成一行；本机判断和 `inst.disk` 完全相同（复用它）。
- **表空间大小按权限用 CASE 包住**：kbdiag_ro 调 `pg_tablespace_size(sys_global)` 会让整条 SQL 失败（阶段 0 实采），包住之后看不到的只是 NULL，记 `redacted[]`。
- **WAL 只给参照，不下结论**：超过 `max_wal_size` 加 `wal_keep_segments × wal_segment_size` 时（PG12 的保留量大约是这两者加上最近检查点以来的 WAL，只超过其中一个是常态），文本提示去看 slots、archive；`max_wal_size` 是软上限，超出不等于故障。kbdiag_ro 调不了 `sys_ls_waldir`，这一块是 skipped，所以 kbdiag_ro 下 space 是 UNKNOWN。
- 库大小复用 `inst.databases`（同一 probe、同样的列），文本和 status 共用 `writeDatabases`。

### freeze（2026-09-27）

场景表：FZ1 离事务号回卷还有多远；FZ2 哪些表最老；FZ3 为什么冻结推不动 → next 指向 `txn`、`slots`；FZ4 别的库 → `kbdiag -d <库> freeze`。不归 freeze：谁压着视界 → `txn`、`slots`；死元组 → `vacuum`；单表 → `table`。

- **两条线都来自服务器，不设 kbdiag 自己的阈值**（修 GAP-6"冻结阈值两处来源不同"）：WARN 是 `autovacuum_freeze_max_age`（到这里 autovacuum 就该强制冻结，平时年龄会被压在线下，超过说明它正在跑或推不动）；FAIL 是停止线（PG12：xid 在回卷线 2^31 − 1 之前 100 万，multixact 之前 100；服务器拒绝分配，写事务失败）。停止线是 PG12 内核的常数（PG14 才改成 300 万，起草时写错过，审查纠正），KES V8R6 相同（2026-10-07 反汇编 `kingbase` 二进制核实：`TransactionIdLimitSet` 减 `0xf4240`，`MultiTransactionIdLimitSet` 减 `0x64`；在临时实例上靠改年龄实测走不通：KES 不让改目录表，强制 autovacuum 又会把年龄压回去），写在 `facts` 一处。
- **只按库判，表只展示**：库的年龄就是它最老的表；逐表出 finding 会把同一个问题报几十遍。当前库的表按年龄列出，别的库用 next 的 `kbdiag -d <库> freeze`。
- **表大小是估算**（`relpages × block_size`）：`pg_total_relation_size` 对每个表加 AccessShareLock，救回卷时常有的 VACUUM FULL、TRUNCATE 会让整条 probe 卡到 lock_timeout 失败。
- tables 只展示，没采到不影响 verdict；库名要转义或加引号时 next 的 `-d` 同 txn 对 gid 的做法处理。
- **排除 `relfrozenxid = 0`**：阶段 0 实采 `_kingbase_loginfo` 就是 0，`age(0)` 读成 2147483647，不排除会误报 FAIL。
- **备库照常报**：年龄是复制过来的，和主库相同；VACUUM 要到主库上跑，next 写明。
- limits 没采到时仍能判 FAIL（常数不依赖参数），年龄在停止线以下则 UNKNOWN。
- **报告上下文加了 `database`**（JSON `context.database`，`omitempty`，只增不改）：freeze、vacuum、top-objects 这类按库查询的命令要说清楚是哪个库，next 也要用它拼 `-d`。

### vacuum（2026-09-27）

场景表：VA1 哪些表死元组多、autovacuum 该不该管它了；VA2 autovacuum 开着吗；VA3 现在有没有 vacuum 在跑、跑到哪了；VA4 清不动的原因 → next 指向 `txn`、`slots`。不归 vacuum：膨胀估算 → `bloat`（待排）；冻结 → `freeze`；统计是否过时 → 待排；单表 → `table`。

- **只报"没人会清"的两种情况，都是 WARN**：`autovacuum` 或 `track_counts` 关了（`vacuum.disabled`）；表级 `autovacuum_enabled=off` 而死元组已过触发线（`vacuum.table_disabled`）。客观线是开关和服务器自己的触发线公式。
- **过线但 autovacuum 开着的不报，文本标 `due`**：那是 autovacuum 的正常队列，每个 naptime 轮一次；"过线很久没清"需要一条时间线，没有客观的（用户 2026-09-27 定：按建议不判）。
- **触发线在 rule 里算**（`rule.VacuumThreshold`，纯函数），probe 只给原样的 `reloptions`：不依赖 KES 上有没有 `pg_options_to_table`，也能单测各种写法。
- **备库不判**：表统计是节点本地的，备库全是 0（阶段 0 实采），autovacuum 也不在备库跑；表和进度 `not_applicable`，设置照样展示。
- **修复 SQL 里的名字按 quote_ident 加引号**（`rule.quoteIdent`，含关键字），带控制字符或格式字符（和文本转义的范围相同）时改指 `--json`（同 txn 的 gid）。
- **跟服务器算得一样**：触发线用 float4 算（`relation_needs_vacanalyze` 就是这样），`autovacuum_enabled` 按 parse_bool 认前缀，否则卡在线上的表会和 autovacuum 的判断不一致。
- **`track_counts=off` 时表的 probe 是 skipped**：计数器停了，旧值不能当成现在的（同 `track_activities` 的做法）。

### archive（2026-09-27）

场景表：AR1 归档开了吗、命令是什么；AR2 是不是在失败；AR3 积压了多少没归档的 WAL；AR4 积压撑大了 WAL 目录没有 → `space`。不归 archive：槽保留的 WAL → `slots`；备份是否完整 → 不做。

- **`archive.failing` 是 WARN，线是"最后一次尝试失败了"**：最后一次失败晚于最后一次成功，或从没成功过。WAL 堆在本节点、备份缺段，但业务照常。实验环境的归档本来就在失败，VM 上天然能测。
- **先排除主动配置**（queries.md 的归档注记）：`archive_mode=off` 不判；`archive_mode=on` 的备库本来就不归档，只有 `always` 才判。
- **`archive_command` 为空不判**：PG 文档说这时 WAL 会一直留着，但 KES 没实采，只在文本里写 `(empty)`（用户 2026-09-27 定按建议：VM 阶段核实 KES 的行为后再说）。
- `archive.ready`（`.ready` 个数和最老的等了多久）只展示：kbdiag_ro 调不了 `sys_ls_archive_statusdir`，不能让它影响结论。
- next 指向 `kbdiag space`（WAL 目录被撑到多大）和服务器日志：失败原因只在日志里，kbdiag 不读日志（K1 待排）。

### params（2026-09-27）

场景表：PA1 哪些参数不是默认值、在哪设的；PA2 哪些改了还没生效（要重启）；PA3 某个参数现在是多少 → 不加参数，`ksql -c 'show x'` 或 `--json` 配 jq。不归 params：调优建议（不收录）；可观测性开关是否就绪 → `ready`（待排）。

- **只列有人设过的参数**：去掉 default、override（编译或 initdb 定的）和 client/session（kbdiag 自己连接时带的参数就是 client，列出来是噪声）。
- **`params.pending_restart` 是 WARN，每个参数一条**：线是服务器自己的 `pending_restart` 列。现在跑的还是旧值，业务没受影响；但下一次重启（包括故障切换后的重启）会突然换成新值。
- **看不到来源文件的账号，没有 finding 时是 UNKNOWN**：kbdiag_ro 的 `sourcefile` 是 NULL，而且根本看不到超级用户专属参数（实采少 27 行），它们的 `pending_restart` 也就看不到。
- 不加 `params <pattern>`：看单个参数用 `ksql -c 'show x'` 或 `--json` 配 jq（用户 2026-09-27 定按建议不加）。配置文件有错（`sys_file_settings.error`）也按建议先不做：kbdiag_ro 调不了。
- **撤回的修复语句是把运行值写回**（`ALTER SYSTEM SET x = '<运行值>'` 再 reload）：PG12 的 reload 只在文件值等于运行值时清掉 pending 标志，只 `ALTER SYSTEM RESET` 会一直挂到重启（阶段 6 审查从 PG12 源码查出来的，注入脚本的 down 也照这个顺序）。
- 看不全的两处在文本里写明：kbdiag 自己连接设的四个参数（来源 client）看不到配置值；按角色、按库的设置只看得到当前角色和库的。

### repl（2026-09-27）

场景表：RP1（主库）各备库连着吗、落后多少；RP2（主库）同步复制够不够数；RP3（备库）在不在收 WAL（复用 `inst.upstream`）；RP4（备库）回放落后多少、是不是被暂停了；RP5（备库）walreceiver 是不是卡住了 → `inst.upstream`（last_msg 超过 `wal_receiver_timeout`）。不归 repl：槽保留的 WAL → `slots`；repmgr 眼里的集群 → `cluster`；"在不在复制"的一句话 → `status`。

- **三条 WARN，没有 FAIL**：备库没在收 WAL（复用 status 的 `inst.upstream` 规则）、回放被暂停（`repl.replay_paused`）、同步备库不够数（`repl.sync_short`）。最后一条本想报 FAIL（提交会卡住），但阶段 0 实采看到备库断开后同步提交照样过去了（怀疑 KES 或 repmgr 自动降级，未验证），所以 symptom 只说"要么在等，要么已经不等了，同步副本没有保证"。
- **延迟只展示**：没有服务器端的客观线；digoal 的 1 分钟/5 分钟是经验值。落后字节按本节点当前位置算，备库上按回放位置（同 slots）。
- **最近回放的事务多久前不当成延迟**：空闲主库上它能到 7 小时，而 LSN 完全追平（阶段 0 实采）；收到 = 回放时文本写 caught up。
- **卡住的 walreceiver 由 `inst.upstream` 报**：last_msg 超过 `wal_receiver_timeout` 而状态仍是 streaming（用户 2026-09-27 选 B，见 status 小节），repl 复用同一条规则。
- **有几个同步候选，数服务器给的 `sync_state`（sync/quorum）**，不自己按名单和 state 重算：服务器挑候选的规则（streaming 或 stopping、flush 有效、名字不分大小写）自己重做只会不一致。名单只用来解析要几个；解析不了的不猜，UNKNOWN。只在 `synchronous_commit` 让提交等备库时判（这个值是本连接的，按角色或库另设的看不到）。
- **OK 不代表提交在流动**：remote_apply 下候选停止回放，它仍是候选，只能从 replay_lag 看出来。所以回放暂停没有注入：在实验环境会卡住主库的所有提交（阶段 7 审查指出，删了注入脚本）。
- 备库上落后字节按收到和回放中较远的那个算：级联 walsender 发到那里。
- `sys_stat_wal_receiver.conninfo` 不采：可能带密码。

### cluster（2026-09-27）

场景表：CL1 repmgr 认为有哪些节点、谁是主、谁跟谁；CL2 repmgr 的看法和数据库自己的看法一致吗；CL3 最近出过什么事（最近 20 条事件）；CL4 有没有两个主。不归 cluster：节点实际能不能连上、repmgrd 在不在跑 → 到各节点跑 `kbdiag status`；落后多少 → `repl`；槽 → `slots`。

- **repmgr 的看法对照数据库自己的看法，四条都是 WARN**：两个 active 的 primary、inactive 节点、本节点类型和恢复角色不一致、主库上该挂上来的备库没挂上来。repmgr 按元数据做切换，不一致是隐患，业务此刻还没受影响；是不是真脑裂要到各节点上跑 status，所以不给 FAIL。
- **本节点先问 repmgr 的 `get_local_node_id()`，再按 `primary_slot_name` 认**：repmgr 给节点 N 的槽叫 `repmgr_slot_N`，在克隆或 rejoin 时写进节点 N 的 `primary_slot_name`（阶段 0 两节点都是；node1 的是当备库时留下的），从没当过备库的主库可能没有，所以先问函数（repmgrd 设的，可能没有，未实采）。认不出来就不做角色对照，verdict 不说 OK。
- **没给 `-d` 时连 `esrep`**：元数据在那个库，而 kbdiag 只占一个连接（N-02），不同时开两个连接。没有 `esrep` 库（3D000）时退回默认库，不给 69（69 只表示连不上实例）；当前库没有 repmgr schema 时 `not_applicable`（不是 repmgr 集群，或者元数据在别的库）。**`esrep` 拒绝这个用户（pg_hba、CONNECT 权限，任何服务器返回的错误）时也退回默认库**，但两个元数据 probe 记 skipped、写明 esrep 的错误，verdict UNKNOWN：实例是通的，不能给 69（审查 2026-10-07）。**显式给了 `-d` 而那个库没有 repmgr schema 时是 skipped（UNKNOWN），不是 not_applicable**：用户指名了元数据库，没找到就是没看到，不能说集群 OK；只有默认路径（没有 `esrep` 库）才能推断"不是 repmgr 集群"。
- **不调 repmgr 二进制**（和不调 ksql 同一个理由），所以 `repmgr cluster show` 的 Status 列（逐个连节点）做不了，文本和 next 指到各节点跑 `kbdiag status`。
- conninfo 不采：可能带密码。

### top-objects（2026-09-27）

场景表：TO1 库里空间被哪些表占了；TO2 哪些索引最大；TO3 大是因为数据、索引还是 TOAST。不归 top-objects：库、表空间、WAL → `space`；单表 → `table`；膨胀 → `bloat`（待排）；分区按父表汇总 → 以后。

- **只展示**：多大算大没有客观线。
- **用精确大小，不用 relpages 估算**（和 freeze 相反）：这条命令就是回答"现在谁占了空间"，批量导入后没 analyze 的表 relpages 还是旧的。代价是大小函数要加 AccessShareLock，碰上 VACUUM FULL 这类独占锁时整条 probe 在 lock_timeout 上 skipped（写明原因，不给部分结果）。
- 总大小拆成堆、索引、TOAST 三列，加起来等于总数（堆用 `pg_table_size` 减 TOAST，含 FSM/VM）：大是因为数据、索引还是大字段，处理办法不同。TOAST 表和它的索引不单列，算在父表里。
- 查询期间被删掉的表读成 NULL，外层过滤掉：ETL 反复建删临时表时整条 probe 不会因此失败。
- 命令名带连字符，README/PRD 的一致性测试的正则跟着改成 `[a-z-]+`。

### table \<name\>（2026-09-27）

场景表：TB1 多大、多少行；TB2 最近什么时候 vacuum/analyze 过；TB3 有哪些索引、用没用；TB4 冻结年龄；TB5 读得多吗、命中率；TB6 表级 autovacuum 被关了吗。不归 table：表上现在的锁 → `locks`；精确膨胀、列统计、分区 → 以后。

- **判定复用 freeze 和 vacuum 的规则，不另立**：年龄对照同样的两条线（id 换成 `freeze.table_age`，evidence 不同），表级关了 autovacuum 又过线同样是 `vacuum.table_disabled`。
- **名字按 SQL 规则解析**（`to_regclass`，参数绑定）：不带引号的折成小写、按 search_path 找，和用户在 ksql 里写的一致；不自己拆 schema 和名字。
- **找不到是 UNKNOWN（3），不是用法错误（64）**：表可能在别的库里（stderr 提示 `-d`），同 `session <pid>` 找不到 pid；只有空名字是 64。
- 备库上统计不适用（节点本地），索引扫描次数给 NULL 而不是 0；年龄照判（复制过来的）。
- 块命中率保留一位小数：几次读盘不该被四舍五入成 100%。
- **解析不加锁，大小单独一个 probe**（阶段 10 审查）：有人看这张表时，它常常正被 VACUUM FULL 之类锁住；大小函数等到 lock_timeout 时只丢大小，别的照常，文本指向 `kbdiag locks`。堆大小和 top-objects 用同一个定义（`pg_table_size` 减 TOAST），同一张表两处数字一致。**索引大小也只在 `table.size` 读到时才读**（审查 2026-10-07）：`pg_relation_size` 对每个索引加锁，VACUUM FULL、CLUSTER、TRUNCATE 独占着索引，原来索引列表也跟着丢；现在大小读不到时 `bytes` 是 NULL（文本 `-`），列表和定义照常。

### top（2026-09-27）

场景表：TP1 哪些 SQL 累计耗时最多；TP2 单次最慢、调用最多、读盘最多、写临时文件最多（`--by`）；TP3 统计开着吗、从什么时候算的。不归 top：此刻在跑的 → `sessions`；此刻在等什么 → `waits`；最近一分钟 → `top --interval`（待排）；执行计划 → 待排。

- **只展示**：哪条 SQL 算太贵没有客观线。每条给出占全部执行时间的比例，这是"数据库的时间花在哪"的直接答案。
- **标明是累计值**（修 GAP-5 的文案部分）：从上次重置算起，而这个版本没有 `sys_stat_statements_info`，重置时间拿不到，标题直说。
- **没在收集时 skipped 写明开关、UNKNOWN**，不给空表加 OK：实验环境就是 `track=none`。没装在当前库、没加载进 `shared_preload_libraries` 各有各的提示。但本连接的 `track=none` 不等于没有数据（角色、库可以自己设，`save=on` 留着旧数据），所以照样读，一行都没有才 skipped。
- **按 `sys_extension` 找到的 schema 读视图**，不走 search_path：别人在 `public` 或同名 schema 里放一个同名视图，就能让 kbdiag 报假数据。
- **`--by` 是排序，不是过滤**：场景 TP2（单次最慢、调用最多、读盘最多、临时文件最多）要它；时间单位按 SQL 的量级显示（0.09 ms、503 ms、1.75 s）。
- `top --interval`（最近 N 秒）不做，待排。

### progress（2026-09-27）

场景表：PG1 那个跑了很久的 VACUUM / CREATE INDEX / CLUSTER 到哪一步了；PG2 checkpoint 在跑吗、写到哪了；PG3 CREATE INDEX CONCURRENTLY 卡在哪 → `locks`。不归 progress：ANALYZE 和 basebackup 的进度（V8R6 没有视图）；vacuum 该不该跑 → `vacuum`；谁挡着 DDL → `locks`。

- **PG 的三个进度视图并成一个 probe**（UNION ALL，同一组列）：读者问的是"那个长操作到哪了"，不关心它在哪个视图里。KES 特有的 checkpoint 视图单独一个 probe：它只实采到列名，cast 失败时不能连累另三个。
- **已做/总量按阶段取**：块计数在扫描结束后停住，接着的回收、建索引的排序加载阶段要换计数，否则长时间显示 100%。
- **只展示**：多久算慢没有客观线；vacuum 该不该跑由 `vacuum` 判。
- CREATE INDEX CONCURRENTLY 还在等事务时，阶段后面写还剩几个、指向 `kbdiag locks`：这正是它"卡住"的常见原因。
- 没有 ANALYZE 和 basebackup 的进度视图（V8R6，阶段 0），空的时候文本写明查了哪几种。
- **被遮蔽的 CREATE INDEX、CLUSTER 行写成 `CREATE INDEX or REINDEX`、`CLUSTER or VACUUM FULL`**（审查 2026-10-07，VM 实测）：kbdiag_ro 看别人的这两种操作时 `command` 是 NULL，原来整条 probe 报错，连看得到的 VACUUM 也一起丢了；和遮蔽的 VACUUM 写 `VACUUM or autovacuum` 同一个做法。

### checkpoint（2026-09-27）

场景表：CK1 checkpoint 是定时触发还是被请求的；CK2 最近一次是什么时候、redo 在哪；CK3 脏页是谁写的；CK4 相关参数。不归 checkpoint：WAL 目录多大、谁留着 → `wal`、`space`；进行中的进度 → `progress`；日志里的 checkpoint 记录 → 待排。

- **只展示**：服务器自己的"checkpoint 太频繁"看的是两次 checkpoint 的间隔（`checkpoint_warning`），累计计数算不出间隔；被请求的占比只给数，不下结论。
- **脏页是谁写的**分三方给比例，但第三方写成"backends and others"：PG12 的 `buffers_backend` 还算关系扩展、VACUUM/COPY 的环形缓冲和备库的 startup 进程，空闲节点上也能占六七成（实采 65%、70%），不能读成"checkpointer 跟不上"（阶段 15 审查纠正）。`buffers_backend_fsync > 0` 只展示、不报 WARN（用户 2026-09-27 定按建议）：它表示 fsync 请求没能交给 checkpointer，队列满或 checkpointer 当时没在跑都会（node2 被 kbha 拉起过，实采 9）。
- **备库上不是 restartpoint 计数**：PG12 的 checkpointer 每次尝试都加一，没有新的 checkpoint 记录时每 15 秒试一次（node2 4 天多 15917 次），文本写"restartpoint attempts"、不给比例；控制文件里的时间是主库写那条 checkpoint 记录的时间，标题写明。
- "requested"不只是 WAL 量：还有手工 CHECKPOINT、基础备份（包括 repmgr clone）、promote、建删库。

### wal（2026-09-27）

场景表：WL1 现在写到哪了；WL2 WAL 目录多大（复用 `space.wal`）；WL3 谁让 WAL 留着：参数、槽、归档积压。不归 wal：生成速率 → `wal --interval`（待排）；槽不活跃的判定 → `slots`；归档失败的判定 → `archive`；checkpoint → `checkpoint`。

- **把"谁让 WAL 留着"放在一屏**：参数、每个槽、归档积压，DBA 不用分别跑 space、slots、archive 再拼起来。数据全部复用这三条命令的 probe（同一 probe、同样的列），只多一个 `wal.position`。
- **只展示，不重复判**：槽不活跃、归档失败各有自己的命令和 finding，这里只在那一行指过去；同一个问题在三条命令里各报一次只会让人以为出了三件事。
- 生成速率要隔一段时间采两次，单次查询给不了，不在这一版。

### seq（2026-09-27）

场景表：SQ1 有没有序列快用完；SQ2 哪些是 int/smallint 的；SQ3 会循环的（写 cycles，不算用完）。不归 seq：序列是 bigint 而那一列是 int 的错配（列先溢出，要查 `sys_depend`；用户 2026-09-27 定按建议先不做）；表的其他信息 → `table`。

- **FAIL 只给"取不出下一个值"**：nextval 报错，插入已经失败，线是序列自己的上/下限；会循环的不算。快用完只展示比例，没有客观线（用户 2026-09-27 定按建议不报 WARN）。
- **精确算**（`math/big`）：bigint 序列跨满 int64，普通相减会溢出。
- **NULL 要分开**：没权限读、从没调用过、`setval(..., false)` 之后都让 `last_value` 是 NULL（kbdiag_ro 实采全是 NULL）。SQL 按 oid 判权限（不按名字重解析，被删掉的序列不会让整条 probe 失败），看不到的记 `redacted[]`、UNKNOWN，提示授予序列的 SELECT（sys_monitor 解不开）；其余写 `no value yet`。
- **备库不判**：备库上的序列值是 WAL 里的副本，主库每次预写 32 个，会显示"到头了"而主库还有值（阶段 17 审查指出）。
- 修复语句按"是什么挡住了"给：序列自己的 MAXVALUE/MINVALUE 比类型小就挪它（`AS bigint` 不会动它，注入脚本 `maxvalue 3` 就是这种）；到了类型的边界，int/smallint 改 bigint（列先改，要重写表），bigint 只能换键。

### 三层深度（看 / 查 / 断）

保留为概念，不体现在命令分组上（PRD §4）：看 = 给一个确定事实；查 = 单维度深查，输出可机读，也用来验证"断"的结论；断 = 多维关联，输出症状→证据→根因→建议的链路。v0.1 只做看和查。

### `sessions` 输出形态（2026-09-23 定，2026-09-26 打磨）

- 按事务时长 `xact_age_s` 从长到短排列，空值排后，同值按 PID 升序（probe 的 SQL 排好，文本和 JSON 共用）。
- 默认最多显示 50 行；`--limit` 只裁文本列表和 JSON 行，判定和汇总覆盖全部会话。截断提示另报剩余行数，JSON 的 `truncated` 表示未显示的行数。
- 命令名、列名/顺序、JSON 字段名、finding.id 和 probe_id 以 PRD §5/§5.1 为契约。

### sessions 打磨（2026-09-26）

- **文本默认不列后台进程，只在汇总里计数**（推翻 2026-09-23 的"默认包含后台进程"，用户 2026-09-26 确认）：node1 上它们占 11 行里的 7 行，却不回答"连接是谁占的""谁在干活"任何一个问题。`--all` 才列全部；JSON 不变，总是全部会话。
- **删 `--active`**：默认列表已经只看不是 idle 的客户端会话；`--active` 只会再筛掉 idle in transaction，而那正是要看的。alpha 阶段直接删，不留兼容。不加 `--user`/`--app` 之类的过滤：汇总已经按它们分组，再细的筛选用 `--json` 配 jq。
- **汇总按 用户/库/应用/客户端 分组计数**：回答 status 连接 FAIL 指过来的"连接是谁占的"，所以 status 的下一步从 `kbdiag sessions --limit 0` 改成 `kbdiag sessions`。
- **untracked 会话照样列出，但时长显示 `?`**：state 是 `disabled`，读者一眼能看出状态不明；xact/query 时长是旧值（实测），不能当成当前值显示。
- **被遮蔽的会话单独算 hidden，照样进汇总**：非监控账号看不到别人会话的类型和状态，分不出在干活、idle 还是后台进程，不能假装它们是 idle；客户端显示 `?`，和 NULL 的 `-` 区分开。
- **`session.idle_in_txn` 保留 300 秒和参数**：idle in transaction 放 5 分钟无论应用怎么设计都是毛病（连接池泄漏、漏了 commit），和"连接数 80%"这种因应用而异的比例不同；300 秒是滤噪声的下限，不是容量线。DBA 手工开事务改数据是合理例外，所以参数留着。和 txn 的 `txn.long` 在默认阈值下必然同时报，但问题不同（"它闲着" vs "事务太长"），各自保留。
- **文本合成一行 `redacted`**：原来每个被遮蔽的列一行（9 行），现在按原因一行，列名折成 state、backend_type、client_addr、ages、wait、query；JSON 的 `redacted[]` 不变。
- **表格按终端列宽对齐**（`internal/report/table.go`）：`text/tabwriter` 按 rune 计宽，中文用户名、应用名会错位；改用 `golang.org/x/text/width`，东亚宽字符算 2 列。sql 仍截到 60 个字符，没按终端宽度截：输出常被管道和重定向，终端宽度拿不准（和附录 A 写的"截到终端宽度"不同，用户 2026-09-26 确认）。
- **slots 的下一步改指 `kbdiag status`**：walreceiver 是后台进程，sessions 默认不再列出；status 的 `inst.upstream` 本来就回答"在不在收 WAL"。2026-09-27 起它也报卡住的 walreceiver（last_msg 超过 `wal_receiver_timeout`），note 里原来"隔十几秒再跑一次"的提示随之删掉。

## KingbaseES 特有行为

- 系统视图前缀 `sys_`：`sys_stat_activity`、`sys_locks`、`sys_stat_replication` 等；函数 `sys_`/`pg_` 两套并存时优先 `sys_`
- Size 函数只有 `pg_` 前缀：`pg_relation_size`、`pg_total_relation_size`、`pg_database_size`
- 本地 socket 是 `/tmp/.s.KINGBASE.54321`，不是 `.s.PGSQL.54321`
- `database_mode=oracle`：`''` 当 NULL（`ora_input_emptystr_isnull=on`）；`||` 把 NULL 当空串（`NULL || '/'` 得 `'/'`），不能靠 `coalesce(a||b, fallback)` 兜底，要用 CASE 显式判断；`greatest`/`least` 有一个参数是 NULL 就返回 NULL（`greatest(1, null)` 是 NULL，2026-10-07 实测，PG 会忽略 NULL），同样用 CASE
- ksql 输出布尔值是 `t`/`f`（旧 shell 版 CLAUDE.md 写的 `true`/`false` 不对）；脚本比较时两种都接受
- interval 返回 KES 自有格式文本（`+000000002 17:10:03.47`）：一律在 SQL 里 `extract(epoch ...)` 转成秒数
- `sysdate` 不带时区：时间一律用 `now()` 或 timestamptz
- 备库上 `sys_current_wal_lsn()` 报错（recovery is progressing）：按角色改用 `sys_last_wal_replay_lsn()`
- 主库上的 2PC 事务在备库 `sys_prepared_xacts` 里查不到
- 非监控账号看别人的会话：`query` 显示 `<insufficient privilege>`，state/wait_event/时间戳/client_addr 等为 NULL，backend_xid/backend_xmin 仍可见；授予 `sys_monitor` 后解除
- WAL 目录是 `sys_wal`（不是 `pg_wal`）
- `pg_is_wal_receiver_up()` 不存在，用 `sys_stat_wal_receiver.status`
- `ksql -c "a; b"` 会把多条语句放进同一个事务块：`ROLLBACK PREPARED` 之类不能在事务块里跑的语句要单独调用

## Go / pgx 注意事项

- socket 连接用自定义 `DialFunc` 直连 `.s.KINGBASE` 路径；pgx 报错里仍显示 `.s.PGSQL`，conn 层要改写错误信息
- xid 扫描成 `uint32`
- `sys.date` 等非内置 OID 在 SQL 里显式 cast 成标准类型再返回

## Shell 脚本陷阱（`e2e/inject/`）

- macOS 自带 bash 3.2：`$(cat <<'EOF' ...)` 里的 `case` 模式 `)` 会被当成 `$(` 的结尾，改用前导括号 `(t | true)`
- 等待循环按 `SECONDS` 计真实时间，不按循环次数（每次调 ksql 都有开销）
- `set -e` 下 `((x++))` 结果为 0 会触发退出，改用 `x=$((x+1))`
- 每次修这类问题都配一个回归验证

## Development Environment

### Test VMs (Lima)

| Node | Role | Shell | SSH 端口 | Internal IP |
|------|------|-------|----------|-------------|
| kes-node1 | primary | `limactl shell kes-node1` | 51900 | 192.168.105.10 |
| kes-node2 | standby | `limactl shell kes-node2` | 51901 | 192.168.105.11 |

推荐 `limactl shell <node> sudo -iu kingbase <cmd>`，不受端口和 host key 变化影响。raw SSH 前先用 `limactl list --format '{{.Name}} {{.SSHLocalPort}}'` 核对端口；VM 重建后 host key 会变，需要 `ssh-keygen -R '[127.0.0.1]:<port>'`。

### KingbaseES 实例

- 端口 54321，OS 用户 `kingbase`，连接 `ksql test system`（本地 socket 免密；hba：local trust，host scram）
- 集群管理 repmgr（`repmgr cluster show`），复制用户 esrep；wal_level=replica（建不了逻辑槽），hot_standby_feedback=on（所以槽上有 xmin）
- node2 上 kbha 守护进程加每分钟 cron 会拉起停掉的实例
- `max_connections=100`，`superuser_reserved_connections=3`，`max_prepared_transactions=100`，`wal_sender_timeout=30s`
- 测试夹具：`kbdiag_ro`（密码 `kbdiag_ro_T3st`，不授监控角色），用于断言 `redacted[]`

## 开发工作流

```bash
go vet ./... && go test ./...                               # L1/L2，本机
KB_TEST_NODE=kes-node1 go test -tags vm ./e2e/...           # L3/L4/L5，主库视角
KB_TEST_NODE=kes-node2 go test -tags vm ./e2e/...           # 备库视角
KB_TEST_NODE=kes-node1 e2e/inject/<名>.sh up|down           # 手工注入故障
```

测试分层、防假绿机制、注入脚本清单见 `docs/engineering.md` §6–8。新 SQL 进代码前按 `docs/queries.md` 的"SQL 引入规则"过四关。

## Git / Deploy

- Remote: `https://github.com/Kevin-wenyu/kbdiag.git`
- Branch strategy: push directly to `main`
- Go 版 tag 从 `v2.0.0-alpha.1` 起；`v1.0.0` 是 shell 版最后一次发布

## Agent 技能工作流

**kbdiag 2.0 起统一使用 oh-my-claudecode（OMC）技能**（2026-09-23 确认）。之前 agent-skills 和 mattpocock/skills 按场景分工的做法停用，避免三套框架同时抢活；用户的目的之一是借这个项目完整学一遍 OMC。

**成本约定**：用户是订阅制。默认只走单 agent 的主线；会同时起多个 agent 的工作流（`ralplan`、`team`、`autopilot`、`ultragoal`）只在关键节点用，比如定版本范围、做架构决策、大批量实现，用之前先跟用户说一声。

### 主线：plan → execute → review → verify

| 阶段 | OMC 技能 / 角色 | 用途 | 成本 |
|------|------|------|------|
| 需求模糊，要对齐意图 | `/oh-my-claudecode:deep-interview` | 逐问澄清真实诉求，产出需求文档 | 低 |
| 出计划 | `/oh-my-claudecode:plan`（planner） | 分阶段计划，写入 `.omc/plans/` | 低 |
| 关键决策要共识 | `/oh-my-claudecode:ralplan` | planner、architect、critic 三方共识，取代以前手动做的对抗审查 | **高**，仅关键节点 |
| 实现 | `/oh-my-claudecode:execute`（executor），配合 `tdd` 关键词 | 薄切片实现，先写失败测试再改 | 中 |
| 审查 | `/oh-my-claudecode:review`（code-reviewer） | 提交前做代码审查；不能自己写完自己审 | 中 |
| 验证 | `/oh-my-claudecode:verify`（verifier） | 拿证据证明完成：测试输出、VM 实跑结果 | 中 |

### 辅助

| 场景 | OMC 技能 / 角色 | 说明 |
|------|------|------|
| 调研（对标工具、KES 官方文档） | document-specialist 角色 / `research` | KES 特有行为必须对照官方文档（help.kingbase.com.cn/v8）加 VM 实测，别凭记忆写 |
| 难缠 bug | debugger / tracer 角色 | 复现 → 定位 → 修复 → 加回归测试 |
| 去掉 AI 腔的冗余代码或文字 | `deslop` 关键词（ai-slop-cleaner） | 需要显式打出这个词才会触发 |
| 知识沉淀 | `/oh-my-claudecode:wiki`、`/oh-my-claudecode:remember` | 设计上的"为什么"仍然写进本文件 |
| 大批量、长时间自主执行 | `ralph` / `ultragoal` / `team` | **高成本**，要用户点头才用 |
| 结束执行模式 | `/oh-my-claudecode:cancel` | 做完并验证，或确认卡住时用 |

GitHub Issues 发布不再走技能，直接用 `gh issue create`。

### OMC + Jev（2026-09-23 用户确定）

开发流程是 OMC 加 TypeSafe 的 Jev，目的是质量更高、速度更快。原则是**能用上 Jev 的环节就用**。Jev 只做短小的类型化判断（是否、选哪个、打分，带概率），不写代码、不推理；API key 在环境变量 `TYPESAFE_API_KEY` 里。

| 环节 | Jev 判断什么 |
|------|------|
| 审查意见分级 | Codex 或 code-reviewer 的每条意见：成立 / 不成立 / 需复核，以及严重程度 |
| 文档与代码一致性 | 活文档里的一句描述（PRD、queries.md、engineering.md）和对应代码是否一致 |
| SQL 引入第 2 关 | KES 官方文档的摘录是否支持 SQL 里用到的视图、列和语义 |
| issue triage | 给 issue 打 triage 标签 |

边界：
- **不进 kbdiag 产品**：kbdiag 是跑在数据库主机上的离线二进制，主机通常不通外网；结论必须能追溯到确定证据，不能靠概率判断。
- **Jev 的判断只是分诊信号，不是证据**：定论仍然靠测试、VM 实跑和人工核对。Jev 判"不成立"的审查意见也要看一眼再放掉。
- 某个环节第一次用时，先读 TypeSafe 官方文档（docs.typesafe.ai），建最小脚本；把 Jev 的判断和最终核对结果一起记进 chronicle，用来检验它准不准。审查意见分级用 `scripts/jev_review.py`（只用标准库）；证据要包含意见依赖的全部代码，只给一半时它会五五开。

## Agent skills

### Issue tracker

GitHub Issues on `Kevin-wenyu/kbdiag`（`gh` CLI）；PR 不作为 triage 请求面（单人直推 main，无外部贡献者）。详见 `docs/agents/issue-tracker.md`。

### Triage labels

沿用 mattpocock/skills 默认命名（`needs-triage`/`needs-info`/`ready-for-agent`/`ready-for-human`/`wontfix`）。详见 `docs/agents/triage-labels.md`。

### Domain docs

领域术语和决策以上面的活文档为准：术语和契约在 `docs/PRD.md`，设计理由在本文件。不另建 `CONTEXT.md` 或 ADR 目录。详见 `docs/agents/domain.md`。

## shell 版（冻结）

旧 shell 版（v1.x）已冻结，不再维护，全部内容保存在 tag `shell-final`，需要时用 `git show shell-final:<路径>` 取。
