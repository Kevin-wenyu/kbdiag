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
- 2026-09-27：阶段 1 完成（云会话）：附录 B 写完 10 条命令的场景表，PRD §11 补 DS-25~30。下一步：阶段 2 space
- 2026-09-27：阶段 2 space 完成（云会话，提交见 git log `v02(space)`）。下一步：阶段 3 freeze
- 2026-09-27：阶段 3 freeze 完成（云会话，`v02(freeze)`）。下一步：阶段 4 vacuum
- 2026-09-27：用户追加范围（top-objects、table、top，阶段 9–11），收口改为阶段 12、本地 VM 收尾改为阶段 13；补采这三条；明确云会话一口气做到阶段 12

## 附录 B：场景表（阶段 1 写）

阶段 1 写于 2026-09-27（云会话）。证据只来自 CLAUDE.md、`e2e/testdata/captures/v02/` 和计划 §3；按 PG 行为推出来、实采里没有的，标"未验证"并列进该节的 VM 待验点。判定的级别按"FAIL = 业务已经受影响；WARN = 还没受影响，但不处理会出事"，只用客观线。

**所有 10 条命令共用的约定**（后面各节不再重复）：

- verdict：有判定的命令，只有判定依赖的 probe 没采到（`skipped`/`error`）才让 verdict 变成 UNKNOWN；只展示的 probe（像 `inst.disk`）没采到不影响 verdict。**纯展示的命令**（space、top-objects、top 这类没有判定的）任何一个 probe 没采到就是 UNKNOWN（同 waits：看不全就不说 OK）。`not_applicable` 不参与。
- 文本排版照 status/slots：单行用键值，大小用 `internal/units`（1024 进位），时长留两个最大单位，表格用 `writeTable`；JSON 保留字节和秒。
- `--limit` 只裁文本和 JSON 的行，判定覆盖全部行；新命令的列表默认 20 行（sessions/locks/txn 的 50 不变）。
- 列表命令都是当前库（`-d`）的视角时，文本标题写明库名，next 用 `kbdiag -d <库> ...` 指向别的库。

### B.1 space（F1）

| # | DBA 想问 | 现在能不能答 | space 怎么答 |
|---|---|---|---|
| SP1 | 磁盘还剩多少，是哪块盘在满 | status 的 `inst.disk` 只看数据目录所在的一块盘，一行 | `space.disk`：数据目录、`sys_wal`、每个非默认表空间目录各自所在的文件系统（同一文件系统合并显示），本机运行才有 |
| SP2 | 哪个库最大 | status 的 `inst.databases` 能答 | 复用 `inst.databases`（同一 probe_id，同样的列），按大小排，给总数 |
| SP3 | 表空间各多大、在哪 | 不能 | `space.tablespaces`：名字、位置、大小；看不到大小的记 `redacted[]` |
| SP4 | WAL 目录多大，是不是超出了配置该有的量 | 不能 | `space.wal`：`sys_wal` 的文件数和总大小，旁边给 `max_wal_size`、`wal_keep_segments × wal_segment_size`（实验环境 512 × 16 MB = 8 GB）作参照，并提示谁会保留 WAL：`kbdiag slots`、`kbdiag archive` |

不归 space：表和索引这一级 → `top-objects`；单表 → `table <t>`；谁保留了 WAL → `slots`（槽）、`archive`（归档失败）；数据目录那块盘的一行摘要 → `status`（它是第一条命令，所以 `inst.disk` 留着）。

参数：无。

判定：**只展示，不判**。剩多少算少因库而异，没有客观线。唯一可能的客观线是"可用空间为 0（或不够一个 WAL 段）"，列为拍板点。

probe：
- `space.disk`（statfs，本机运行才有；远程 `not_applicable`）：列 `path_kind`（data_directory / wal / tablespace）、`path`、`total_bytes`、`used_bytes`、`avail_bytes`。这是 `inst.disk` 之外第二个不是 SQL 的 probe，理由同 `inst.disk`：KES 没有查剩余空间的函数，而 space 的问题就是"盘还剩多少"。路径来自 `data_directory` 和 `sys_tablespace` 的 location，都要先经过 SQL 拿到，所以它依赖 `inst.info` 和 `space.tablespaces`。
- `space.tablespaces`：`spcname`、`location`、`size_bytes`。kbdiag_ro 调 `pg_tablespace_size(sys_global)` 被拒（`space_*_ro_tablespace`），整条 SQL 失败，所以大小要按权限用 CASE 包起来，看不到的给 NULL 记 `redacted[]`（权限判断的写法未在 VM 验证）。
- `space.wal`：`files`、`bytes`、`max_wal_size_bytes`、`wal_keep_bytes`。`sys_ls_waldir()` 对 kbdiag_ro 被拒（`space_*_ro_waldir`），整条 probe `skipped`，所以 kbdiag_ro 下 space 是 UNKNOWN。

