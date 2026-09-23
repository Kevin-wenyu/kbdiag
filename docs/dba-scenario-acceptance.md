# kbdiag 诊断能力验收：DBA 故障场景测试集

**状态**：场景设计阶段——本文档只定义 Diagnosis Contract，不改代码、不改测试脚本。
**目的**：现有测试回答的是"命令能不能跑通"，本文档回答的是"kbdiag 能不能在真实故障发生时，帮 DBA 用最短路径找到证据、定位根因、给出下一步动作"。
**产出用途**：作为下一阶段把这些场景变成可执行测试（`test/cases/test_scenario_*.sh`）时的验收基准；本轮不动 `test/`、不动 `lib/`。

---

## 一、现状评估

读完 `test/cases/*.sh`（37 个命令级测试文件）、`test/lib/assert.sh`、`test/setup/*.sql` 后的结论：

**现有测试是"执行+标签"断言，不是"诊断质量"断言。** 三类典型模式：

1. **只断言退出码或"有输出"**（如 `test_locks_wait_runs`：exit 0 就算过；`test_locks_deadlock_output`：有 ≥1 行输出就算过）——不验证内容对不对。
2. **只断言输出包含某个中文标签子串**（如 `test_diagnose_section_locks`：`assert_contains "$out" "锁"` ）——`diagnose` 在没有任何锁等待时根本不会输出"锁"这个 finding，这条测试在健康环境下必然依赖别的巧合文本才能过；更关键的是，**它不验证 kbdiag 报的"锁"是不是测试真正制造的那把锁**，换一个完全不相关的误报也能让测试通过。
3. **少量场景有故障注入但验证肤浅**：`test_locks_hold_shows_locks` 调了 `setup_test_table`（只建表插数据，不加锁），然后断言输出包含 "Lock-holding" 表头字符串——**这条测试从未真正持锁过**，`_locks_hold` 在无锁时同样会打印这个表头（`ok "Lock-holding sessions: none"` 之前没有 header 判断），所以它测的是"命令有 header"，不是"能不能发现持锁会话"。

**唯一命中"制造故障→验证发现"模式的现有资产**：
- `test/setup/make_lock.sql`（`BEGIN; LOCK TABLE ... EXCLUSIVE`，但**从未被任何 test_*.sh 引用执行**——是个孤儿 fixture）
- `test/setup/make_bloat.sql` + `setup_bloat_table()`（有引用，但检索未发现对应的 bloat 场景断言用例）
- `start_slow_query_bg`/`stop_slow_query_bg`（有引用，用于慢查询相关测试，是目前质量最高的一组）
- `test_diagnose_archiver_finding_consistent_with_backup`：**全仓库唯一一条"用另一条命令的裁决结果反向验证 diagnose 结论一致性"的测试**，是这批场景应该效仿的模式。

**结论**：kbdiag 的诊断代码本身（`diagnose`/`advisor`/`check`/`locks`/`perf`/`cluster ready` 等）证据链设计得相当完整——症状/证据/根因/建议四段式在 `cmd_diagnose.sh` 里是显式字段（见 `_diag_long_txn`/`_diag_locks`/`_diag_bloat` 等每个 finding 的 body 构造），远好于测试暴露出来的验收深度。**风险不在代码能力，在测试没有真正验证过这条能力。**

---

## 二、测试环境与可复用基础设施

沿用 `CLAUDE.md` 已定义的 Lima 双节点：

| 节点 | 角色 | 连接 |
|------|------|------|
| kes-node1 | primary | `limactl shell kes-node1` → `sudo -i -u kingbase` |
| kes-node2 | standby | `limactl shell kes-node2` → `sudo -i -u kingbase` |

DB：`ksql -p 54321 -U system test`（本地 socket 免密）。

可直接复用的现有 fixture（`test/setup/` + `test/lib/ssh_helpers.sh`）：

| Fixture | 用途 | 场景覆盖 |
|---------|------|----------|
| `make_test_table.sql` / `setup_test_table()` | 建 `kbdiag_test` 表 + 100 行 | 通用底座 |
| `make_lock.sql` | `BEGIN; LOCK TABLE ... EXCLUSIVE`（未提交） | DS-05/06/07/09 |
| `make_slow_query.sql` + `start_slow_query_bg`/`stop_slow_query_bg` | 后台 30-60s `pg_sleep` | DS-10/11 |
| `make_bloat.sql` / `setup_bloat_table()` | 插 1 万行删一半，不 VACUUM | DS-17 |
| `cleanup.sql` | 清理 | 所有场景收尾 |

新场景需要的 fixture 在下方各条 Setup 里给出，多数是新增 `.sql`/shell 片段，本轮只描述不落地。

---

## 三、Diagnosis Contract 场景集（24 项）

编号规则：`DS-NN`（DBA Scenario）。每条给出 kbdiag 里承接该场景的**具体命令**，Expected Evidence 尽量引用代码里实际会出现的字段/阈值名（如 `KB_WARN_CONN`），使其可直接转成断言。

### 连接（DS-01 ~ DS-04）

#### DS-01: 连接数正常（阴性对照）

**Scenario**：日常健康状态,DBA 巡检确认无异常。

**Setup**：空跑,不注入任何故障,仅确保 `sys_stat_activity` 连接数 < `KB_WARN_CONN`(70%)。

**Expected Symptom**：`kbdiag check` 全绿,`kbdiag diagnose` 报"未发现异常"。

**Expected Evidence**：
- `check`: `Connections: N% (used/max)` 为 `ok`
- `diagnose`: `_diag_connections` 不产生 finding(pct < 70 直接 return)

**Expected Root Cause**：无。

**Expected Action**：无需操作。

**False Positive**：**这是本场景集里最重要的一条**——验证 kbdiag 在健康状态下不会把正常的系统会话(如 `sys_stat_activity` 里 `backend_type <> 'client backend'` 的进程)、或者刚巡检本身建立的连接误判成连接压力。`diagnose`/`check` 的连接数统计包含 kbdiag 自身查询用的连接,需确认 `sys_backend_pid()` 排除逻辑覆盖到位(现状:`_diag_connections`/`_diag_long_txn` 都用了 `pid <> sys_backend_pid()`,但 `check` 的连接数统计`SELECT count(*) FROM sys_stat_activity`**没有**排除自身连接——量级上通常不影响阈值判断,但在阈值临界点(DS-02)时,kbdiag 自己的这条连接可能就是压垮 WARN 判定的最后一根稻草,值得作为已知细节记录而非误报)。

---

#### DS-02: 连接数接近 WARN 阈值

**Scenario**：业务高峰期连接池未回收,DBA 收到连接数告警,需要判断是否要扩容 `max_connections` 还是查连接泄漏。

**Setup**：批量开启空闲连接逼近 `KB_WARN_CONN`(默认 70%)。例如 `max_connections=100` 时,用一个 for 循环起 72 个 `ksql -c "SELECT pg_sleep(60);"` 后台会话(每个占 1 个连接,状态 active,不是 idle)。

**Expected Symptom**：`kbdiag check` 报 `WARN`,`kbdiag status`/`kbdiag sessions` 能看到连接数上升。

**Expected Evidence**：
- `check`: `warn "Connections: 72% (72/100) >= WARN threshold 70%"`,`json_item "connections" "warn" ...`
- `diagnose`: finding category `connections`,body 含"连接数饱和"+ 按 state/应用分解(`_diag_connections` 的 `breakdown`/`top_apps`)

**Expected Root Cause**：连接数逼近阈值,需要区分是瞬时高峰还是持续泄漏——kbdiag 本身不做趋势判断(单点快照),这是 DBA 需要结合 `kbdiag watch 30 check` 人工判断的部分。

**Expected Action**：`kbdiag sessions` 看会话明细按应用/状态分布;若集中在单一 `application_name`,提示检查该应用连接池配置(`_diag_connections` 已有此建议文案)。

