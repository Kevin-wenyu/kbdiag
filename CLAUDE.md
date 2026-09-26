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
- **`last_msg_age_s` 只展示不判**：空闲的主库每 `wal_receiver_status_interval`（默认 10s）才发一条，实测 8 秒前是正常值。代价是 walreceiver 被暂停（SIGSTOP）时状态仍是 `streaming`，status 不报；要判就得定一条客观线（比如超过 `wal_receiver_timeout`），留给以后。
- **`inst.upstream` 的 WARN 没在 KES 上验证就合并**（用户 2026-09-26 定）：能可靠造出"备库没在收 WAL"的办法都要动 sudo 或集群网络，比如改 `primary_conninfo` 要重启、会被 kbha 拉起，在主库上杀 walsender 后 5 秒就重连。判定本身只是"查询没返回行"和一次字符串比较，L1/L2 已经覆盖。等 slots 或复制延迟打磨需要复制中断注入时再补 L4。文档页如实写明这条 finding 是从源码摘的，不是实采。
- **`inst.disk` 是"一个 probe 一条 SQL"的唯一例外**：KES 没有查磁盘剩余空间的函数，而磁盘满是库挂掉最常见的原因之一，status 又是第一个跑的命令，所以直接对 `data_directory` 做 statfs。只有确定跑在数据库主机上才读：走 socket，或者 host 是 localhost/127.0.0.1/::1 并且目录在本机能 stat（端口可能被转发到别的机器，所以只看 host 不够）；否则 `not_applicable`。它只展示、不参与 verdict，所以没读到也不会让结论变成 UNKNOWN。
- **`inst.upstream` 在主库上由 probe 自己报 `not_applicable`**：和备库上的 2PC 一样，是"能不能采"，不是业务判断。
- **downstreams 的 `sync_state` 不翻译**：repmgr 下实测是 `quorum`，不是 `sync`/`async`，翻译会丢信息。
- **status 有专用的文本排版**（`internal/report/status.go`）：单行数据用键值、大小按 1024 进位（和 `pg_size_pretty` 一致）、时长留两个最大单位、各段按问题的先后排（备库上游在前，主库下游在前）。JSON 不变形，保留字节和秒。后面 6 条命令打磨时照这个模板。

### locks 打磨（2026-09-26）

场景表：L1 有没有人在等锁；L2 谁是罪魁（挡的人最多）、它持有什么；L3 每个等锁的会话等什么、等多久、被谁直接挡住；L4 挡路者在干什么 → `kbdiag session <pid>`（finding 的 next）。不归 locks：多级链 → v0.2 `locks --tree`；长事务 → `txn`；大家在等什么事件 → `waits`。

- **文本先列 blockers，再列 waiting**：出事时第一问是"谁挡的"，而一个挡路者常挡住几十个会话，逐行看 waiting 数不出来。blockers 按挡住的会话数排，列出它在这些会话想要的对象上持有的锁；它自己也在排队时写 `(queued ahead for ...)`：`sys_blocking_pids()` 会把排在前面、锁模式冲突的等待者也算作挡路者。2PC 挡路者（pid 0）写成 `2PC`。
- **waiting 按等待时长排**，最久的在前；看不到时长（遮蔽、untracked）写 `?`。`--limit` 只裁 waiting 列表。
- **JSON 不变**：lock.list 仍是等锁的行加挡路者在同一对象上的锁。
- **`lock.waiting` 保留 WARN、10 秒和 `--lock-wait-warn`**（待用户确认）：没有服务器端的客观线（`lock_timeout` 由各应用自己设，kbdiag 看不到；`deadlock_timeout` 是死锁检测的间隔，不是"等太久"）。等锁 10 秒对 OLTP 来说已经是事故，对批处理可能正常，所以不升 FAIL；10 秒是滤掉行锁瞬时争用的下限，不是容量线，和 idle in transaction 的 300 秒同一个道理。参数保留，因为批处理库会想调高。
- advisory 等没有 relation 的锁只按锁类型比对象（lock.list 没带 objid），同一挡路者的几个 advisory 锁会一起列在 holds 里。几个 2PC 同时挡路时合成一个 `2PC` 挡路者，holds 列出所有 pid 为 NULL 的持锁行。
- **挡路者去重**：`sys_blocking_pids()` 对每个挡路的 2PC 都报一个 0，并行查询时同一个 pid 也会出现多次；文本、finding 和 evidence 的 `blocker_pids` 都按 `Lock.Blockers()` 去重，JSON 的 `blocked_by` 保留原值。
- **blocks 数的是进程**：并行查询的 worker 有自己的 pid 和锁行，一条等锁的并行查询会被算成几个；lock.list 没有 leader pid 可以归并，已知限制。

### session \<pid\> 打磨（2026-09-26）