DS：16（WAL 增长）；新增 DS-25（磁盘或表空间快满）。

需要用户拍板：
1. 纯展示不判（建议）；还是加一条 FAIL"数据目录/WAL 所在文件系统可用空间 < 一个 WAL 段"（写不出新 WAL 段，业务已经受影响）。
2. `space.disk` 作为第二个 statfs 例外（建议加，否则 space 回答不了"是哪块盘在满"）。

VM 待验点：表空间大小的权限 CASE；`sys_wal` 是符号链接时 statfs 跟到链接目标；kbdiag_ro 下 `space.wal` 为 `skipped`。

### B.2 freeze（G2）

| # | DBA 想问 | 现在能不能答 | freeze 怎么答 |
|---|---|---|---|
| FZ1 | 离事务号回卷还有多远 | 不能（txn 只给最老的活跃 xid，不是冻结年龄） | 每个库的 `age(datfrozenxid)`、`mxid_age(datminmxid)`，对照 `autovacuum_freeze_max_age` 和回卷硬线，写"还剩多少" |
| FZ2 | 是哪些表最老 | 不能 | 当前库里按 `age(relfrozenxid)` 排的表（含 TOAST、物化视图），给大小，方便估 VACUUM FREEZE 的量 |
| FZ3 | 为什么冻结推不动 | txn（谁压着视界）、slots（槽的 xmin） | 不自己查，finding 的 next 指向 `kbdiag txn`、`kbdiag slots`、`kbdiag vacuum`（autovacuum 在不在跑） |
| FZ4 | 别的库里哪些表老 | 不能 | next：`kbdiag -d <库> freeze` |

不归 freeze：谁压着视界 → `txn`、`slots`；死元组和 autovacuum 状态 → `vacuum`；单表 → `table <t>`。

参数：`--limit`（表列表，默认 20）。

判定（按库，年龄取 xid 和 multixact 两种）：
- `freeze.database_age` **WARN**：`age(datfrozenxid) ≥ autovacuum_freeze_max_age`（或 `mxid_age ≥ autovacuum_multixact_freeze_max_age`）。客观线是服务器参数：到了这条线 autovacuum 就该强制冻结，平时年龄会被它压在线下；在线上说明防回卷的 vacuum 正在跑或者推不动。业务还没受影响，所以 WARN。
- 同一 id **FAIL**：`age(datfrozenxid) ≥ 2^31 − 1 − 1,000,000`（阶段 3 审查纠正：起草时写的 300 万是 PG14 的值）。这是 PG12 内核的 xidStopLimit：到这里服务器拒绝分配新 xid，写事务全部失败，业务已经受影响。**这个常数来自 PG 源码，KES V8R6（PG12 内核）是否相同未验证**，列进 VM 待验点，实验环境造不出这个年龄（实采年龄只有 5364）。
- next：verify `kbdiag txn`（最老 xid 是谁压的）、`kbdiag slots`；fix `VACUUM (FREEZE, VERBOSE)` 在该库里对最老的表跑（`kbdiag -d <库> freeze` 列出它们）。
- 表级不另出 finding（表的年龄决定库的年龄，库级已经报了）。

probe：
- `freeze.databases`：`datname`、`datfrozenxid`、`xid_age`、`datminmxid`、`mxid_age`、`datallowconn`（kbdiag_ro 能看全，实采一致）。
- `freeze.tables`：`relation`、`relkind`、`relfrozenxid`、`xid_age`、`relminmxid`、`mxid_age`、`total_bytes`，只含 `relfrozenxid <> 0` 的行。**实采发现** `_kingbase_loginfo` 和它的 TOAST 表 `relfrozenxid = 0`，`age(0)` 返回 2147483647（`freeze_*_rel`），不排除就会误报 FAIL。
- `freeze.limits`（一行）：`autovacuum_freeze_max_age`、`autovacuum_multixact_freeze_max_age`、`vacuum_freeze_table_age`。
- 备库：年龄和主库相同（系统表是复制过来的，`age()` 在备库可用，实采 5364），照常报；next 写明 VACUUM 要到主库上跑。

DS：19（冻结年龄风险；阈值来源统一，修 GAP-6：只用服务器参数，没有 kbdiag 自己的阈值）。L6：实验环境造不出高年龄，只能 L1/L2 手造 facts；VM 上只验证阴性（年龄几千、OK）和 `relfrozenxid = 0` 的表不出现。

需要用户拍板：
1. WARN 线用 `autovacuum_freeze_max_age`（建议）还是更晚的 xidWarnLimit（PG12 是停止线前 1000 万，服务器开始在日志里告警的线）。
2. FAIL 用 PG12 的 xidStopLimit 常数（建议，VM 阶段查 KES 文档核实）。
3. multixact 也报 FAIL（阶段 3 改：PG12 的 multiStopLimit 是回卷线前 100，写入同样会失败；原建议只报 WARN）。