**False Positive**：不应把这 72 个后台 `pg_sleep` 会话误判成"慢查询"(`_diag_slow_queries` 用的是 `query_start` 超过 `KB_SLOW_THRESHOLD` 且 `state='active'`——这批会话确实会同时触发慢查询 finding,这是**预期的双重命中**而非误报,因为 `pg_sleep(60)` 本质上就是一条运行超过 5s 阈值的"慢查询"。需要在验收时明确:这条不算 false positive,而是同一次注入合理触发两个独立 finding 的正确行为。)

---

#### DS-03: 连接数达到 FAIL 阈值

**Scenario**：连接池耗尽,新连接被拒绝,应用报 `too many connections`,DBA 需要紧急定位并释放连接。

**Setup**：继续加压至 `KB_FAIL_CONN`(默认 90%)以上,如 92/100。

**Expected Symptom**：`kbdiag check` 报 `FAIL`,exit code 2。

**Expected Evidence**：
- `check`: `fail "Connections: 92% (92/100) >= FAIL threshold 90%"`,`--exit-code` 模式下进程退出码为 2
- `diagnose`: finding level 应为 `CRITICAL`(`_diag_connections` 里 `pct >= KB_FAIL_CONN` 时 `level="CRITICAL"`)

**Expected Root Cause**：连接数超限,body 里的 Top 应用分解应指向具体是哪个 `application_name` 占用最多连接。

**Expected Action**：`_diag_connections` 建议文案"检查连接池配置(PgBouncer/应用侧)"+ `kbdiag kill --idle-txn N` 释放空闲事务腾出连接位。

