# v0.2：巡检五条 + 复制两条 + 对象和 SQL 三条（云会话长跑 + 本地 VM 收口）

**状态**：active（用户 2026-09-27 定："都做了吧，主要是考虑场景"；云会话额度快到期，按上一轮的方式交给云会话长跑；同日用户追加"我要的继续是开发，任务量可以更多"，把 v0.2 剩下的 top-objects、table、top 也加进来）

**范围**：10 条新命令，都在 `docs/queries.md` 标为 v0.2：

| 组 | 命令 | queries.md | 一句话 |
|---|---|---|---|
| 巡检 | `space` | F1 | 库、表空间、WAL 目录多大，磁盘还剩多少 |
| 巡检 | `freeze` | G2 | 离事务号回卷还有多远 |
| 巡检 | `vacuum` | G1 | 哪些表死元组多、多久没 vacuum、现在有没有 autovacuum 在跑 |
| 巡检 | `archive` | H2 | 归档是不是在正常工作 |
| 巡检 | `params` | A2 | 哪些参数不是默认值、哪些改了还没生效 |
| 复制 | `repl` | I1+I4 | 主库上看各备库的延迟；备库上看接收和回放 |
| 复制 | `cluster` | I3 | repmgr 眼里的集群：节点、角色、上游、状态 |
| 对象 | `top-objects` | F2 | 最大的表和索引（含 TOAST） |
| 对象 | `table <t>` | F3 | 单表概况：大小、行数、死元组、最近 vacuum/analyze、索引、年龄 |
| SQL | `top` | D2 | 累计 Top SQL（标明是累计值；统计没开时明说） |

**不在范围**：`conn`（B3，sessions 的汇总已经回答了"连接是谁占的"，先不做）、`top --interval`（D2b，待排）、`kill`、`--dump-facts`，以及 queries.md 里"待排"和"以后"的条目。

## 0. 云会话必读（开工前）

1. 先读 `CLAUDE.md`（全部，特别是各"打磨"小节：新命令的排版和判定照它们来），再读本计划、`chronicle/` 最新两天、`docs/PRD.md` §4/§5/§10、`docs/queries.md`（追溯表、F/G/H/I/A 节、"SQL 引入规则"、"默认阈值参考"）。代码模板看 `internal/report/status.go`、`internal/report/slots.go` 和对应 golden。
2. **云容器连不到 Lima VM**。能做的只有 L1/L2（`go vet`、`go test`、`-race`、交叉编译、`go vet -tags vm ./e2e/` 只编译）。所有 VM 相关的结论写成"未验证"，列进该阶段的"VM 待验清单"，**不许写成已验证**。
3. **KES 行为只认三处证据**：`CLAUDE.md` 的"KingbaseES 特有行为"、`e2e/testdata/captures/v02/`（本地阶段 0 的实采，见 §3）、KES V8R6 官方文档（help.kingbase.com.cn/v8，引用时写明章节）。实采里没有的行为不许按 PG 猜着写进断言；拿不准的写进 VM 待验清单，e2e 断言写宽或先不写。
4. **阶段 0 的实采可能比你开工晚**：阶段 1 不需要它。开始阶段 2 前 `git fetch` 并把 `origin/main` 合进你的分支，确认 `e2e/testdata/captures/v02/` 已经在了；还没有就先做不依赖实采的部分，别编数据。
5. **只推云会话指定的分支**，不推 main、不打 tag、不碰 kbdiag-docs。
6. `TYPESAFE_API_KEY` 没有就跳过 Jev，在 chronicle 里注明，回本地再补。
7. 不起多 agent 工作流（ralplan/team/autopilot/ultragoal）。单 agent 主线加上每阶段一个 code-reviewer 审查。
8. 输出全部英文（help、文本、finding、reason），没有 `--lang`。
9. **续做**：会话中断或回合结束后，用户只会发"继续"。收到后先看 §5 最后一行和 `git log`，从断点接着做，不要重新规划，也不要问用户做什么。每做完一个阶段，就在 §5 加一行（阶段、提交号、下一步）。