### B.3 vacuum（G1）

| # | DBA 想问 | 现在能不能答 | vacuum 怎么答 |
|---|---|---|---|
| VA1 | 哪些表死元组多，autovacuum 该不该管它了 | 不能 | 当前库按 `n_dead_tup` 排的表：死/活元组、触发线（`autovacuum_vacuum_threshold + scale_factor × reltuples`，表级 reloptions 覆盖）、是否已过线、最近一次 vacuum/autovacuum 多久前 |
| VA2 | autovacuum 开着吗 | 不能 | `autovacuum`、`track_counts` 两个开关 |
| VA3 | 现在有没有 vacuum 在跑，跑到哪了 | waits 只看等待事件 | 正在跑的 vacuum（autovacuum worker 和手工 VACUUM）：库、表、阶段、已扫/总块数、跑了多久 |
| VA4 | 清不动是因为什么 | txn、slots | 不自己查，next 指向 `kbdiag txn`、`kbdiag slots` |

不归 vacuum：膨胀估算 → `bloat`（待排）；冻结 → `freeze`；统计信息是否过时 → `stats-age`（待排）；单表 → `table <t>`。

参数：`--limit`（表列表，默认 20）。

判定：
- `vacuum.disabled` **WARN**：`autovacuum=off` 或 `track_counts=off`。客观线是开关本身：关了就没有任何表会被自动清理和冻结（防回卷的 vacuum 除外）。
- `vacuum.table_disabled` **WARN**：表级 `autovacuum_enabled=false` 且死元组已过触发线：没人会清它。实采 `kbdiag_inj_dead`（5000 死元组，触发线 50 + 0.2 × 10000 = 2050）正是这样。
- 过了线但 autovacuum 开着的表**不报**，文本标 `due`：那是 autovacuum 的正常队列，每个 naptime 才轮一次，过线本身不是故障。"过线很久没清"要一条时间线，没有客观的，列为拍板点。
- next：fix `VACUUM <表>`；verify `kbdiag txn`（清完还剩很多死元组时，是谁压着视界）。

probe：
- `vacuum.tables`：`schemaname`、`relname`、`n_live_tup`、`n_dead_tup`、`reltuples`、`reloptions`（原样，文本数组）、`last_vacuum_age_s`、`last_autovacuum_age_s`、`vacuum_count`、`autovacuum_count`。触发线和表级开关由 rule 从 `reloptions` 解析（纯函数，不依赖 `pg_options_to_table` 在 KES 上是否可用）。
- `vacuum.progress`：`pid`、`datname`、`relation`、`phase`、`heap_blks_total`、`heap_blks_scanned`、`is_autovacuum`、`xact_age_s`。kbdiag_ro 看不到别人会话的 backend_type（实采 workers 查询 0 行），`sys_stat_progress_vacuum` 对它也是 0 行，这时要记 `redacted[]` 而不是说"没有 vacuum 在跑"——怎么区分"真的没有"和"看不到"，未验证，列进 VM 待验点。
- `vacuum.settings`（一行）：`autovacuum`、`track_counts`、`autovacuum_vacuum_threshold`、`autovacuum_vacuum_scale_factor`、`autovacuum_naptime_s`、`autovacuum_max_workers`。
- **备库**：`sys_stat_user_tables` 是节点本地统计，备库上全是 0（实采 `vacuum_node2_*_tables_dead`），autovacuum 也不在备库跑，所以 `vacuum.tables`、`vacuum.progress` 在备库上是 `not_applicable`，reason 写"run on the primary"。

DS：17（死元组/膨胀）、18（autovacuum 长时间未执行，关联长事务：next 指向 txn，修 GAP-1 的一半）。L6：建表、关表级 autovacuum、删一半行（同阶段 0 的做法）→ `vacuum.table_disabled` WARN；可以写 `e2e/inject/dead_tuples.sh`，阶段 13 验证。

需要用户拍板：
1. 过线的表在 autovacuum 开着时只标 `due` 不报（建议）；还是加一条"过线且最近一次 autovacuum 早于 N 个 naptime"的 WARN（N 要定，没有客观来源）。
2. `vacuum.disabled` 和 `vacuum.table_disabled` 分两个 id（建议）还是合一个。

### B.4 archive（H2）

| # | DBA 想问 | 现在能不能答 | archive 怎么答 |
|---|---|---|---|
| AR1 | 归档开了吗，归档命令是什么 | 不能 | `archive_mode`、`archive_command`、`archive_timeout` |
| AR2 | 归档是不是在失败 | 不能 | `sys_stat_archiver`：成功/失败次数，最后一次成功和失败的 WAL 和时间，距今多久 |
| AR3 | 积压了多少没归档的 WAL | 不能 | `archive_status` 下 `.ready` 的个数（kbdiag_ro 调不了，`skipped`） |
| AR4 | 积压撑大了 WAL 目录没有 | 不能 | next 指向 `kbdiag space` |