场景表：P1 这个会话是谁（用户、库、应用、客户端、类型）；P2 在干什么、干了多久、完整 SQL；P3 在等什么锁、被谁挡住；P4 挡住了谁；P5 持有哪些锁。不归它：压着视界多严重 → `txn`；全局谁挡得最多 → `locks`；终止会话 → 不做（只读）。

- **文本是键值块加 sql 块加三段**（waiting for / blocking / holds），段标题带数量；JSON 除了下面去掉的 next 以外不变。sql 不截断、保留原来的换行：这是唯一能看到完整 SQL 的命令。idle 的会话标题写 `last sql`（那条语句已经结束），state 看不到或 untracked 时仍写 `sql`。
- **看挡路者时 WARN 保留**（阶段 0 的发现）：`session <挡路者>` 带出等待者的 `lock.waiting`，它说的正是"这个会话挡住了别人 N 秒"，是挡路者自己的问题，所以保留 WARN（待用户确认）。
- **指向自己的 next 去掉**：finding 的 next 如果是 `kbdiag session <当前 pid>`（挡路者看到的 lock.waiting、idle in transaction），读者已经在看它了；去掉后可以没有 next。
- **holds 不列自己的 virtualxid 和 transactionid**，除非有人在等同类型的锁：每个事务都持有这两把，列出来只是噪声；xid 在上面的键值块里。重复的行（子事务的多个 transactionid、同一张表的多个 tuple 锁）只列一次。
- **行锁写明类型**：有 relation 但不是表锁的（tuple、page）写成 `public.t (tuple)`，否则 `public.t ExclusiveLock` 会被读成表锁；locks 同样。
- **转义也覆盖 finding 行和格式字符**：finding 的症状、next、原因里引用的表名和 gid 同样转义；除 C0/C1 控制字符外，bidi 覆盖字符（U+202E、U+2066–2069）和 U+2028/2029 也显示成转义，防止完整 SQL 块被重新排序。
- 参数不变（`--lock-wait-warn`、`--idle-in-txn-warn`），判定复用 sessions 和 locks 的规则。

### txn 打磨（2026-09-26）

场景表：T1 谁压着 vacuum 视界（最老的 xid/xmin，谁持有）；T2 哪些事务开着、开了多久、在干什么；T3 有没有遗留的 2PC；T4 备库上 2PC 看不到 → `not_applicable`，去主库。不归 txn：idle in transaction 闲多久 → `sessions`；锁 → `locks`；复制槽的 xmin → `slots`。

- **长事务和 2PC 都只报 WARN**（待用户确认）：按"FAIL = 业务已经受影响"，它们的危害是压住视界（表膨胀、2PC 还占着锁），是以后的事；它们挡住的会话由 locks 的 `lock.waiting` 报。删 `txn.long` 的 1800 秒 FAIL 和 `--xact-fail`；`txn.prepared` 从 FAIL 改 WARN，`--prepared-fail` 改名 `--prepared-warn`（alpha 阶段直接改，不留兼容）。
- **300 秒和 900 秒保留，参数保留**：没有服务器端的客观线；300 秒会碰上正常的批处理，所以参数留给批处理库调高。保留参数还有一个原因：e2e 要靠把阈值缩到 1 秒来造出 finding（engineering.md §6.5 A）。
- **文本先给 oldest xid**：`oldest xid: 6170  (801150 xid, 801314 xmin)`，按 2^32 取模比较（和服务器一样），列出持有这个值的所有会话和 2PC。再列开着的事务（有 xact_start、xid 或 xmin 的），最后是 2PC。
- **被遮蔽的会话照样列**：KES 对非监控账号不遮蔽 backend_xid/backend_xmin（阶段 0 实采），所以有 xid/xmin 的遮蔽会话仍然能说"它在事务里、压着多少"，其余列写 `?`；两者都没有的只计数（`N known, M hidden`）。
- 和 sessions 的 `session.idle_in_txn` 在默认阈值下必然同时报（附录 A.3），两条回答的问题不同，都保留。

### waits 打磨（2026-09-26）

场景表：W1 此刻在干活的会话在等什么；W2 有没有大量会话堆在同一个等待事件上；W3 是哪些会话。不归 waits：谁挡的 → `locks`；单个会话详情 → `session <pid>`；历史等待 → KSH/KWR（v0.1 不做）。

- **只列在干活的组**：idle 会话的 `Client:ClientRead` 和后台进程的 `Activity:*` 占了原来输出的大半，却不回答"卡在哪"。`Activity` 类等待按 PG/KES 的定义是进程在主循环里空闲（walsender、checkpointer 等），归后台；state 为空的也归后台。它们合成一行 `not shown: N idle, M background`。
- **排序**：会话数多的在前（堆积最显眼），同数时 active 在前；active 却没有等待事件的写 `(running)`（在 CPU 上或这段代码没埋点）。pids 最多列 10 个，其余写 `... (+N)`，JSON 全有。
- **没有判定，没有参数**：等待事件本身没有客观线（同样 10 个会话等 IO，对一个库是事故，对另一个库是常态）；锁等太久由 locks 报。看不全（遮蔽、untracked）照旧 UNKNOWN。
- JSON 不变。