## 1. 场景先行（用户 2026-09-27 强调："主要是考虑场景"）

**阶段 1 只写场景，不写代码**，10 条命令的场景表一次写完，写进本计划的附录 B（每条命令一节），提交推送后再往下做。每节包含：

1. **场景表（MECE）**：DBA 在什么情况下会跑这条命令、想问什么（编号，如 SP1、FZ1、VA1、AR1、PA1、RP1、CL1）。每个问题写：现在 kbdiag 能不能回答（哪条命令）、这条命令怎么回答。
2. **不归它管的问题**：指向哪条已有或新命令。重点写清和已有命令的边界：
   - `space` ↔ `status` 的 `inst.disk`（status 只看数据目录所在磁盘，一行；space 是完整的空间账）
   - `freeze` ↔ `txn` 的 oldest xid（txn 回答"谁压着视界"，freeze 回答"离回卷还有多远"；freeze 的 finding 可以指向 txn、slots 找压着的人）
   - `vacuum` ↔ `txn`/`slots`（vacuum 清不动的原因常是长事务或槽的 xmin，指过去，不自己查）
   - `repl` ↔ `status` 的 `inst.upstream`/`inst.downstreams` ↔ `slots`（status 回答"在不在复制"，repl 回答"落后多少"，slots 回答"保留了多少 WAL"）
   - `cluster` ↔ `repl`（cluster 是 repmgr 的看法，repl 是数据库自己的看法；两者不一致本身就是线索）
   - `top-objects` ↔ `space`（space 是库、表空间、WAL 这一级的账，top-objects 往下一层到表和索引）
   - `table <t>` ↔ `vacuum`/`freeze`/`locks`/`top-objects`（table 是单表的全貌；判定复用 vacuum、freeze 的规则，只对这张表，不另立规则；表上的锁指向 `locks`）
   - `top` ↔ `sessions`/`waits`（sessions、waits 是此刻，top 是从统计起点到现在的累计；`top --interval` 不做）
3. **参数**：从场景推，没有场景对应的不加。不加过滤参数，要筛用 `--json` 配 jq。
4. **判定**：FAIL = 业务已经受影响；WARN = 还没受影响，但不处理会出事。只用客观线（服务器参数、硬上限）；没人会调的阈值不做成参数。每条 finding 写：id、级别、客观线是什么、为什么是这个级别、next 指向哪。没有客观线的就只展示不判（像 waits）。候选判定（都要论证，不是定论）：
   - `freeze`：库或表的年龄逼近 `autovacuum_freeze_max_age`（autovacuum 已经该强制冻结了）、逼近 20 亿（服务器会拒绝分配 xid）
   - `vacuum`：死元组超过 autovacuum 的触发线却很久没 vacuum 过（触发线由 `autovacuum_vacuum_threshold` + `scale_factor` × 行数算，表级 reloptions 可以覆盖）
   - `archive`：`archive_mode` 开着但最后一次失败晚于最后一次成功（实验环境正是这样，见 §3）
   - `params`：`pending_restart = true`
   - `repl`：`synchronous_standby_names` 要求的同步备库数不够（主库提交会卡住：FAIL？）；延迟怎么判（有没有客观线）
   - `cluster`：repmgr 说的角色和数据库自己的 `sys_is_in_recovery()` 不一致；节点不在 running
   - `space`：没有客观线（剩多少算少因库而异），倾向只展示
   - `top-objects`：只展示
   - `table <t>`：复用 vacuum、freeze 的判定；表不存在给用法错误还是 UNKNOWN，要论证
   - `top`：`sys_stat_statements.track=none`（实验环境就是这样）或扩展没装时不能报 OK 加空表，要明说没在收集（`skipped` 加原因，还是 UNKNOWN，要论证）