不归 archive：WAL 目录多大 → `space`；槽保留的 WAL → `slots`；备份（sys_rman）是否完整 → 不做。

参数：无。

判定：
- `archive.failing` **WARN**：`archive_mode` 不是 off，且最后一次失败晚于最后一次成功（或从没成功过而失败数 > 0）。客观线是"最近一次尝试失败了"。WAL 在主库上堆着、备份缺段，但业务还没受影响，所以 WARN。实验环境正是这样（failed 17961，最后成功 2026-09-16，`archive_node1_*_stat`），VM 上天然能测。
- `archive_mode=off` 不报：主动配置，文本写"archiving is off"。备库上 `archive_mode=on`（不是 always）时归档进程不跑，文本写明，同样不报（未验证：实验环境是 always）。
- `archive_command` 为空而 mode 开着：PG 文档说此时 WAL 会一直留着等命令，是 WARN 的候选，但 KES 行为没有实采，只在文本里标出来，列为拍板点。
- next：verify 看服务器日志里 archive_command 的报错（`log_directory`，实验环境是 `sys_log`）；verify `kbdiag space`（WAL 目录被撑到多大）。

probe：
- `archive.status`（一行）：`archive_mode`、`archive_command`、`archive_timeout_s`、`archived_count`、`last_archived_wal`、`last_archived_time`、`failed_count`、`last_failed_wal`、`last_failed_time`、`stats_reset`。设置走 `sys_settings` 子查询（kbdiag_ro 看得到，实采一致）。
- `archive.ready`（一行）：`ready`、`done`。kbdiag_ro 被拒（`archive_*_ro_statusdir`）→ `skipped`；它只展示、不是判定输入，不影响 verdict。

DS：16（WAL 增长：归档失败）；新增 DS-26（归档失败）。

需要用户拍板：
1. `archive.failing` 用 WARN（建议）。
2. `archive_command` 为空而 mode 开着要不要报（建议：先只在文本标出，VM 阶段核实 KES 行为后再定）。

### B.5 params（A2）

| # | DBA 想问 | 现在能不能答 | params 怎么答 |
|---|---|---|---|
| PA1 | 哪些参数不是默认值，在哪设的 | 不能 | 非默认参数：当前值、单位、来源（配置文件/ALTER SYSTEM/库级/用户级）、文件和行号、默认值 |
| PA2 | 哪些改了还没生效（要重启） | 不能 | `pending_restart = true` 的参数 |
| PA3 | 某个参数现在是多少 | 不能 | 不加参数：`ksql -c 'show x'` 能答，kbdiag 的 JSON 配 jq 也能答；列为拍板点 |

不归 params：参数调优建议（不收录）；可观测性开关是否就绪 → `ready`（待排）；复制相关参数的含义 → `repl`。

参数：无（不加 `[pattern]`，见拍板点）。

判定：
- `params.pending_restart` **WARN**：有参数改了（reload 过）但要重启才生效。客观线是服务器自己的 `pending_restart` 列。现在跑的还是旧值，业务没受影响；但下次重启（包括故障切换后的重启）会突然换成新值，所以 WARN。实验环境没有这样的参数（`params_*_pending` 0 行），L6 要注入：`ALTER SYSTEM SET` 一个 postmaster 级参数再 reload，撤注入用 `ALTER SYSTEM RESET` 再 reload（不重启、不 sudo），脚本 `e2e/inject/pending_restart.sh`，阶段 13 验证。
- next：verify `kbdiag params --json`（看新旧值）；fix：计划一次重启，或 `ALTER SYSTEM RESET <name>` 撤回。

probe：
- `params.changed`：`name`、`setting`、`unit`、`source`、`sourcefile`、`sourceline`、`boot_val`、`reset_val`、`context`、`pending_restart`。过滤 `source not in ('default', 'override', 'client', 'session')`：override 是编译或 initdb 定的（block_size、data_checksums），client/session 是 kbdiag 自己这个连接设的（实采里 ksql 的 `application_name`），都不是"谁改了参数"。
- **kbdiag_ro**：`sourcefile`、`sourceline` 是 NULL，而且看不到超级用户专属的参数（实采全表 482 行对 509 行，非默认 78 行对 85 行，缺的是 config_file、hba_file、primary_conninfo 这类）。文件位置记 `redacted[]`；看不到的参数没法计数，但它们的 `pending_restart` 也看不到，所以 kbdiag_ro 下没有 finding 时 verdict 是 UNKNOWN，文本写明"only parameters this account may read"。

DS：新增 DS-27（参数改了没生效/待重启）。