### 三层深度（看 / 查 / 断）

保留为概念，不体现在命令分组上（PRD §4）：看 = 给一个确定事实；查 = 单维度深查，输出可机读，也用来验证"断"的结论；断 = 多维关联，输出症状→证据→根因→建议的链路。v0.1 只做看和查。

### `sessions` 输出形态（2026-09-23 定，2026-09-26 打磨）

- 按事务时长 `xact_age_s` 从长到短排列，空值排后，同值按 PID 升序（probe 的 SQL 排好，文本和 JSON 共用）。
- 默认最多显示 50 行；`--limit` 只裁文本列表和 JSON 行，判定和汇总覆盖全部会话。截断提示另报剩余行数，JSON 的 `truncated` 表示未显示的行数。
- 命令名、列名/顺序、JSON 字段名、finding.id 和 probe_id 以 PRD §5/§5.1 为契约。

### sessions 打磨（2026-09-26）

- **文本默认不列后台进程，只在汇总里计数**（推翻 2026-09-23 的"默认包含后台进程"，待用户在检查点确认）：node1 上它们占 11 行里的 7 行，却不回答"连接是谁占的""谁在干活"任何一个问题。`--all` 才列全部；JSON 不变，总是全部会话。
- **删 `--active`**：默认列表已经只看不是 idle 的客户端会话；`--active` 只会再筛掉 idle in transaction，而那正是要看的。alpha 阶段直接删，不留兼容。不加 `--user`/`--app` 之类的过滤：汇总已经按它们分组，再细的筛选用 `--json` 配 jq。
- **汇总按 用户/库/应用/客户端 分组计数**：回答 status 连接 FAIL 指过来的"连接是谁占的"，所以 status 的下一步从 `kbdiag sessions --limit 0` 改成 `kbdiag sessions`。
- **untracked 会话照样列出，但时长显示 `?`**：state 是 `disabled`，读者一眼能看出状态不明；xact/query 时长是旧值（实测），不能当成当前值显示。
- **被遮蔽的会话单独算 hidden，照样进汇总**：非监控账号看不到别人会话的类型和状态，分不出在干活、idle 还是后台进程，不能假装它们是 idle；客户端显示 `?`，和 NULL 的 `-` 区分开。
- **`session.idle_in_txn` 保留 300 秒和参数**：idle in transaction 放 5 分钟无论应用怎么设计都是毛病（连接池泄漏、漏了 commit），和"连接数 80%"这种因应用而异的比例不同；300 秒是滤噪声的下限，不是容量线。DBA 手工开事务改数据是合理例外，所以参数留着。和 txn 的 `txn.long` 在默认阈值下必然同时报，但问题不同（"它闲着" vs "事务太长"），各自保留。
- **文本合成一行 `redacted`**：原来每个被遮蔽的列一行（9 行），现在按原因一行，列名折成 state、backend_type、client_addr、ages、wait、query；JSON 的 `redacted[]` 不变。
- **表格按终端列宽对齐**（`internal/report/table.go`）：`text/tabwriter` 按 rune 计宽，中文用户名、应用名会错位；改用 `golang.org/x/text/width`，东亚宽字符算 2 列。sql 仍截到 60 个字符，没按终端宽度截：输出常被管道和重定向，终端宽度拿不准（和附录 A 写的"截到终端宽度"不同，待用户确认）。
- **slots 的下一步改指 `kbdiag status`**：walreceiver 是后台进程，sessions 默认不再列出；status 的 `inst.upstream` 本来就回答"在不在收 WAL"。它仍然看不出被暂停的 walreceiver（状态停在 streaming），所以 note 里提示隔十几秒再跑一次，看 last_msg 是否还在涨。

## KingbaseES 特有行为

- 系统视图前缀 `sys_`：`sys_stat_activity`、`sys_locks`、`sys_stat_replication` 等；函数 `sys_`/`pg_` 两套并存时优先 `sys_`
- Size 函数只有 `pg_` 前缀：`pg_relation_size`、`pg_total_relation_size`、`pg_database_size`
- 本地 socket 是 `/tmp/.s.KINGBASE.54321`，不是 `.s.PGSQL.54321`
- `database_mode=oracle`：`''` 当 NULL（`ora_input_emptystr_isnull=on`）；`||` 把 NULL 当空串（`NULL || '/'` 得 `'/'`），不能靠 `coalesce(a||b, fallback)` 兜底，要用 CASE 显式判断
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