5. **DS 场景**：新场景补进 `docs/PRD.md` 的 DS 场景表（接着现有编号），并说明 L6 验收怎么在 VM 上造出来；造不出来的写明。
6. **需要用户拍板的点**：列在每节末尾。

**两件用户还没定的事，本计划不替用户定**：
- 暂停的 walreceiver 要不要用 `wal_receiver_timeout` 判：`repl` 的场景表里一定会碰到。写出选项和你的建议，列进"需要用户拍板"，**实现时只展示 `last_msg` 年龄，不加这条判定**，等用户定。
- `inst.upstream` WARN 的注入：已定为已知限制，不写注入脚本。

## 2. 阶段

| 阶段 | 在哪 | 内容 | 检查点（用户异步审核的东西） |
|---|---|---|---|
| 0 | 本地 | 在 node1/node2 上用 ksql 实采 7 条命令要用的视图原始行，存进 `e2e/testdata/captures/v02/`（见 §3），推 main | 采集清单齐全 |
| 1 | 云 | **10 条命令的场景表**（§1），写进附录 B，补 PRD DS 场景表 | 附录 B：场景、边界、判定、需要拍板的点 |
| 2 | 云 | **space** | 草样、判定 |
| 3 | 云 | **freeze** | 同上 |
| 4 | 云 | **vacuum** | 同上 |
| 5 | 云 | **archive** | 同上 |
| 6 | 云 | **params** | 同上 |
| 7 | 云 | **repl** | 同上；walreceiver 暂停那条只列选项 |
| 8 | 云 | **cluster** | 同上；repmgr 元数据的读法和权限 |
| 9 | 云 | **top-objects** | 同上 |
| 10 | 云 | **table** | 同上；表名解析（大小写、schema、不存在） |
| 11 | 云 | **top** | 同上；统计没开、kbdiag_ro 看不到 query |
| 12 | 云 | **跨命令收口和加固**：所有 `verify:` 指向的命令和参数都存在（扩展已有的 Next.Command 扫描测试）；各命令 next 互相一致；README（中英）和 PRD 命令表一致；新 report 格式化的边界测试；汇总 10 条命令的 VM 待验清单成一份核对表写进 chronicle；起草 `v2.0.0-alpha.3` 发布说明写进 chronicle，不打 tag | 核对表、发布说明草稿 |
| 13 | 本地 | fetch 云分支 → 两节点 e2e（`-count=1`）→ 修 → 把 VM 实跑文本贴给用户 → Codex 审查 → Jev 分诊 → 修 → queries.md 验证状态 → kbdiag-docs 10 个新命令页（VM 实跑输出，注明 commit）→ 用户说了才合 main、打 tag、合 kbdiag-docs main | 每条命令的 VM 输出；Codex 意见处理表 |

### 阶段 12 之后：后备队列（按顺序取，做完一条再取下一条）

用户 2026-09-27：云额度还剩 $60，"值得用完"，不要让云会话停下来。阶段 12 做完后按下表往下做，每条走同一套固定流程（场景表先补进附录 B，再实现）。这四条原来在 queries.md 里是"待排"，用户同意提前做：它们都是单次查询，而且 §3 已经有实采。

| 阶段 | 命令 | queries.md | 一句话 | 实采 |
|---|---|---|---|---|
| 14 | `progress` | D5 | 正在跑的 VACUUM、CREATE INDEX、CLUSTER，以及 KES 自己的 checkpoint 进度 | `progress_*` |
| 15 | `checkpoint` | H3 | checkpoint 是定时触发还是被 WAL 量逼出来的，以及 bgwriter 和后端进程写了多少 | `checkpoint_*` |
| 16 | `wal` | H1 | 当前 LSN、WAL 目录多大、谁让 WAL 留着（槽、`wal_keep_segments`） | `wal_*` |
| 17 | `seq` | F7 | 快用完的序列，int 和 smallint 的单独标出 | `seq_*` |
| 18 | 加固 | — | 见下面的列表 | — |