**False Positive**：验证 `kbdiag kill` 命令本身不会被连接数耗尽卡死(它走的是否是同一个 `ksql_q` 连接池,在 `max_connections` 已满时 kbdiag 自己的诊断查询是否还能连上——这是需要在验收时**明确观察并记录**的边界行为,而不是假设它一定能连上。如果 kbdiag 自己也连不上,应该看到明确的 `query failed` 错误而不是静默假 OK。）

---

#### DS-04: 大量 idle in transaction

**Scenario**：应用 bug 导致事务开启后未提交也未回滚(常见于异常处理漏掉 `finally` 里的 commit/rollback),连接长期占用,可能进一步阻塞 VACUUM(旧事务阻止 xmin 推进)。

**Setup**：起 5-10 个会话执行 `BEGIN; SELECT 1;`(不 COMMIT),用 `nohup ... &` 挂后台,持续 > `KB_WARN_IDLE_TXN`(默认 300s,测试时可用环境变量调低)。

**Expected Symptom**：`kbdiag sessions` 报 idle-in-transaction 告警;`kbdiag check` 的长事务项(阈值 `KB_WARN_TXN=300s`)命中。

**Expected Evidence**：
- `sessions`: `warn "Idle-in-transaction: N session(s) idle > 300s (oldest Xs)"`
- `check`: `Long transactions: N exceeding WARN threshold 300s`(注意 `check` 的长事务查询用 `state IN ('active','idle in transaction')`,不区分两者,验收时要确认告警文案是否让 DBA 误以为是"活跃查询卡住"而不是"忘记提交")
- `diagnose`: `_diag_long_txn` 会标注 `[可安全终止]`——需验证这几个 idle-in-transaction 会话是否真的满足"不持有排他锁+不阻塞任何等待者"这两个条件从而被正确标记为可安全终止

**Expected Root Cause**：应用事务管理缺陷(未提交/未回滚),而非数据库本身问题。

**Expected Action**：`_diag_long_txn` 给出的 `SELECT pg_terminate_backend(<pid>)`(仅限标了 `[可安全终止]` 的);其余提示先用 `kbdiag sql <pid>` 确认。

**False Positive**：**这是验证"可安全终止"标记准确性的关键场景**——必须额外制造一个对照组:另开一个 idle-in-transaction 会话,但让它在事务里持有一把排他锁(`BEGIN; LOCK TABLE kbdiag_test IN EXCLUSIVE MODE;` 不提交),验证 kbdiag **不会**把这个会话标记为 `[可安全终止]`(因为 `has_excl` 检测应该命中)。如果标记逻辑有 bug,DBA 照着建议直接 kill 掉一个正持有锁、可能有业务副作用的连接,是这个功能里风险最高的一种误报。

---

### 锁（DS-05 ~ DS-09）

#### DS-05: 普通锁等待

**Scenario**：会话 A 持有行锁做批量更新,会话 B 尝试更新同一行,进入短暂等待,几秒后 A 提交,B 自动获得锁继续——这是最常见的、通常不需要人工介入的锁等待。

**Setup**：
- Session A: `BEGIN; UPDATE kbdiag_test SET data='x' WHERE id=1;`(不提交,保持 5-10s)
- Session B: `UPDATE kbdiag_test SET data='y' WHERE id=1;`(会阻塞在 A 提交前)

**Expected Symptom**：`kbdiag locks wait` 期间能看到 1 条等待记录;A 提交后立刻清空。

**Expected Evidence**：
- `locks wait`: `warn "Waiting locks: 1 blocked session(s) (longest Xs...)"`,表格里 `WAIT_PID`(B)、`BLOCK_PID`(A)、`MODE`(行锁模式)、`WAIT_QUERY`/`BLOCK_QUERY` 都要能对上真实 PID 和 SQL 文本
- `diagnose`: `_diag_locks` finding,`WARN` 级别,body 里"等待 X on Y,被 pid Z 阻塞"

**Expected Root Cause**：正常的行级并发写冲突,不是异常,是否升级为需要关注取决于等待时长(`KB_WARN_LOCK_WAIT=60s`)。

**Expected Action**：若等待时长 < 60s 且事务很快提交,不需要人工介入;kbdiag 应通过 `Long lock waits: 0 > 60s` 这行明确告诉 DBA"有等待但不严重",避免和 DS-06 的长事务阻塞混为一谈。

**False Positive**：验证这种**短暂正常等待不会被 `check`/`diagnose` 升级为 CRITICAL**——`_diag_locks` 目前固定给 `WARN` 级别不区分等待时长,这是一个值得记录的粒度缺口(见第五节 GAP-2),验收时应如实观察这一行为而非假设它会区分。

---

#### DS-06: 长事务阻塞(锁等待的根因是长事务)

**Scenario**：DS-05 场景里,如果 A 不是几秒后提交,而是一个跑了 10 分钟的报表查询忘了加 `LIMIT`,B 及后续所有想碰这张表的会话全部排队——这是 kbdiag README 示例给出的标准场景。

**Setup**：
- Session A: `BEGIN;` 开启事务,持有对 `kbdiag_test` 的锁(可以是 `LOCK TABLE ... IN EXCLUSIVE MODE` 或对同一行的 `UPDATE`),保持 > 30s 不提交
- Session B: 对冲突资源发起 `UPDATE`/`SELECT ... FOR UPDATE`,进入等待

**Expected Symptom**：`kbdiag diagnose` 应同时产生两个相关联的 finding——`long_txn`(A)和 `locks`(B 等待 A),且 `locks` finding 的阻塞方 PID 应等于 `long_txn` finding 里报告的 PID。

**Expected Evidence**：
```text
Symptom: lock contention
Evidence: PID B waiting for PID A(_diag_locks 的 "被 pid ${block_pid} 阻塞" 一行)
Root Cause: long-running transaction(_diag_long_txn 报告 A 持续时间 > KB_WARN_OLDEST_TXN)
Impact: >= 1 blocked session
Action: inspect PID A(用 kbdiag sql <A> 确认 A 在跑什么)
```
- `long_txn` finding 里 A 不应被标 `[可安全终止]`(A 持有排他锁,`has_excl` 应命中)

**Expected Root Cause**：长事务(报表查询/忘记提交)持锁不放,而非锁机制本身的问题。

**Expected Action**：`kbdiag sql <A的PID>` 确认 A 的 SQL 性质;若确认是可中断的报表查询,`SELECT pg_terminate_backend(<A>)`;若是业务事务,协调业务方尽快提交/回滚。

**False Positive**：验证 kbdiag **不会**把 B(等待方)误认成问题根源——`locks wait` 表格里 B 是 `WAIT_PID`,A 是 `BLOCK_PID`,方向不能反;同时验证 `diagnose` 的两个 finding 之间没有互相矛盾的建议(比如不能一边说"终止 A"一边又把 A 标成安全终止候选之外的矛盾状态)。

---

#### DS-07: 多级锁等待(阻塞链)

**Scenario**：三方阻塞链——C 等 B,B 又在等 A。真实场景常见于:A 做批量更新(持锁),B 是一个想读同一批数据并加了 `FOR UPDATE` 的查询(等 A,同时自己也持有对另一张表的锁),C 想写 B 持有的那张表(等 B)。DBA 只 kill 掉 B 会发现 C 立刻解锁,但表面看 C 报的"阻塞者"如果不是链路终点,排错方向会错。

**Setup**：
- Session A: `BEGIN; UPDATE kbdiag_test SET data='a' WHERE id=1;`(不提交)
- Session B: `BEGIN; UPDATE kbdiag_test SET data='b' WHERE id=2; SELECT * FROM kbdiag_test WHERE id=1 FOR UPDATE;`(第二条语句被 A 阻塞,同时 B 自己因未提交的第一条语句持有对 id=2 的锁)
- Session C: `UPDATE kbdiag_test SET data='c' WHERE id=2;`(被 B 阻塞)

**Expected Symptom**：`kbdiag locks wait` 应同时列出两条等待记录(B 等 A、C 等 B)。

**Expected Evidence**：
- `locks wait` 表格里两行:`WAIT_PID=B, BLOCK_PID=A` 和 `WAIT_PID=C, BLOCK_PID=B`
- **需要验收时重点核实**:`_diag_locks`(diagnose 里的锁 finding)的 SQL 只做了一层 JOIN(`lw`/`lb` 各一次),即只能报告"谁直接阻塞谁",**不会自动把 B→A、C→B 拼成一条 A→B→C 的完整链路**给 DBA。DBA 需要自己读两条独立的 finding 在脑内拼接,或者用 `kbdiag locks wait` 的完整表格自己找规律。

**Expected Root Cause**：链路的真正源头是 A(链首),不是 B——B 只是"中间人",kill B 能暂时解开 C 但 A 仍然会继续挡下一个访问同一行的会话。

**Expected Action**：`kbdiag sql <A>` 确认链首在做什么,而不是优先处理离用户报障最近的 C/B。

**False Positive**：**这是本场景集里专门设计用来验证一个假设缺口的场景**——验证 kbdiag 是否会让 DBA 误以为"kill 掉 B 就是根本解决方案"。如果 kbdiag 的输出没有明确提示"B 自身也在等待另一个会话",DBA 可能会 kill 掉一个无辜的中间事务而没有触及真正持锁的 A。这本身应该被记为一个能力缺口(见第五节 GAP-1),而不是要求"必须通过"——先如实验证现状,再决定是否要为此改代码。

---

#### DS-08: Deadlock

**Scenario**：两个事务以相反顺序申请同一对资源的锁,数据库检测到循环等待,自动 abort 其中一个,应用端报 `deadlock detected`。DBA 需要判断这是偶发还是代码有系统性的加锁顺序问题。

**Setup**：
- Session A: `BEGIN; UPDATE kbdiag_test SET data='a' WHERE id=1;`
- Session B: `BEGIN; UPDATE kbdiag_test SET data='b' WHERE id=2;`
- Session A(继续): `UPDATE kbdiag_test SET data='a2' WHERE id=2;`(等 B)
- Session B(继续): `UPDATE kbdiag_test SET data='b2' WHERE id=1;`(等 A → 循环,数据库应在 `deadlock_timeout` 后检测并 abort 一方)

**Expected Symptom**：`kbdiag locks deadlock` 的累计计数应比注入前 +1;`kbdiag check` 的死锁项从 `ok` 变 `warn`。

**Expected Evidence**：
- `locks deadlock`: `warn "Deadlocks: 1 across 1 database(s) (since stats reset)"`
- `check`: `Deadlocks: 1 since last reset`

**Expected Root Cause**：`sys_stat_database.deadlocks` 是**累计计数器,不重置,不带时间戳**——kbdiag 现有实现**无法回答"这次死锁是什么时候发生的、涉及哪两个 PID、SQL 是什么"**,只能回答"发生过,一共几次"。这是一个需要如实记录的能力边界。

**Expected Action**：`kbdiag locks deadlock` 只能确认"有没有",无法定位"是谁"——DBA 实际排查死锁需要去 KingbaseES 日志(`log_lock_waits`/`deadlock` 相关日志行)里找具体的 SQL 和锁模式,这不在 kbdiag `logs` 命令当前扫描的"慢查询/报错"范围内(需要验收时确认 `kbdiag logs` 是否恰好覆盖了 deadlock 日志行,大概率不覆盖)。

**False Positive**：验证被 abort 的那一方(A 或 B)在 `sessions`/`locks` 里干净退出,不会被误判成"仍在等待锁"的残留状态。

---

#### DS-09: DDL 被业务事务阻塞

**Scenario**：DBA 想给一张热点表加字段/建索引(`ALTER TABLE`/`CREATE INDEX` 非 CONCURRENTLY),需要 `ACCESS EXCLUSIVE` 锁,但业务上有个长事务还在这张表上读写。DDL 排队等待的同时,**新来的所有查询(包括只读 SELECT)也会排在 DDL 后面**——这是最容易造成"业务全站瘫痪"的一类锁场景,和 DS-05/06 的关键区别是"连只读都被挡住"。

**Setup**：
- Session A: `BEGIN; SELECT * FROM kbdiag_test WHERE id=1;`(持有 `ACCESS SHARE` 锁,不提交)
- Session B(DBA): `ALTER TABLE kbdiag_test ADD COLUMN tmp_col TEXT;`(等待 A 释放,自己进入等待队列)
- Session C(新业务查询): `SELECT * FROM kbdiag_test WHERE id=2;`(即使只读,也会排在 B 后面,而不是直接执行)

**Expected Symptom**：`kbdiag locks wait` 应看到 B 等 A(mode=`AccessExclusiveLock`),以及 C 等 B(而不是 C 等 A)——这个排队顺序验证了 PostgreSQL/KingbaseES 的锁队列 FIFO 特性。

**Expected Evidence**：
- `locks wait` 表格里 B 的 `MODE` 列应为 `AccessExclusiveLock`,这是和 DS-05/06/07 里常见的 `RowExclusiveLock`/`ShareLock` 的关键区别——DBA 一眼看 MODE 列就能判断"这是 DDL 卡住了,不是普通业务锁等待"
- `diagnose` 的 `_diag_locks` finding 里应能看到 B 的 `wait_query` 是 `ALTER TABLE ...`

**Expected Root Cause**：DDL 需要独占锁,和任何时长的读写事务都冲突,不是"锁太多"而是"锁类型决定了它必须排在所有人后面"。

**Expected Action**：kbdiag 当前建议文案是通用的"kbdiag sql <block_pid> 确认持锁 SQL",**没有针对"等待方是 DDL"给出专门建议**(例如提示用 `lock_timeout` 限时重试,或在业务低峰期执行,或改用 `CREATE INDEX CONCURRENTLY`)——这是可以考虑但本轮不做的增强点(见第五节 GAP-3)。

**False Positive**：验证 kbdiag 不会把这种"体量正常"的锁等待和 DS-04 的"idle in transaction 长期占用"混淆——A 在本场景里是 active 短查询,不是 idle in transaction,`_diag_long_txn` 不应对 A 产生 finding(因为 A 的持续时间通常不会超过 `KB_WARN_OLDEST_TXN` 除非人为拖长)。

---

### 性能（DS-10 ~ DS-16）

#### DS-10: 单条慢 SQL

**Scenario**：某个报表接口突然变慢,DBA 需要确认是不是有慢 SQL 在跑,以及是不是新出现的模式。

**Setup**：`start_slow_query_bg`(已有 fixture,后台 `SELECT pg_sleep(60);`)。

**Expected Symptom**：`kbdiag perf slow` 报 1 条;`kbdiag diagnose` 的 `slow_queries` finding 命中。

**Expected Evidence**：
- `perf slow`: `warn "Slow queries: 1 running > 5s"`,表格含 PID/USER/DURATION/QUERY
- `diagnose`: `_diag_slow_queries` finding,`INFO` 级别(注意:代码里这条固定是 `INFO` 不是 `WARN`,和其他判定型 finding 级别体系不一致,验收时如实记录而非假设它是 WARN)
- 若 `sys_stat_statements` 已装,应能看到"历史均值"对比小节,判断是"一贯慢"还是"这次异常"

**Expected Root Cause**：`pg_sleep` 场景下根因是"故意的",真实场景根因需要 `kbdiag sql <pid>` 拿到 EXPLAIN 才能看到(seq scan/无索引/统计信息过期等)。

**Expected Action**：`kbdiag sql <pid>` 看执行计划,`kbdiag stmt <queryid>` 看历史统计。

**False Positive**：验证正常的、刚好运行了 4.9s(< 5s 阈值)的查询不会被误报;可作为边界值补充测试(阈值 off-by-one)。

---

#### DS-11: 多条慢 SQL(并发)

**Scenario**：批量任务或 ETL 窗口与业务高峰重叠,多条慢查询同时出现,DBA 需要判断是"个别语句写得差"还是"系统性资源不足导致大面积变慢"。

**Setup**：并发起 5-8 个后台 `pg_sleep(N)`(N 略有差异,模拟真实场景耗时不完全一致),部分附加对 `kbdiag_test` 的实际扫描(`SELECT count(*) FROM kbdiag_test, pg_sleep(6);` 之类让 EXPLAIN 有内容可看)。

**Expected Symptom**：`kbdiag perf slow` 列出全部;`kbdiag diagnose` 的 `slow_queries` finding 里 `cnt` 反映真实数量,且受 `TOP_N`(默认 10)截断。

**Expected Evidence**：
- 数量 ≤ `TOP_N` 时,`perf slow`/`diagnose` 报告的条数应与实际注入数量一致(逐条核对 PID)
- 数量 > `TOP_N` 时(可以加压到 12 条验证),需要确认是否有"仅显示前 N,共 M 条"的提示——**需验收时确认**:`_perf_slow`/`_diag_slow_queries` 目前的 `ORDER BY query_start`/`LIMIT ${TOP_N}` 只是静默截断,没有像 `_perf_bloat`("showing top N")那样明确告诉 DBA 还有多少条被截断,这是个可记录的一致性缺口(见 GAP-4)

**Expected Root Cause**：需要 DBA 自行判断——kbdiag 不做"这是不是同一个模式导致的批量慢查询"的聚类分析,只列清单。

**Expected Action**：`stmt` 历史统计 + `sql <pid>` 逐条排查;如果 SQL 文本高度相似,提示可能是同一段业务逻辑的循环调用。

**False Positive**：验证 8 条注入的慢查询不会因为其中几条 `queryid` 相同(比如同一条 SQL 跑了多次)而在计数上被去重或错误合并——`_diag_slow_queries` 是按 `sys_stat_activity` 逐行算的,不该有去重问题,但 `_diag_stmt_top`(历史均值那部分)是按 `queryid` 聚合的,两处计数口径不同,验收时要分别核对。

---

#### DS-12: Buffer Hit Rate 异常

**Scenario**：内存配置不足或工作集远超 `shared_buffers`,大量物理 IO,查询延迟上升但单条 SQL 本身不一定超过慢查询阈值。

**Setup**：需要真实制造大量物理读——例如建一张明显超过 `shared_buffers` 大小的表(测试 VM 内存有限,可以用一张几百 MB 的表 + 反复全表扫描不同列制造 cache miss),或者更直接的办法:临时把 `shared_buffers` 调到极小值(如 16MB)重启实例,再跑常规查询,人为制造低命中率。

**Expected Symptom**：`kbdiag check` 的 buffer hit rate 项报警(`KB_WARN_HIT=95` / `KB_FAIL_HIT=90`);`kbdiag perf io` 能定位到具体哪些表命中率低。

**Expected Evidence**：
- `check`: `warn "Buffer hit rate: 88.x% < WARN threshold 95%"` 或更低触发 `fail`
- `perf io`: 表格列出具体表的 `HIT_PCT`,应该能看到刚才被扫描的大表命中率最低
- `diagnose --full`: `_diag_buffer_hit` finding,建议"增大 shared_buffers"

**Expected Root Cause**：`shared_buffers` 配置过小相对于工作集,或者存在全表扫描扫穿 cache 的异常大查询(需要用 `perf io` 交叉验证是全局性还是个别表的问题)。

**Expected Action**：`advisor params` 会基于系统内存给出 `shared_buffers` 推荐值(`_advisor_params` 里 `mem_mb * 25 / 100`)——验收时验证这个推荐值在测试 VM 的实际内存下是否合理(不能推荐一个超过物理内存的值)。

**False Positive**：命中率统计是**数据库启动以来的累计值**(`sys_stat_database.blks_hit/blks_read`,不是滑动窗口),验证:如果实例已经启动很久、早期有过一次性大批量导入的物理读,即使现在完全健康,累计命中率也可能长期"被拉低"回不去——这种情况下 kbdiag 会持续误报 WARN,而 DBA 需要知道该看 `stat`(区间采样,反映当前)而不是 `check` 的累计值。**这是一个需要在文档/建议文案里向 DBA 讲清楚的重要陷阱,而不是代码 bug**——验收时应确认 `check`/`perf io`/`diagnose` 里是否有"这是自 stats_reset 以来的累计值"这类提示(目前检索代码,`check` 的 buffer hit 输出**没有**注明是累计值,容易误导 DBA,见 GAP-5)。

---

#### DS-13: IO 等待异常

**Scenario**：存储层出现瓶颈(慢盘、网络存储抖动、并发 IO 过高),会话大量卡在 IO 相关的 wait event 上,而不是锁等待。

**Setup**：制造并发大 IO——例如同时对 DS-12 建的大表跑 5-10 个并发全表扫描(`SELECT count(*) FROM big_table;`),配合把 VM 磁盘 IO 人为限速(如果 Lima/VM 层可控)或至少制造足够并发让 `wait_event_type='IO'` 出现。

**Expected Symptom**：`kbdiag wait`/`kbdiag perf wait` 报告 `wait_event_type=IO` 类别升高。

**Expected Evidence**：
- `wait`: `warn "Lock waits: ..."` 不应命中(这不是锁问题),但 `Waiting sessions: N (longest wait Xs)` 应能看到多条 IO 相关等待——**需验收时确认**:`_WAIT_EVENT_WHERE` 排除了 `Activity`/`Client` 两类,IO 等待应该能通过
- `perf wait`: 按 `wait_event_type, wait_event` 分组的分布表格里能看到 IO 类事件计数升高

**Expected Root Cause**：存储层瓶颈,需要结合 `perf io`(表级 IO 统计)判断是否是某几张表的问题,还是整体存储慢。

**Expected Action**：kbdiag 没有直接的"磁盘延迟"指标(它查的是数据库层面的 wait event,不是 OS 层的 iostat),这是一个**架构性边界**——DBA 最终还是要跳出 kbdiag 去看 `iostat`/云盘监控确认物理层。`check --os` 的 OS 检查目前也不含磁盘 IO 延迟项(只有文件系统类型检查)。

**False Positive**：验证 `_WAIT_EVENT_WHERE` 的排除逻辑(`Activity`/`Client` 类型)不会把真正的 IO 等待也误排除掉——两者是不同的 `wait_event_type` 枚举值,理论上不冲突,但需要在真实 KingbaseES 版本上核实 IO 等待具体报的 `wait_event_type` 字符串是什么(PostgreSQL 生态是 `IO`,需确认 KingbaseES 是否一致)。

---

#### DS-14: CPU/负载异常但 SQL 本身不明显

**Scenario**：主机 CPU 被打满(可能是数据库进程本身计算密集,也可能是其他进程抢占,或是并发量高但单条 SQL 都不算慢),应用普遍感觉"卡",但看不到任何一条超过慢查询阈值的 SQL。

**Setup**：制造大量并发但单条执行都很快的查询(例如 50 个并发 session 各自跑 `SELECT count(*) FROM kbdiag_test WHERE data LIKE '%x%';` 这种单条几百毫秒但因为并发数高导致 CPU 打满的场景),同时可以用 `stress-ng --cpu N` 之类工具在 OS 层直接抢占 CPU 制造对照组。

**Expected Symptom**：这是本场景集里**预期会暴露真实能力缺口**的场景,不是要求 kbdiag "通过"。

**Expected Evidence**：预期 kbdiag **看不出异常**,或只能看到间接信号——`kbdiag stat` 的 TPS 会下降,`kbdiag wait` 可能看到 `CPU`(如果 KingbaseES 有此 wait_event_type)或压根没有直接对应的等待事件(纯 CPU-bound 的查询不在任何 IO/Lock 上等待,`wait_event IS NULL`,天然被 `_WAIT_EVENT_WHERE` 排除,不会出现在 `wait` 命令里)。

**Expected Root Cause**：OS 层 CPU 饱和,kbdiag 全线命令都不直接采集 `/proc/loadavg`、CPU 使用率、`vmstat` 这类系统指标(`check --os` 只查 THP/swappiness/swap/overcommit/ulimit/NTP/文件系统/CPU governor 这 8 项静态配置,**没有实时 CPU 负载数值**)。

**Expected Action**：无——这是要 DBA 明确知道"这个场景请去看 `top`/`vmstat`/主机监控,不要指望 kbdiag"的边界场景。

**False Positive**：N/A——本场景的验收目标是**准确记录 kbdiag 在此场景下的实际表现(大概率是沉默/无 finding)**,写进第五节的能力缺口清单,而不是假装它能发现。这正是用户要求的"哪些不该被误判成这个问题"的反向验证:如果 kbdiag 在纯 CPU 饱和场景下报出了看似相关但实际上文不对题的 finding(比如把并发查询数量升高误判成"慢查询"或"连接数异常"),那才是需要记录的假阳性。

---

#### DS-15: Temp spill(临时文件/排序溢出)

**Scenario**：`work_mem` 配置偏小,复杂排序/哈希聚合/大 `JOIN` 溢出到磁盘临时文件,查询变慢且产生大量磁盘 IO。

**Setup**：跑一条明确会触发排序溢出的查询,例如对 `kbdiag_test` 插入足量数据后 `SELECT * FROM kbdiag_test ORDER BY data;` 并临时调低 `work_mem`(session 级 `SET work_mem='64kB';`)强制触发 spill。

**Expected Symptom**：`kbdiag temp` 报告累计临时文件用量上升;`kbdiag check` 的临时文件项(`KB_WARN_TEMP=100MB`/`KB_FAIL_TEMP=1GB`)可能命中(取决于制造的数据量)。

**Expected Evidence**：
- `temp`: `info "Temp usage (cumulative): N files, XMB across M database(s)"` + "Top statements by temp written" 表格应能看到刚才这条 SQL 的 `queryid`(依赖 `sys_stat_statements` 已装且 `temp_blks_written>0`)
- `diagnose --full`: `_diag_temp` finding,达到阈值才会产生(`KB_WARN_TEMP` 默认 100MB,测试环境可能需要生成较大数据集才能触发,或临时调低 `KB_WARN_TEMP` 环境变量)

**Expected Root Cause**：`work_mem` 相对于查询的排序/哈希工作集过小。

**Expected Action**：`_diag_temp` 建议"增大 work_mem 减少排序溢出"+ `kbdiag temp` 查看具体会话。

**False Positive**：`cmd_temp.sh` 明确注释了"KingbaseES 没有单会话实时 temp 列,只有累计的库级/语句级统计"——验证 kbdiag **不会**把这个累计值包装成看起来像"当前实时"的数据(检查文案措辞是否清楚标注了"cumulative"/"since stats reset"),避免 DBA 误以为看到的是这条 SQL 正在产生的实时 spill。

---

#### DS-16: WAL 增长异常

**Scenario**：写入量突增或归档(`archive_command`)失败,WAL 目录持续增长,最终有撑满磁盘的风险。这是 DS-16(性能类"WAL 增长")和 backup 域的"归档失败"场景有重叠但触发路径不同——本场景聚焦"WAL 生成速率异常"而不仅是"归档失败积压"(DS-16 应覆盖两种子路径)。

**Setup**：
- 子场景 A(归档失败导致积压)：设置一个必然失败的 `archive_command`(如 `archive_command = 'false'` 或指向不存在的目录),开启 `archive_mode=on`,持续写入制造 WAL 切换(`INSERT` 大批量数据)
- 子场景 B(纯粹写入量突增,不涉及归档失败)：`archive_mode=off` 下批量写入,观察 `checkpoints_req` 是否因为 `max_wal_size` 相对写入速率过小而被迫频繁触发

**Expected Symptom**：
- 子场景 A: `kbdiag backup`/`kbdiag check` 的 WAL 归档项报 `fail`(`.ready` 文件数 ≥ `KB_FAIL_WAL_READY=100`)或 `warn`;`kbdiag diagnose` 的 `_diag_archiver` 应给出完整根因链(症状→证据→根因→建议,这是代码里少数已经做到用户要求的四段式模板的 finding)
- 子场景 B: `kbdiag perf wal` 报 `warn "Checkpoints: N requested vs M timed — forced checkpoints may indicate max_wal_size is too small"`

**Expected Evidence**（子场景 A）：
```text
症状：N 个 WAL 段待归档（.ready 积压）
证据：archiver 累计失败 N 次 | 最后失败段: X @ time | 最后成功: time
根因：archive_command 执行失败: <实际配置的命令>
建议：kbdiag backup 查看完整归档/备份链检查 / 手工执行 archive_command 看真实报错
```

**Expected Root Cause**：子场景 A 是 archive_command 配置/权限/目标磁盘问题;子场景 B 是 `max_wal_size` 相对写入吞吐过小。

**Expected Action**：子场景 A 按 `_diag_archiver` 给的路径手工执行 archive_command 排查;子场景 B 调大 `max_wal_size` 或 `checkpoint_completion_target`(`advisor params` 已覆盖后者)。

**False Positive**：验证子场景 B(纯写入量大但归档本身健康)不会被误判为"归档失败"——两条判断路径在代码里是独立的(`_diag_archiver` 看 `archiver` 状态,`_perf_wal` 看 `checkpoints_req` vs `checkpoints_timed`),理论上不会混淆,但需要在同一次测试里两个子场景分别单独触发,确认互不误报对方的 finding。

---

### Vacuum / Storage（DS-17 ~ DS-19）

#### DS-17: Dead tuples / bloat

**Scenario**：大量 `UPDATE`/`DELETE` 后 autovacuum 没跟上,表膨胀,查询扫描效率下降,磁盘空间被无效占用。

**Setup**：复用现有 `make_bloat.sql`/`setup_bloat_table()`(插 1 万行删一半,不 VACUUM)。

**Expected Symptom**：`kbdiag perf bloat`/`kbdiag idx bloat` 报告;`kbdiag diagnose --full` 的 `_diag_bloat` finding。

**Expected Evidence**：
- `perf bloat`: 表格里 `kbdiag_test` 的 `LIVE_PCT` 应明显低于 `100 - KB_WARN_DEAD_PCT`(默认即 80%)
- `diagnose --full`: dead_pct 超过 `KB_FAIL_DEAD_PCT`(30%)时应为 `CRITICAL`("表膨胀严重"),否则 `WARN`("Autovacuum 积压")——验收时用刚好 20%~30% 之间和 >30% 两组数据分别验证级别切换是否准确
- `advisor vacuum --fix`: 应生成 `VACUUM (ANALYZE) kbdiag_test;`

**Expected Root Cause**：autovacuum 未及时清理(可能是 `autovacuum_vacuum_scale_factor` 配置不适合大表,或本身 autovacuum 被关闭/停滞——需要和 DS-18 交叉验证)。

**Expected Action**：手动 `VACUUM (ANALYZE)` 或 `advisor vacuum --fix` 生成的 SQL;长期方案是调整 autovacuum 参数或对大表单独配置 per-table 阈值。

**False Positive**：验证一张**行数少(< 1000 行)**但删了一半数据的小表**不会**触发 bloat 告警——`_perf_bloat`/`_diag_bloat` 的 WHERE 条件都有 `n_live_tup+n_dead_tup > 1000` 这道行数门槛,专门排除小表噪音,需要显式测一张小表确认这道门槛生效(否则每个空表/配置表都会常年报膨胀,是很容易踩的噪音源)。

---

#### DS-18: Autovacuum 长时间未执行

**Scenario**：autovacuum 因为长事务持续阻挡(旧事务的 snapshot 阻止死元组被回收判定)、或 `autovacuum=off`、或 worker 数不够导致排队,表长期没有被自动清理过,即使 dead tuple 比例还不算太高,"多久没清理过"本身就是一个独立信号。

**Setup**：
- 子场景 A：`ALTER TABLE kbdiag_test SET (autovacuum_enabled = false);` 后持续写入删除,观察 `last_autovacuum` 停滞不前
- 子场景 B：制造一个长期存在的事务(类似 DS-04,但要足够长,比如几十分钟)阻挡 autovacuum 的 xmin 推进,同时正常写入删除该表

**Expected Symptom**：`kbdiag perf vacuum` 的"stale > 7d"分支命中(注意:测试环境短时间内无法真实等到 7 天,子场景 A 可以直接验证"从未 vacuum 过"的 `never` 值路径,这是可以在短时间内验证的部分;真正的 7 天陈旧判断逻辑本身只能靠代码审查确认 SQL 条件正确,无法在验收阶段实测)

**Expected Evidence**：
- `perf vacuum`: `over_cnt`(超过 autovacuum 阈值)和 `cnt - over_cnt`(仅陈旧但未超阈值)是两个独立计数,输出里有 `info "Also stale (no autovacuum in 7d): N"` 这行——验收时确认这两类是否被清楚区分,还是混在一起让 DBA 分不清"到底是快超标了还是纯粹很久没人管"

**Expected Root Cause**：子场景 A 是配置问题(autovacuum 被关);子场景 B 是长事务副作用(和 DS-04/DS-06 是同一个根因在不同表现层面的体现——这是一个很好的"多个场景共享同一根因"的交叉验证点)。

**Expected Action**：子场景 A 恢复 `autovacuum_enabled=true`;子场景 B 先解决长事务(参见 DS-06 的处理路径),autovacuum 会自然恢复。

**False Positive**：**这是验证"根因关联"能力的关键场景**——理想情况下,`diagnose --full` 应该能让 DBA 看出"这张表 vacuum 停滞"和"那个长事务"是同一件事的两个症状,而不是把它们呈现成两个无关的独立 finding 让 DBA 自己去连。验收时如实记录 `_diag_bloat`(表膨胀 finding)和 `_diag_long_txn`(长事务 finding)之间是否有任何交叉引用——检索代码确认目前**没有**(两个 finding 完全独立生成,互不引用),这是需要记录的关联缺口(见 GAP-1 的另一个具体案例)。

---

#### DS-19: Freeze age 风险(XID wraparound)

**Scenario**：表长期未做 `VACUUM FREEZE`,`relfrozenxid` age 持续增长,一旦接近 20 亿(`2^31`)的 wraparound 上限,数据库会拒绝写入并强制进入单用户模式清理——这是数据库运维里少数几个"不处理会导致完全停机"的场景之一,优先级极高。

**Setup**：**测试环境很难在合理时间内真实把 age 刷到 10 亿+(`KB_WARN_XID` 默认值)**,更现实的验收方式是临时调低 `KB_WARN_XID`/`KB_FAIL_XID` 环境变量(比如降到几万),配合正常的事务提交量(每次提交消耗一个 XID)让 age 自然增长穿过这个人为调低的阈值,而不用真的等到十亿量级。

**Expected Symptom**：`kbdiag check`/`kbdiag advisor vacuum` 的 XID age 项从 `ok` 变 `warn`/`fail`。

**Expected Evidence**：
- `check`: `warn "XID age: N >= WARN threshold ${KB_WARN_XID}"`
- `advisor vacuum`: 列出具体哪些表 `xid_age > 500000000`(注意:`advisor vacuum` 的 freeze 判断阈值是**硬编码的 5 亿**,不读 `KB_WARN_XID` 环境变量——这是两处判断使用不同阈值来源的不一致,验收时应如实记录,调低 `KB_WARN_XID` 不会影响 `advisor vacuum` 这条判断的触发点,见 GAP-6)
- `diagnose`: `_diag_xid_age` finding,body 含"距 wraparound 还有约 N 个事务"+ 高风险表 Top 5

**Expected Root Cause**：`autovacuum_freeze_max_age` 相关的自动 freeze 机制被阻挡(常见原因和 DS-18 一样:长事务、autovacuum 被禁用、或者表实在太大 freeze 跟不上写入速度)。

**Expected Action**：`_diag_xid_age` 建议 `VACUUM FREEZE <table>;` + `kbdiag advisor vacuum` 查看完整建议。

**False Positive**：验证 `advisor vacuum` 和 `check`/`diagnose` 报告的"高风险表"名单在同一次运行里是否一致——三处分别用不同 SQL(`sys_class` 直查 vs JOIN `sys_stat_user_tables`)算 `relfrozenxid` age,理论上结果应该相同,但实现路径不同,值得交叉核对避免同一张表在不同命令里报出不同的 age 数值造成 DBA 困惑。

---

### HA / 集群（DS-20 ~ DS-24）

#### DS-20: Standby replication lag

**Scenario**：主备复制延迟增大,可能是网络带宽不足、standby 硬件性能不够、或者 standby 上有其他重负载查询占用资源导致 WAL apply 跟不上。

**Setup**：在 standby(kes-node2)上执行 `SELECT pg_wal_replay_pause();` 制造人为延迟(比直接抠网络/带宽更可控、更适合测试环境复现),持续写入 primary 一段时间后观察延迟增长,再 `pg_wal_replay_resume()` 观察收敛。

**Expected Symptom**：primary 上 `kbdiag check`/`kbdiag replication` 报延迟告警(`KB_WARN_LAG=30s`/`KB_FAIL_LAG=300s`);`kbdiag diagnose` 的 `_diag_replication` 应能看到延迟趋势(增大中/收敛中)。

**Expected Evidence**：
- `check`(primary): `warn "Replication lag: Xs >= WARN threshold 30s"`
- `diagnose`(primary): `_diag_replication` 用两次 3 秒间隔采样算 delta,body 里 `方向：增大中(+Ns/3s)` 应在 `pg_wal_replay_pause` 期间为正,`resume` 后应转为"收敛中"
- standby 上 `kbdiag check` 走的是另一条路径(直接查 `pg_last_xact_replay_timestamp`),两边(primary 视角的 `write_lag` vs standby 视角的 `replay delay`)理论上应该在数量级上互相印证,建议交叉验证不是各自为战

**Expected Root Cause**：本场景是人为暂停 replay,真实场景根因需要结合 standby 主机资源(CPU/IO,kbdiag 查不到,见 DS-14 的同类边界)或网络带宽判断,kbdiag 只能告诉"延迟多少、在变大还是变小",不能告诉"为什么"。

**Expected Action**：`_diag_replication` 建议"kbdiag replication 查看详情";延迟持续增大且非人为暂停时,需要 DBA 去 standby 主机层面排查(kbdiag 覆盖不到)。

**False Positive**：验证 primary 完全空闲(没有写入)时,即使 standby 被 `pause`,延迟数字也不会凭空增长(`_diag_replication` 的算法依赖 `write_lag` 而不是"距今多久没同步",纯空闲不应该报警)——这个对照有助于确认 kbdiag 测的是"复制赶不上写入"而不是"复制看起来很久没动静"。

---

#### DS-21: Standby disconnected

**Scenario**：standby 进程崩溃、网络中断或被人为下线,primary 完全失去这个复制目标,如果没有其他 standby,意味着 HA 能力已经名存实亡。

**Setup**：在 kes-node2 上停止 kingbase 进程(或者更贴近真实故障的做法:`iptables` 阻断 node2 到 node1 的复制端口,模拟网络分区而非进程本身死亡——两种故障模式在 kbdiag 视角下可能表现不同,建议都测)。

**Expected Symptom**：primary 上 `kbdiag replication` 报"No standbys connected";`kbdiag cluster`(repmgr 视角)应该也能看到 node2 状态异常。

**Expected Evidence**：
- `replication`(primary): `warn "No standbys connected"`,`json_item "standbys" "warn" "0" ...`
- `cluster`(primary,repmgr 视角): `warn "Cluster: 1 of 2 node(s) not running"`(依赖 `repmgr cluster show` 的 status 列包含 `running` 字样的判断逻辑,进程崩溃 vs 网络分区这两种故障 repmgr 上报的具体 status 文案可能不同,需要实测确认)
- `cluster ready`(primary): 第 6 项"标准备份连接"应该 `fail`("Standby attach: 0 of 1 standby(s) streaming")

**Expected Root Cause**：进程崩溃 vs 网络分区是两种完全不同的根因,`kbdiag replication`/`cluster` 目前**只能看到"连接没了"这个结果,不能区分是 standby 死了还是网络断了**——这是需要如实记录的边界(见 GAP-7),现实中这个区分很重要(前者需要重启/重建 standby,后者可能几分钟后自愈)。

**Expected Action**：`cluster ready` 的完整检查清单能帮 DBA 系统性确认"现在到底还有没有 failover 能力",这是它比单看 `replication` 更有价值的地方——验收时重点验证 `cluster ready` 在 standby 完全失联时,第 2 项(promotion candidate)是否正确报 `fail`("no standby registered — nothing to fail over to",在只有一个 standby 且它失联的两节点集群里,这条应该命中)。

**False Positive**：验证 kingbase 进程只是短暂重启(比如几秒内的服务重载)而非真正的长期断连时,不会被过度解读成"HA 能力已丧失"——这需要人工卡好时间窗口测试,不属于本轮重点。

---

#### DS-22: Replication slot 堵塞

**Scenario**：一个复制槽(可能是逻辑复制、也可能是曾经存在过的物理复制 standby 留下的槽)没有消费者持续读取,`restart_lsn` 停滞不前,WAL 因为这个槽的存在被强制保留,即使 standby 本身健康,这个孤儿槽也会持续吃盘,最终撑爆磁盘。

**Setup**：`SELECT pg_create_physical_replication_slot('kbdiag_test_slot');`,创建后不接任何消费者,持续对 primary 写入制造 WAL 增长,观察这个槽的 `restart_lsn` 停滞而 `pg_current_wal_lsn()` 持续前进。

**Expected Symptom**：`kbdiag check` 的 slot lag 项(`KB_WARN_SLOT=100MB`/`KB_FAIL_SLOT=1GB`)报警;`kbdiag cluster ready` 的第 7 项(slots)报 `warn`。

**Expected Evidence**：
- `check`: `warn "Replication slot lag: XMB >= WARN threshold 100MB"`(需要写入足够数据积累到 100MB 差值,测试时可以临时调低 `KB_WARN_SLOT` 环境变量加速验证)
- `cluster ready`: `warn "Slots: inactive slot(s): kbdiag_test_slot — WAL retention risk"`(这条判断的是 `active=false`,和 `check` 判断的 `pg_wal_lsn_diff` 是两个不同维度——一个孤儿槽通常两条都会触发,但**槽是 active 的但消费者消费很慢**这种情况下只有 `check` 的 lag 判断会触发,`cluster ready` 的 inactive 判断不会,这是两个互补而非重复的检查,验收时应分别构造两种子场景验证)

**Expected Root Cause**：复制槽没有被正确清理(常见于 standby 下线后忘记 `pg_drop_replication_slot`),或逻辑复制订阅端长期离线。

**Expected Action**：确认这个槽是否还需要,不需要则 `SELECT pg_drop_replication_slot('kbdiag_test_slot');`。

**False Positive**：验证一个**正常工作、有 standby 在消费**的复制槽不会被误判——需要在有 DS-20/21 场景的健康 standby 并存时对照测试,确认"正常槽"和"孤儿槽"在 `active` 列和 `lag` 数值上有清晰区分。同时验证清理该槽后(`pg_drop_replication_slot`),后续检查立刻恢复干净,不留残影。

---

#### DS-23: repmgrd 异常

**Scenario**：`repmgrd` 守护进程(负责自动故障检测和 failover)在某个节点上崩溃或被人为停止,集群此时看起来一切正常(数据库本身没问题),但一旦真的发生 primary 故障,**不会有自动 failover**——这是一种"平时无感知,关键时刻要命"的静默风险,恰恰是 kbdiag `cluster ready` 存在的核心价值。

**Setup**：在其中一个节点上停止 repmgrd(`systemctl stop repmgrd` 或直接 kill 对应进程)。

**Expected Symptom**：数据库本身 `kbdiag status`/`kbdiag check` 应该完全正常(repmgrd 停止不影响数据库本身可用性——这本身也是一个需要验证的对照点);只有 `kbdiag cluster ready` 能发现问题。

**Expected Evidence**：
- `cluster ready`: `fail "repmgrd: <node>(not running) — automatic failover is DISABLED"`,这是 `_cluster_ready` 第 4 项,直接读 `repmgr daemon status --csv` 的 f5(repmgrd 运行位)
- 同时验证:`kbdiag cluster`(不带 `ready`)这个更"看层"的命令**不会**发现这个问题——它只读 `repmgr cluster show` 的节点 running 状态,不检查 repmgrd 守护进程本身,这正是 README 里"看层给事实,断层/cluster ready 做更深检查"分层设计的体现,值得作为正面案例验证分层是否真的按预期工作而非重复劳动

**Expected Root Cause**：repmgrd 进程被停止/崩溃(测试用例是人为停止;真实场景可能是 OOM kill、配置错误导致启动失败等,kbdiag 查不到"为什么停的",只能查到"现在没在跑")。

**Expected Action**：重启 repmgrd(`repmgrd -f repmgr.conf` 或对应的服务管理命令),重启后需要重新跑 `cluster ready` 确认 f5/f7(paused 位)都恢复正常。

**False Positive**：**核心反向验证点**——确认 `kbdiag status`/`kbdiag check`(这些看层/健康检查命令)在 repmgrd 停止时**不会**误报数据库本身有问题(两者是完全独立的进程,不应互相污染判断)。如果连基础 `check` 都因为 repmgrd 缺失而报错或报警,说明检查逻辑有耦合 bug。

---

#### DS-24: Standby 不具备 promote 条件

**Scenario**：即使 repmgrd 在跑、复制连接也正常,standby 本身可能因为配置问题(`hot_standby=off`)、被人为设置了应用延迟(`recovery_min_apply_delay`)、或者 WAL replay 被暂停(`pg_wal_replay_pause`,常见于 DBA 做只读分析时忘记恢复),而实际上不能被 promote——这种"看起来在同步,但不能真的接管"的场景,是 `cluster ready` 第 9 项专门设计要抓的。

**Setup**：三选一或全测(相互独立,可分别验证):
- 子场景 A：standby 上 `SELECT pg_wal_replay_pause();`(直接复用 DS-20 的手段,但这次验证的是 `cluster ready` 而不是 lag 数值)
- 子场景 B：standby 的 `postgresql.conf`/`kingbase.conf` 里设置 `recovery_min_apply_delay = '60s'` 后重启
- 子场景 C：standby 设置 `hot_standby = off` 后重启(此时 standby 通常连只读查询都不接受,是最极端的一种)

**Expected Symptom**：`kbdiag cluster ready`(在 primary 或任一节点跑均可,因为这条检查会通过 `_repmgr_q` 远程连 standby)第 9 项报 `fail`/`warn`。

**Expected Evidence**：
- 子场景 A: `fail "Standby <name>: WAL replay is PAUSED — promotion would stall"`
- 子场景 B: `warn "Standby <name>: recovery_min_apply_delay=60 delays promotion"`(注意这条是 `warn` 不是 `fail`——因为有延迟但最终仍可 promote,只是不是立即;子场景 A 的 pause 是 `fail`,因为不手动 resume 永远卡住,两者严重程度分级需要在验收时确认符合直觉)
- 子场景 C: `fail "Standby <name>: hot_standby=off"`

**Expected Root Cause**：三种配置/操作性问题,均非复制链路本身故障(复制本身可能显示 streaming 正常),这正是这一检查项存在的意义——**光看复制连上了不代表能 failover**。

**Expected Action**：子场景 A 执行 `pg_wal_replay_resume()`;子场景 B/C 修改配置并重启 standby。

**False Positive**：这是全场景集里最该重视误报风险的一条——验证一个**完全健康、可以立即 promote 的 standby** 不会被误判成三者之一(即 `sb_hot='on'`、`sb_delay='0'`、`sb_paused='false'` 时必须干净地报 `ok`,这是 DS-20/21/23 里其他健康 standby 场景应该顺带覆盖到的对照)。另外要验证:当 standby 本身**网络不可达**(区别于配置问题)时,走的是第 246 行的 `unreachable` 分支(`fail "Standby $sb_name ($sb_host): unreachable via SQL"`),不会被误判成上述三种"活着但配置不对"的情况而给出误导性的"改配置"建议——不可达时给的建议应该是排查网络连通性,不是改 `recovery_min_apply_delay`。

---

## 四、场景矩阵总览

| # | 场景 | 承接命令 | 严重度分级验证 | 已知/预期缺口 |
|---|------|----------|:---:|:---:|
| DS-01 | 连接正常(对照) | check | — | — |
| DS-02 | 连接接近 WARN | check/diagnose | ✓ | — |
| DS-03 | 连接达到 FAIL | check/diagnose | ✓ | — |
| DS-04 | 大量 idle in txn | sessions/check/diagnose | — | 安全终止标记需重点验证 |
| DS-05 | 普通锁等待 | locks/diagnose | — | GAP-2(不分等待时长) |
| DS-06 | 长事务阻塞 | diagnose(locks+long_txn) | — | — |
| DS-07 | 多级锁等待 | locks | — | GAP-1(不拼接链路) |
| DS-08 | Deadlock | locks deadlock/check | — | 无法定位具体 PID/SQL |
| DS-09 | DDL 被阻塞 | locks/diagnose | — | GAP-3(无专门建议) |
| DS-10 | 单条慢 SQL | perf slow/diagnose | — | — |
| DS-11 | 多条慢 SQL | perf slow/diagnose | — | GAP-4(截断不提示) |
| DS-12 | Buffer Hit 异常 | check/perf io/diagnose | ✓ | GAP-5(未标注累计值) |
| DS-13 | IO 等待异常 | wait/perf wait | — | 无 OS 层 iostat |
| DS-14 | CPU 负载异常 | (预期无覆盖) | — | 架构性缺口,重点场景 |
| DS-15 | Temp spill | temp/diagnose | — | — |
| DS-16 | WAL 增长异常 | backup/check/perf wal/diagnose | — | — |
| DS-17 | Bloat | perf bloat/advisor/diagnose | ✓ | — |
| DS-18 | Autovacuum 停滞 | perf vacuum | — | 与 DS-06 关联未打通 |
| DS-19 | Freeze age | check/advisor/diagnose | — | GAP-6(阈值来源不一致) |
| DS-20 | 复制延迟 | check/replication/diagnose | ✓ | — |
| DS-21 | Standby 断连 | replication/cluster/cluster ready | — | GAP-7(不区分崩溃/网络) |
| DS-22 | 复制槽堵塞 | check/cluster ready | — | — |
| DS-23 | repmgrd 异常 | cluster ready(对照 cluster/check) | — | — |
| DS-24 | Standby 不可 promote | cluster ready | ✓ | — |

---

## 五、已识别的能力缺口(非本轮修复,仅记录)

- **GAP-1 锁链路不自动拼接**（DS-07/DS-18）：`_diag_locks` 只做一层直接阻塞 JOIN,多级阻塞链和"膨胀 finding 与长事务 finding 同根同源"都需要 DBA 自己在多个 finding 间做关联,kbdiag 不提供。
- **GAP-2 锁等待不区分严重程度**（DS-05）：`_diag_locks` 固定给 `WARN`,不像 `_diag_long_txn` 那样按时长升级到 `CRITICAL`。
- **GAP-3 DDL 阻塞无专门建议**（DS-09）：等待方是 DDL 时,建议文案和普通锁等待一样通用,没有针对性提示(`lock_timeout`/`CREATE INDEX CONCURRENTLY` 等)。
- **GAP-4 Top N 截断无提示**（DS-11）：`_perf_slow`/`_diag_slow_queries` 静默截断到 `TOP_N`,不像 `_perf_bloat` 那样告知"还有 M 条未显示"。
- **GAP-5 Buffer Hit 累计值未标注**（DS-12）：`check`/`_diag_buffer_hit` 报的是数据库启动以来的累计命中率,不是当前状态,文案未提示这一点,长期运行的实例容易被历史一次性物理读拖累出持续误报。
- **GAP-6 Freeze 阈值来源不一致**（DS-19）：`check`/`diagnose` 用可配置的 `KB_WARN_XID`(默认 10 亿),`advisor vacuum` 用硬编码 5 亿,同一实例可能在两个命令里得到不一致的"要不要处理"结论。
- **GAP-7 Standby 失联不区分崩溃/网络分区**（DS-21）：`replication`/`cluster` 只能看到"连接没了"的结果,不能提示 DBA 该往哪个方向排查。
- **GAP-8(架构性) 无 OS 层实时资源指标**（DS-13/DS-14）：CPU 负载、内存压力(除 `check --os` 的静态 swap/overcommit 配置项外)、磁盘 IO 延迟均不在 kbdiag 采集范围内,这类场景 kbdiag 天然沉默,需要 DBA 明确知道要切到主机层监控。

这些缺口是否值得修，以及优先级如何排，留待场景集本身通过验收后再讨论——本轮不动代码。

---

## 六、下一步（待用户拍板）

本文档只是 Diagnosis Contract 设计稿。往下有三条路径，具体走哪条/是否要走，等场景集本身先经过你审阅：

1. 把 24 个场景转成 `test/cases/test_scenario_*.sh`，复用/扩展 `test/setup/` fixture，跑真实注入+断言（工作量最大，但是唯一能验证"诊断质量"而非"命令执行"的方式）。
2. 先挑高优先级的几个（长事务阻塞链 DS-06、cluster ready 全套 DS-21/23/24、bloat DS-17）落地，其余场景留作文档，按需再转测试。
3. 针对第五节列出的 GAP，单独开 issue 讨论是否要修，与场景测试解耦。