需要用户拍板：
1. 不加 `params <pattern>`（建议：plan §1 说不加过滤参数；看单个参数用 `ksql -c 'show x'`）；还是加一个可选位置参数，列出匹配名字的全部参数（含默认值）。
2. `sys_file_settings.error` 非空（配置文件有错，下次重启可能起不来）要不要报：kbdiag_ro 调不了（`params_*_ro_file`），建议先不做，列进以后。

### B.6 repl（I1 + I4）

| # | DBA 想问 | 现在能不能答 | repl 怎么答 |
|---|---|---|---|
| RP1 | （主库）各备库连着吗，落后多少 | status 的 `inst.downstreams` 只给状态 | 每个 walsender：应用名、地址、状态、同步状态，sent/write/flush/replay 各落后多少字节，write/flush/replay_lag 秒数，上次回复多久前 |
| RP2 | （主库）同步复制够不够数 | 不能 | `synchronous_standby_names` 要求的数量和实际在 streaming 的候选数 |
| RP3 | （备库）在不在收 WAL | status 的 `inst.upstream` 能答 | 复用 `inst.upstream`（同一 probe、同一 finding） |
| RP4 | （备库）回放落后多少，回放是不是被暂停了 | 不能 | 收到的 LSN 和回放的 LSN 差多少字节，最近回放的事务是多久以前的，`sys_is_wal_replay_paused()` |
| RP5 | （备库）walreceiver 是不是卡住了 | status 展示 `last_msg` 年龄 | 同样展示 `last_msg`；判不判等用户定（§1，只展示） |

不归 repl：槽保留的 WAL → `slots`；repmgr 眼里的集群 → `cluster`；"在不在复制"的一句话 → `status`。

参数：无。

判定：
- `inst.upstream`（复用 status 的规则，WARN）：备库没有接收进程或状态不是 streaming。
- `repl.replay_paused` **WARN**：备库上回放被暂停（有人执行了 `sys_wal_replay_pause()`）。客观线是函数返回 true。查询照常，但备库越落越远，切换时要先回放完，所以 WARN。实采是 false（`repl_node2_*_lsn`），注入要调 `sys_wal_replay_pause()`，是写操作但不重启、不 sudo，撤注入 `sys_wal_replay_resume()`；脚本可以写，阶段 13 验证。
- `repl.sync_short` **WARN**（主库）：`synchronous_standby_names` 非空，而 state=streaming 且在名单里的备库少于要求的个数。**实采发现**：备库 walreceiver 暂停、主库 `sys_stat_replication` 0 行时，主库上的同步提交没有卡住（`repl_node1_*_syncwait_paused` 0 行），怀疑 KES 或 repmgr 会把同步降级为异步，未验证。所以这条不能说"提交会卡住"（那是 FAIL），只能说"同步复制的保证已经没了：要么提交在等，要么已经降级"，WARN。
- 延迟（字节和秒）**只展示**：没有服务器端的客观线（digoal 的 1 分钟/5 分钟是经验值）。**实采提醒**：空闲的主库上 `now() - sys_last_xact_replay_timestamp()` 能到 7 小时（`repl_node2_sys_lsn`：回放时间 13:59，采集 21:23，而 LSN 完全追平），所以文本在 received = replayed 时写"caught up"，不把这个时间差当成延迟。
- 暂停的 walreceiver：只展示 `last_msg`（§1 规定）。

probe：
- `repl.downstreams`：`pid`、`application_name`、`client_addr`、`state`、`sync_state`、`sync_priority`、`sent_lsn`、`write_lsn`、`flush_lsn`、`replay_lsn`、`sent_lag_bytes`、`flush_lag_bytes`、`replay_lag_bytes`、`write_lag_s`、`flush_lag_s`、`replay_lag_s`、`reply_age_s`。LSN 差按角色取（主库 `sys_current_wal_lsn()`，备库上的级联 walsender 用 `sys_last_wal_replay_lsn()`，同 slots）。空闲时 `*_lag` 是 NULL（实采），文本写 `-`。
- `repl.sync`（一行，主库；备库 `not_applicable`）：`synchronous_standby_names`、`synchronous_commit`。名单的解析（`ANY 1( node2)`、`FIRST n (...)`、`n (...)`、`a, b`、`*`）在 rule 里，是纯函数。
- `repl.replay`（一行，备库；主库 `not_applicable`）：`receive_lsn`、`replay_lsn`、`replay_gap_bytes`、`last_replay_age_s`、`replay_paused`。
- `inst.upstream`：复用。**`sys_stat_wal_receiver.conninfo` 不采**：可能含密码。
- kbdiag_ro：两个视图都能看全（实采 `repl_*_ro_*`，同 status 的结论）。

DS：20（复制延迟）、21（备库断连，复用 inst.upstream）；新增 DS-28（同步备库不够数）。L6：`slot.sh`（暂停 walreceiver）→ 主库 `repl.sync_short` WARN（0 行 streaming，名单要 1 个）；备库上只展示 last_msg 涨。