阶段 18 的加固项，全都做完了才算完：

- 给 10 多条新命令的 report 加 fuzz 测试（Go 原生的 `testing.F`），覆盖控制字符、超长字符串、NULL 和极值，查崩溃和转义漏掉的情况；
- 对全部命令的 `--json` 做 schema 一致性测试：字段名和 PRD §5 的契约逐个对上；
- `go test -race`；
- 跑 `deslop` 式的自查，删掉多余的注释和重复的 helper；
- 把 CLAUDE.md 里各命令小节的"为什么"整理成一样的结构；
- 最后把阶段 13 的 VM 待验清单再过一遍，按命令排好，让本地可以照着逐条跑。

阶段 14–17 的 VM 验证同样并进阶段 13（本地）。阶段 13 在全部云阶段之后做。

阶段 2–11 按顺序做（后面的命令会引用前面命令的 next），不停下来等审核；用户对前一阶段提了意见，先处理意见再继续。

**一口气做下去**（用户 2026-09-27："我要的继续是开发，任务量可以更多"）：阶段 1 提交推送后直接进阶段 2，一直做到阶段 12，然后接着做后备队列（阶段 14–18）。只有这几种情况停：碰到 §4 的门槛；分类器拦了；证据不够、只能靠猜 KES 行为（先跳过这一小块，列进 VM 待验清单，接着做别的）。额度快用完时，先把当前阶段做到能提交的状态，写 chronicle（做到哪、下一步是什么），再推送。

### 阶段 2–11 每条命令的固定流程

1. 取附录 B 的场景表；用户已经在上面留了意见就先按意见改。
2. **SQL**：按 queries.md"SQL 引入规则"过第 1、2、4 关（改写成 KES 语义、核对 KES 文档、probe 注释写出处）；第 3 关（VM 实跑）留给阶段 13，列进 VM 待验清单。一个 probe 一条 SQL；列名以实采为准。权限：kbdiag_ro 看不到的列按 `redacted[]` 的规矩处理，实采里有 kbdiag_ro 的结果。
3. **草样**：用 `captures/v02/` 的实采数据手排文本输出，至少覆盖：主库、备库、kbdiag_ro；有 finding 的命令再手造一个触发的例子（从实采改数值，注明是改的）。排版沿用 status/slots（时长两个最大单位，大小按 1024 进位用 `internal/units`，表格用 `internal/report/table.go`，JSON 保留原始值）。
4. **契约**：命令名、参数、列、JSON 字段、probe_id、finding.id、next 写进 PRD §4/§5 和 queries.md 追溯表（验证状态写"未验证"）。
5. **先写失败测试**：golden 手抄草样，确认先失败；facts 尽量从 `captures/v02/` 还原。rule 单测按"暴力测试"补边界：空结果、NULL、0 和负数、极大值（xid 年龄接近 2^31、字节数上 TB）、xid 回绕（按 2^32 取模）、多字节表名。
6. **实现**：`internal/probe/<域>.go`、`internal/rule`（纯函数）、`internal/scenario`、`internal/report/<命令>.go`、`cmd/kbdiag` 注册命令。
7. **e2e**：`e2e/` 里加这条命令的测试，只编译不跑；每条断言在 VM 待验清单里点名。需要故障注入的场景，**可以写新注入脚本但标明未在 VM 上跑过**，阶段 13 本地验证；动 sudo、重启实例、改集群网络的注入不写。
8. **文档**：README（中英两段）、PRD、queries.md；CLAUDE.md 加一节"<命令>（2026-09-xx）"写**为什么**。不新建文档；跑防僵尸检查。
9. **审查**：起一个 code-reviewer 审这一阶段的 diff，处理意见。
10. **检查点提交**：提交信息 `v02(<命令>): ...`；chronicle 当日文件追加"<命令>（云会话）"一节：改动、实际跑过的命令和结果、VM 待验清单、需要用户拍板的点、下一步。推分支，直接进下一阶段。