需要用户拍板：
1. **暂停的 walreceiver 要不要判**（§1 的未定事项）。选项：A 不判，只展示 `last_msg`（现状）；B `last_msg_age_s > wal_receiver_timeout`（实验环境 30s）报 WARN——这是服务器自己判定"对端没反应"的那条线，walreceiver 到这时本该自己断开重连，超过它还没动说明进程卡住了（SIGSTOP、D 状态）。**建议 B**：客观、不用新参数，还能顺带让 status 的 `inst.upstream` 用上。实现先按 A。
2. `repl.sync_short` 用 WARN（建议，直到 VM 上核实 KES 的同步降级行为）。
3. `repl.replay_paused` 用 WARN（建议）。

### B.7 cluster（I3）

| # | DBA 想问 | 现在能不能答 | cluster 怎么答 |
|---|---|---|---|
| CL1 | repmgr 认为集群里有哪些节点、谁是主、谁跟谁 | 不能（`repmgr cluster show` 能，但要切到 repmgr 二进制） | `repmgr.nodes`：id、名字、类型、上游、active、优先级、位置、槽名；标出"本节点" |
| CL2 | repmgr 的看法和数据库自己的看法一致吗 | 不能 | 本节点：repmgr 的类型 vs `sys_is_in_recovery()`；主库上：repmgr 里上游是本节点的 active 备库，是否在 `sys_stat_replication` 里（按 application_name 对 node_name，实采 `node2` = `node2`） |
| CL3 | 最近出过什么事（断开、重连、切换、恢复） | 不能 | `repmgr.events` 最近 20 条：节点、事件、成功与否、时间、详情 |
| CL4 | 有没有两个主 | 不能 | repmgr 元数据里 active 的 primary 个数 |

不归 cluster：节点实际能不能连上、repmgrd 在不在跑（`repmgr cluster show` 的 Status 列要逐个连节点，kbdiag 只连一个库，做不了）→ 文本提示到各节点跑 `kbdiag status`；复制落后多少 → `repl`；槽 → `slots`。

参数：无。

判定：
- `cluster.role_mismatch` **WARN**：repmgr 说本节点是 primary 而数据库在恢复中，或反过来。repmgr 会按错误的元数据做切换决策；业务此刻还没受影响。
- `cluster.inactive` **WARN**：repmgr 里 `active = false` 的节点（repmgr 认为它失效或被摘除）：集群少了冗余。
- `cluster.detached` **WARN**（只在主库上判）：repmgr 里上游是本节点、active 的备库不在 `sys_stat_replication` 里（同 repmgr 自己的 "not attached to its upstream"，实采暂停期间 `repmgr cluster show` 就这么报，退出码 25）。
- `cluster.primaries` **WARN**：active 的 primary 超过一个（元数据层面的脑裂）。是不是真脑裂要到两边各跑 `kbdiag status`，所以不给 FAIL。
- next：verify `kbdiag status`（到对应节点上跑）、`kbdiag repl`。

**本节点怎么认**：repmgr 给节点 N 建的槽叫 `repmgr_slot_N`，并写进节点 N 的 `primary_slot_name`（实采 node1 是 `repmgr_slot_1`、node2 是 `repmgr_slot_2`，和 `repmgr.nodes.slot_name` 一一对应）。所以本节点 = `slot_name = primary_slot_name` 的那一行。认不出来（没设 `primary_slot_name` 或对不上）时，CL2 的本节点检查做不了，文本写明，verdict 不能说 OK（UNKNOWN）。这是 repmgr 的惯例，不是 KES 文档的结论，列进 VM 待验点（切换后是否仍成立）。

**连哪个库**：repmgr 元数据在 `esrep` 库（阶段 0），kbdiag 默认连 `test`，而 N-02 规定只占一个连接。做法：`cluster` 在用户没显式给 `-d` 时连 `esrep`；给了就用给的。当前库没有 `repmgr.nodes` 时 probe 是 `not_applicable`，reason 写"no repmgr metadata in database X; use -d"。

probe：
- `cluster.nodes`：`node_id`、`node_name`、`type`、`upstream_node_id`、`active`、`priority`、`location`、`slot_name`、`is_local`。**不采 conninfo**（可能带密码）。
- `cluster.events`：`node_id`、`event`、`successful`、`event_timestamp`、`details`，最近 20 条（这是"最近 N 条"的定义，不是截断；文本标题写明）。
- 复用 `inst.downstreams`（主库）做 CL2。
- **kbdiag_ro**：`permission denied for schema repmgr`（`cluster_*_ro_*`）→ `skipped`（insufficient_privilege），UNKNOWN，reason 提示授予 repmgr schema 的 USAGE 和表的 SELECT。

DS：23（repmgrd 异常：只能看到元数据和事件）；24（备库不具备 promote 条件）只部分回答（active、上游、是否挂着），完整的就绪检查是待排的 `ha`/`ready`。L6：`slot.sh` 暂停 → 主库 `cluster.detached` WARN；干净时 OK、本节点认对。

需要用户拍板：
1. 不给 `-d` 时 `cluster` 自动连 `esrep`（建议；KES 部署工具固定用这个库名）。
2. 本节点按 `primary_slot_name` 认（建议）。
3. 四条都是 WARN（建议）。

### B.8 top-objects（F2）

| # | DBA 想问 | 现在能不能答 | top-objects 怎么答 |
|---|---|---|---|
| TO1 | 库里空间被哪些表占了 | 不能（space 只到库级） | 当前库最大的表（含物化视图），总大小 = 表 + 索引 + TOAST，三部分分开列，给估算行数 |
| TO2 | 哪些索引最大 | 不能 | 最大的索引，所属表 |
| TO3 | 大是因为数据还是 TOAST/索引 | 不能 | 同 TO1 的分列 |

不归 top-objects：库/表空间/WAL → `space`；单表细节 → `table <t>`；膨胀 → `bloat`（待排）；分区表按父表汇总 → `partitions`（以后；文本注明分区各自列出）。

参数：`--limit`（默认 20，表和索引各自裁）。

判定：只展示。多大算大没有客观线。

probe：
- `object.tables`：`schemaname`、`relname`、`relkind`、`total_bytes`、`table_bytes`、`index_bytes`、`toast_bytes`、`reltuples`（`reltuples` 是 float4，ksql 显示成 `1e+06`，SQL 里 cast 成 bigint，实采 `topobj_*_rels`）。
- `object.indexes`：`schemaname`、`relname`、`table_name`、`bytes`。
- 覆盖全部 schema（系统表偶尔也会大），TOAST 表不单独列（算进父表）。算大小要对每个关系 stat 文件，关系很多时有开销，所以 SQL 不加 limit 但也不做别的重活；超时照常 `skipped`。
- kbdiag_ro：大小都看得到，结果和 system 一样（实采）。备库：大小是本地文件，和主库一致（实采 node2 相同）。

DS：新增 DS-29（空间被谁占了：对象级）。

需要用户拍板：默认 20 行（建议）。

### B.9 table \<t\>（F3）

| # | DBA 想问 | 现在能不能答 | table 怎么答 |
|---|---|---|---|
| TB1 | 这张表多大、多少行 | 不能 | 总/表/索引/TOAST 大小，估算行数，活/死元组 |
| TB2 | 最近什么时候 vacuum/analyze 过 | 不能 | 手工和自动各自多久前、次数，自上次 analyze 以来的修改数 |
| TB3 | 有哪些索引，用没用 | 不能 | 名字、定义、大小、是否唯一/主键/有效、扫描次数（累计） |
| TB4 | 冻结年龄 | 不能 | `age(relfrozenxid)`、`mxid_age(relminmxid)`，对照 `autovacuum_freeze_max_age` |
| TB5 | 读得多吗、命中率 | 不能 | seq_scan/idx_scan 次数，heap/idx 块的读和命中（累计值，注明） |
| TB6 | 表级 autovacuum 被关了吗 | 不能 | reloptions 原样 |

不归 table：表上现在的锁 → `locks`（`locks --table` 待排）；精确膨胀 → `bloat --exact`（待排）；列统计 → `colstats`（以后）；分区 → `partitions`（以后）。

参数：无。位置参数 `<t>`。

判定（复用 vacuum、freeze 的规则，只对这张表，不另立规则）：
- `vacuum.table_disabled`（WARN）：同 B.3。
- `freeze.table_age`（WARN/FAIL）：同 B.2 的两条线，对象是这张表（库级 id 是 `freeze.database_age`，表级换一个 id，evidence 字段不同）。

**表名解析**：用 `to_regclass($1)`（参数绑定，不拼 SQL），按标识符规则折叠大小写、按 search_path 找（实采：`kbdiag_inj_tbl` 和 `KBDIAG_INJ_TBL` 都解析到同一张表，`public."KBDIAG_INJ_TBL"` 是 NULL）。找不到时：同 `session <pid>` 找不到 pid 的做法，退出码 3（UNKNOWN），stderr 写"no table X in database Y (unquoted names fold to lower case; use -d for another database)"。理由：表可能在别的库里，不是命令用法错；64 留给参数本身不合法（比如空串）。解析到的不是表（索引、序列、视图）同样 UNKNOWN 并写明类型。