## 3. 阶段 0：本地实采清单

存到 `e2e/testdata/captures/v02/`，文件名 `<命令>_<节点>_<用户>[_<场景>].txt`（用户是 `sys` 或 `ro`），第一行是 SQL，末尾记退出码。每个视图用 `select *` 采全列，外加计划里要用的函数调用；node1、node2 都采，system 和 kbdiag_ro 各一遍。

| 命令 | 采什么 | 注入/场景 |
|---|---|---|
| space | 各库 `pg_database_size`；`sys_tablespace` 加 `pg_tablespace_location`/`pg_tablespace_size`；`sys_ls_waldir()` 的个数和总大小；`data_directory`；主机上 `df -k` | 干净 |
| freeze | `sys_database` 的 `age(datfrozenxid)`、`mxid_age(datminmxid)`；`sys_class` 按 `age(relfrozenxid)` 排的前几十行；freeze 相关参数 | 干净（实验库的年龄只有几千，触发的例子只能手造） |
| vacuum | `sys_stat_user_tables` 全列；`sys_stat_progress_vacuum`；`sys_stat_activity` 里的 autovacuum worker；autovacuum 相关参数；表级 reloptions | 干净 + 一张关了 autovacuum、删过一半行的测试表（采完删表） |
| archive | `sys_stat_archiver`；`archive_mode`/`archive_command`/`archive_timeout`；`sys_wal/archive_status` 下 `.ready` 的个数 | **实验环境归档本来就在失败**（2026-09-27 实测 failed_count 17952，最后成功 2026-09-16），照实采 |
| params | `sys_settings` 全列（全部行 + `source <> 'default'` 的行）；`pending_restart` 为 true 的行 | 干净 |
| repl | 主库 `sys_stat_replication` 全列；备库 `sys_stat_wal_receiver` 全列、`sys_last_wal_receive_lsn()`、`sys_last_wal_replay_lsn()`、`sys_last_xact_replay_timestamp()`、`sys_is_wal_replay_paused()`；`synchronous_standby_names`、`synchronous_commit` | 干净 + slot 注入（暂停 node2 的 walreceiver）期间主备各采一次 |
| cluster | `repmgr cluster show`（文本和 `--csv`）；`esrep` 库 `repmgr` schema 下的表和视图（`nodes`、`events` 最近几十行、`monitoring_history` 等）全列；kbdiag_ro 对它们的权限 | 干净 |

采完删掉测试表、撤掉注入，在 chronicle 里记采集时间、KES 版本和发现。

**这些文件的寿命**：计划 done 时，被 golden 或 e2e 引用的留下，其余删掉。

### 阶段 0 结果（2026-09-27 21:20–21:30，UTC+08，V008R006C009B0014）

`e2e/testdata/captures/v02/` 共 213 个文件（含补采的 top-objects、table、top）。命名 `<命令>_<节点>_<sys|ro>_<名字>[_<场景>].txt`，sys 是 `system`，ro 是 `kbdiag_ro`（不带监控角色）。首行 `-- 用户@库 on 节点: SQL`，末行 `EXIT_CODE=n`；输出格式 `ksql -X -A -F'|' -P null='<NULL>'`。`shell_node{1,2}.txt` 是主机侧的数据：data_directory、`df -k`、`du sys_wal`、archive_status 计数、`repmgr cluster show`（文本和 `--csv`）、`repmgr node status`。场景后缀：`_dead` 是死元组测试表，`_paused` 是 slot 注入。

采集时发现的事实（云会话写场景表时要用；**都是这一次实采，不是文档结论**）：