probe：
- `table.info`（一行）：`oid`、`schemaname`、`relname`、`relkind`、`relpersistence`、`reltuples`、`relpages`、`total_bytes`、`table_bytes`、`index_bytes`、`toast_bytes`、`reloptions`、`xid_age`、`mxid_age`。
- `table.stats`（一行）：`sys_stat_user_tables` 加 `sys_statio_user_tables` 的一行（时间转成 `*_age_s`）。**备库上 `not_applicable`**（统计是节点本地的，备库全是 0，实采 `table_node2_*_stat`）。
- `table.indexes`：`indexrelname`、`definition`（`pg_get_indexdef`）、`bytes`、`is_unique`、`is_primary`、`is_valid`、`idx_scan`（备库上是 NULL，理由同上）。
- `freeze.limits`、`vacuum.settings`：复用，给判定用。

DS：新增 DS-30（单表体检：某张表慢或大）；关联 17、19。

需要用户拍板：
1. 表不存在给 3（建议）还是 64。
2. 表级冻结 id 用 `freeze.table_age`（建议）。

### B.10 top（D2）

| # | DBA 想问 | 现在能不能答 | top 怎么答 |
|---|---|---|---|
| TP1 | 哪些 SQL 累计耗时最多（数据库的时间花在哪） | 不能（sessions/waits 只看此刻） | `sys_stat_statements` 按总执行时间排，给次数、总/平均时间、行数、块读/命中、临时块 |
| TP2 | 哪些 SQL 单次最慢 / 调用最多 / 读盘最多 / 写临时文件最多 | 不能 | `--by time|mean|calls|io|temp` 换排序（只影响排序和显示，JSON 行都在） |
| TP3 | 统计开着吗、从什么时候开始算的 | 不能 | 扩展装没装（当前库）、`sys_stat_statements.track` 是不是 none；起点：`sys_stat_statements_info` 不存在（实采），只能写"since the last reset (time unknown)" |
| TP4 | 最近这一分钟谁最耗 | 不能 | 不归 top：`top --interval`（待排） |

不归 top：此刻在跑的 → `sessions`；此刻在等什么 → `waits`；单条 SQL 的执行计划 → `plan`（待排）；KES 原生 SQL 画像 → `top --native`（待排）。

参数：`--limit`（默认 20）、`--by`（默认 time）。`--by` 不是过滤，是场景 TP2 的排序。

判定：只展示。
- **没在收集**：`sys_stat_statements.track=none`（实验环境就是，`top_*_settings`）或扩展没装在当前库（`to_regclass('sys_stat_statements')` 为 NULL）→ `sql.top` 为 `skipped`，reason 写明开关名和怎么开（同 `track_activities` 的做法：查到空时区分"真的没有"和"开关没开"）；verdict UNKNOWN。不用 not_applicable：功能在这个角色上有意义，只是没开。
- kbdiag_ro 看别人的语句：`query` 是 `<insufficient privilege>`、`queryid` 是 NULL，数值列都看得到（实采 `top_node1_ro_stmts_tracked`）→ `redacted[]`（query），UNKNOWN（回答不了"哪条 SQL"）。

probe：
- `sql.top`：`queryid`、`username`、`datname`、`calls`、`total_exec_s`、`mean_exec_s`、`max_exec_s`、`rows`、`shared_blks_hit`、`shared_blks_read`、`temp_blks_written`、`query`（毫秒转秒）。track 的预检同 `trackActivities`：先查设置和扩展，不满足就不跑主 SQL。
- 备库：`sys_stat_statements` 统计的是本节点的查询，照常（实采 node2 装着、track 同样是 none）。

DS：10、11（慢 SQL：累计视角）；12（累计值和当前值分开：文本标题写明 cumulative，修 GAP-5 的文案部分）。L6：实验环境 track=none，天然测"没在收集"；有数据的场景要 `ALTER SYSTEM SET sys_stat_statements.track='top'` 加 reload（写配置，不重启），列为拍板点——建议只在一个会话里 `SET`（阶段 0 的做法）造数据，kbdiag 自己的连接看到的仍是 none，所以 L6 只能验证"没在收集"那一支。

需要用户拍板：
1. track=none 时 `skipped` + UNKNOWN（建议）。
2. `--by` 的 5 个取值（建议 time、mean、calls、io、temp）。
3. 有数据那一支的 L6 要不要改服务器配置（建议不改，用 L2 + 实采覆盖）。

### B.11 新增 DS 场景（写进 PRD §11）

| DS | 场景 | 命令 | L6 怎么造 |
|---|---|---|---|
| 25 | 磁盘或表空间快满 | space | 造不出来（要填满磁盘）；只核对数值和 `df` 一致 |
| 26 | 归档失败 | archive | 实验环境天然在失败 |
| 27 | 参数改了没生效（待重启） | params | `pending_restart.sh`：ALTER SYSTEM + reload，撤注入 RESET + reload |
| 28 | 同步备库不够数 | repl | `slot.sh` 暂停 walreceiver |
| 29 | 空间被哪些对象占了 | top-objects | 实验库现成的大表（orders 119 MB） |
| 30 | 单表体检 | table | 建测试表（同阶段 0 的 `kbdiag_inj_tbl`） |