- **freeze**：`txid_current_snapshot()` 报 `xid snapshot is not supported`（`freeze_node1_sys_xid_unsupported.txt`）。`sys_control_checkpoint()` 可用，next_xid 形如 `0:6442`，但它是上一次 checkpoint 时的值。`age(xid)` 可用，按当前 next xid 算。
- **ro 账号被拒**：`sys_ls_waldir()`、`sys_ls_archive_statusdir()`；表空间 `sys_global`（所以 `pg_tablespace_size` 失败）；`pg_show_all_file_settings`（`sys_file_settings` 也失败）；`esrep` 库的 `repmgr` schema。这些对 ro 只能是 `skipped` 或 `redacted`，不能当成空。
- **vacuum**：`sys_stat_user_tables` 是节点本地的统计。主库上 `kbdiag_inj_dead` 有 5000 死元组，备库上同一张表全是 0。所以备库上判不了死元组，应当报 `not_applicable`，或者明说只是本节点的统计。
- **archive**：归档确实在失败。`archive_mode=always`，`archive_command` 是 `sys_rman archive-push`。`sys_stat_archiver` 是 archived 35、failed 17952，最后一次成功在 2026-09-16。`repmgr node status` 报 1 个 pending。node2 的 archive_status 下有 175 个 `.done`。
- **params**：`sys_settings` 有 509 行，其中有 `pending_restart` 列。`params_*_all` 里 grep 到的 "ERROR" 是描述文字（"error messages"），不是报错。
- **repl**：参数是 `synchronous_standby_names='ANY 1( node2)'`、`synchronous_commit=remote_apply`、`wal_keep_segments=512`、`max_wal_size=1024`；repmgr.conf 里是 `synchronous='quorum'`。**意外发现**：slot 注入暂停 node2 的 walreceiver、过了 `wal_sender_timeout` 之后，主库 `sys_stat_replication` 是 0 行，在主库上 `select txid_current()` 的提交**没有卡住**（立即返回，也没有 SyncRep 等待，见 `repl_*_paused`）。`sys_settings` 里没有名字带 degrad/async 的参数。怀疑 KES 或 repmgr 有同步自动降级为异步的机制，**没有验证**。repl 的场景表里"同步备库断开时提交会不会卡住"只能标成未验证，由阶段 13 查 KES 文档和 VM。
- **cluster**：暂停期间 `repmgr cluster show` 退出码是 25，警告 `node "node2" not found in sys_stat_replication` 和 `not attached to its upstream`，Upstream 列显示 `! node1`，LSN_Lag 在 node1 上是 320 bytes、node2 上是 272 bytes。干净时退出码 0。repmgr 二进制在 `/home/kingbase/cluster/install/kingbase/bin/repmgr`，元数据在 `esrep` 库。
- **space**：数据盘 `/dev/vda4` 208 GB，用了 6%；`sys_wal` 约 2.6 GB（du 2605092 kB）。
- **top-objects**（`topobj_*_rels`、`topobj_*_idx`）：`reltuples` 是 float4，ksql 显示成 `1e+06`，SQL 里要 cast 成 bigint。kbdiag_ro 也能看到大小，结果和 system 一样（`pg_total_relation_size` 没有被拒）。
- **table**（`table_*`，21:40 左右补采）：测试表 `public.kbdiag_inj_tbl`，2 万行删掉四分之一、带主键、一个普通索引和 TOAST，采完已删。采了 `sys_class` 全列加 `age(relfrozenxid)`、`sys_stat_user_tables`、`sys_statio_user_tables`、`sys_index` 加 `pg_get_indexdef`、`sys_stat_user_indexes`、各项大小。备库的 `sys_stat_user_tables` 同样全是 0。表不存在时 `'x'::regclass` 报 `relation "public.no_such_tbl" does not exist`（`table_*_missing`）；`to_regclass` 按标识符规则折叠大小写：`kbdiag_inj_tbl` 和 `KBDIAG_INJ_TBL` 都能解析到，`public."KBDIAG_INJ_TBL"` 是 NULL（`table_*_ambiguous`）。
- **top**（`top_*`）：`sys_stat_statements` 1.11 装在 test 库里，也在 `shared_preload_libraries` 中；但 **`sys_stat_statements.track=none`**（配置文件里设的），实验库里一行都没有。`sys_stat_statements_info` 不存在，所以拿不到统计起点。列名用 `total_exec_time`、`mean_exec_time`（PG13 以后的命名），另有 `parses`、`total_parse_time` 等 KES 自己加的列。有数据的样本 `top_node1_*_stmts_tracked` 是在一个会话里 `SET sys_stat_statements.track='top'` 后跑几条 SQL 采的，采完已 `sys_stat_statements_reset()`、删测试表。kbdiag_ro 看别人的语句：`queryid` 为 NULL，`query` 是 `<insufficient privilege>`，数值列都能看到。
- **progress**（`progress_*`，22:45 左右补采）：KES 只有 4 个进度视图，`sys_stat_progress_vacuum`、`_create_index`、`_cluster`，加上 KES 特有的 `_checkpoint`（列：pid、phase、flags、buffers_scan、buffers_processed、buffers_written、written_progress、write_rate、start_time）。**`_analyze` 和 `_basebackup` 不存在**。`progress_node1_sys_index_running.txt` 是在 `public.orders` 上 CREATE INDEX 的过程中采的，有一行，phase 是 `building index: scanning table`；那个索引采完已删。kbdiag_ro 也能查这几个视图。
- **checkpoint**（`checkpoint_*`）：`sys_stat_bgwriter` 是 PG13 以前的列（checkpoints_timed、checkpoints_req、buffers_backend_fsync 等），stats_reset 在 node1 是 2026-09-15、node2 是 2026-09-23；两节点的 checkpoints_req 都是 0。另外采了参数和 `sys_control_checkpoint()`。
- **wal**（`wal_*`）：node1 的 `sys_ls_waldir()` 有 179 个文件、2.8 GB，kbdiag_ro 被拒。**`sys_replication_slots` 没有 `wal_status`、`safe_wal_size`**，只有 PG12 的列。`sys_walfile_name()` 在备库上报 `recovery is progressing`。参数里没有 `wal_keep_size` 和 `max_slot_wal_keep_size`（以 `wal_settings` 为准）。
- **seq**（`seq_*`）：`sys_sequences` 有 last_value（从来没调用过的是 NULL）和 data_type；实验库里有 bigint 和 integer 两种。
- 采完已撤注入、删测试表；两节点 `kbdiag_inj%` 会话 0、2PC 0，`repmgr_slot_2` active，node2 walreceiver streaming。

## 4. 审批门槛（压缩、交接、云会话都不能自己跨过）

- 合进 main、推 main、打 tag、推 kbdiag-docs：只有用户明确说了才做（阶段 0 的实采推 main 是本计划批准的）。
- 附录 B 和各阶段"需要用户拍板"的点：云会话先按建议实现，用户否了就改；不能因为已经实现了就当成用户同意。
- 暂停的 walreceiver 的判定：用户没定，只展示不判。
- 任何写操作（`kill`、`CHECKPOINT`、`pg_switch_wal`、重载参数）都不进诊断路径：kbdiag 默认只读。

## 5. 进度

- 2026-09-27：用户定范围（巡检五条 + 复制两条）和做法；计划写成，状态 active
- 2026-09-27：阶段 0 完成（实采 213 个文件，发现见 §3"阶段 0 结果"）。下一步：云会话从阶段 1 开始
- 2026-09-27：加后备队列阶段 14–18（progress、checkpoint、wal、seq、加固），补采；云会话做完阶段 12 接着做
- 2026-09-27：用户追加范围（top-objects、table、top，阶段 9–11），收口改为阶段 12、本地 VM 收尾改为阶段 13；补采这三条；明确云会话一口气做到阶段 12

## 附录 B：场景表（阶段 1 写）
