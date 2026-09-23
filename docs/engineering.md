# kbdiag 2.0 工程方案

状态: active | 最后核对: 2026-09-24

**职责**：选型、架构、工程约定、测试分层、故障注入手段、运行命令。需求和契约看 `docs/PRD.md`，查询条目和验证状态看 `docs/queries.md`。

---

## 1. 选型：Go + pgx/v5

`CGO_ENABLED=0` 交叉编译成单个静态二进制。放行前的技术验证已通过（§3）。

### 1.1 硬约束

| 约束                                         | 来源                       |
| ------------------------------------------ | ------------------------ |
| C1 单一产物，目标机不装运行时                           | PRD N-01，用户确认            |
| C2 直连数据库线协议，不调 `ksql` 子进程                  | 用户确认                     |
| C3 在 Mac 上开发，交叉编译到 linux/amd64、linux/arm64 | 现有开发环境                   |
| C4 判定逻辑能脱离数据库做单测                           | PRD N-05，旧版病根            |
| C5 代码主要由 AI 写、由 DBA 背景的用户审                 | 用户说过"我已经审核不了"——代码必须朴素、好读 |

C5 是最容易被忽略、但对这个项目最重要的一条：**选型要优化"审得动"，而不是"写得炫"**。

### 1.2 候选对比

|         | Go                                    | Rust                          | Python（打包）                                       | 继续 Shell           |
| ------- | ------------------------------------- | ----------------------------- | ------------------------------------------------ | ------------------ |
| C1 单产物  | ✅ 静态二进制，天生                            | ✅ 需 musl target               | ⚠️ PyInstaller/Nuitka 打包，依赖目标机 glibc 版本，体积 30MB+ | ✅                  |
| C2 直连协议 | ✅ pgx 成熟，纯 Go                         | ✅ tokio-postgres / sqlx       | ✅ pg8000 纯 Python                                | ❌ 只能调 ksql         |
| C3 交叉编译 | ✅ `GOOS=linux GOARCH=arm64` 一行        | ⚠️ 需 cross/zig 工具链            | ❌ 必须在目标架构上打包                                     | ✅                  |
| C4 可测试  | ✅ 标准库 testing + 表驱动 + 原生 fuzz         | ✅                             | ✅ pytest                                         | ❌ 旧版已证明            |
| C5 好读好审 | ✅ 语法少、写法单一、`gofmt` 统一风格               | ❌ 生命周期/trait/async 对非专业审阅者是负担 | ✅ 最好读                                            | ❌ `set -e` 陷阱、引号地狱 |
| 类型安全    | ✅ 结构体 + 编译检查                          | ✅✅                            | ⚠️ 需 mypy 纪律                                     | ❌                  |
| 同类工具先例  | pgmetrics、pgcenter、pg_timetable 都是 Go | 少                             | pgcli 等交互工具                                      | 旧 kbdiag           |

**淘汰理由**：

- **Shell**：直接违反 C2、C4，且就是旧版失败的载体。
- **Python**：C1/C3 实际上做不到"拷过去就能跑"——KingbaseES 客户常见 CentOS 7 / 麒麟等老 glibc 系统，打包产物跨发行版不可靠。
- **Rust**：能力上完全够，但 C5 输掉——一个诊断 CLI 用不到 Rust 的内存安全优势（没有高并发、没有不可信输入解析），却要付出审阅成本和编译时间。

**选 Go 的核心理由**（按权重）：

1. C1+C3 零成本：`CGO_ENABLED=0 go build` 直接出静态二进制，Mac 上一条命令出两个架构
2. C5：Go 刻意限制写法，AI 写出来的代码和人写的长得一样，审阅者不需要懂"高级技巧"
3. pgx 是 PostgreSQL 生态最成熟的纯 Go 驱动，同类诊断工具 pgmetrics 已验证过这条路

## 2. 依赖（按复杂度惩罚原则，每个依赖对应一条当下需求）

| 依赖                        | 用途              | 对应需求        | 不用它的代价                                     |
| ------------------------- | --------------- | ----------- | ------------------------------------------ |
| `github.com/jackc/pgx/v5` | 数据库线协议、SCRAM 认证 | C2          | 自己实现协议，不现实                                 |
| `github.com/spf13/cobra`  | 子命令、自动生成英文 help | PRD §4 命令模型 | 用标准库 `flag` 手写 dispatch + help，约 200 行重复代码 |
| 测试：只用标准库 `testing`        | —               | —           | —                                          |

**不引入**：ORM、配置文件库（配置只有 flag + `KB_*` 环境变量，标准库足够）、日志框架（`log/slog` 标准库）、断言库（testify 等，标准库 + `go-cmp` 按需再议）、mock 框架（架构上不需要，见 §4）。

## 3. 技术验证结果（2026-09-23）

Go 1.27.1 + pgx v5.11.0，linux/amd64 静态二进制 14MB，在 kes-node1 以 kingbase 用户直接运行。

| #   | 验证项         | 结果  | 要点                                                                                                                                                                                                                                                                                                                 |
| --- | ----------- | --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| S1  | 本地 socket   | 通过  | socket 是 `/tmp/.s.KINGBASE.54321`，不是 PG 的 `.s.PGSQL.54321`。用 `DialFunc` 直连该路径可以免密连上。pgx 报错时仍按 `.s.PGSQL` 显示路径，conn 层包装错误时要改写                                                                                                                                                                                       |
| S2  | TCP + SCRAM | 通过  | `kbdiag_ro` 走 127.0.0.1 + SCRAM 连通                                                                                                                                                                                                                                                                                 |
| S3  | 类型与语义       | 通过  | 10 条 probe 在 cache statement 和 simple protocol 两种模式下都能 Scan。xid 读成 `uint32`；interval 返回 KES 自有格式的文本（`+000000002 17:10:03.47`），**必须在 SQL 里转成秒数**（`extract(epoch ...)`）；`''` 读回来是 nil（`ora_input_emptystr_isnull=on`）；`sysdate` 不带时区，pgx 会把它当 UTC，**时间一律用 `now()` 或 timestamptz**；`sys.date` 等非内置 OID 在 SQL 里显式 cast |
| S4  | 只读事务        | 通过  | 开启 `default_transaction_read_only` 后 DDL 被拒（25006），probe 正常。只读事务**拦不住** `pg_terminate_backend`，诊断路径的代码里不出现终止调用                                                                                                                                                                                                     |
| S4b | 锁超时         | 通过  | 会话设 `lock_timeout=500ms`：有 AccessExclusive 在排队时，表 probe 509ms 超时报错（55P03）；`sys_locks` probe 34ms 返回                                                                                                                                                                                                                |
| S5  | 部署          | 通过  | 交叉编译出的二进制拷到 CentOS 7 系 VM 直接运行，不需要额外文件                                                                                                                                                                                                                                                                             |
| S6  | 满连接         | 通过  | 普通用户占满 93 个连接后被 53300 拒绝，`system` 仍能连上                                                                                                                                                                                                                                                                             |

**权限与备库**：

- `kbdiag_ro` 不授监控角色时查看别人的会话：`query` 返回 `<insufficient privilege>`；state、wait_event_type/wait_event、backend_start/xact_start/query_start、client_addr/client_port、backend_type 都返回 NULL。usename、datname、application_name、backend_xid、backend_xmin 可见（1.1 用 kbdiag_ro 复测更正：0.2 曾记成 backend_xid 也被遮蔽，1.2 的 L5 测试已断言它可见）。`sys_locks`、`sys_replication_slots`、`sys_stat_replication`、`sys_prepared_xacts`、`pg_database_size`、`current_setting('data_directory')` 都能正常读。
- `sys_monitor` 存在（`pg_monitor` 也在），可以 GRANT，授予后上面这些列全部可见，`sys_blocking_pids()` 也正常。
- 备库：`sys_replication_slots` 可查；`sys_current_wal_lsn()` **报错**（recovery is progressing），计算 WAL 保留量要按角色改用 `sys_last_wal_replay_lsn()`；主库上的 2PC 事务在备库的 `sys_prepared_xacts` 里**查不到**。
- 可用的函数名：`sys_is_in_recovery`、`sys_postmaster_start_time`、`sys_blocking_pids`、`sys_current_wal_lsn`、`sys_wal_lsn_diff`、`sys_last_wal_replay_lsn`（对应的 `pg_` 版本同样存在）。

## 4. 架构

旧版病根之一是"架构是堆出来的"。新版单向依赖，命令不能绕过：

```
cmd/kbdiag/           入口：cobra 命令定义，只做参数解析和组装，不含业务逻辑
internal/
  conn/               连接：socket/TCP、只读、超时、角色识别
  probe/              采集：每个 probe = 一个 ID + 一条 SQL（或一个 /proc 读取）+ 一个 Go 结构体
                      只负责"把数据拿回来"，不做任何判断
  facts/              采集结果的数据类型（probe 输出、rule 输入）
                      每个 fact 带 status: ok | error | skipped（+原因）
  rule/               判定：纯函数 Facts + Thresholds → []Finding
                      不 import conn/probe，编译层面保证不碰数据库
                      输入 fact 的 status 不是 ok → 必须输出 UNKNOWN/SKIPPED，
                      不允许把"没采到"当成"空结果"判 OK（PRD 原则 3）
  scenario/           场景：声明"这个命令需要哪些 probe、跑哪些 rule"
  report/             输出契约（PRD §5）+ text/json 渲染 + 退出码
  （config/）         暂不建：阈值默认值在 rule.Defaults，只用 flag 覆盖；
                      等阈值多了、或确实需要环境变量覆盖时再建
```

**为什么这样切**：

- `rule` 是纯函数 → 单测只需要构造数据，**不需要 mock 数据库**。mock SQL 返回值测出来的只是"我假设的数据库行为"，恰恰是测试不可信的来源。
- `probe` 只做采集 → 它的正确性只能靠真实数据库验证（集成测试），而且只需验证"SQL 能跑、类型能 Scan、取值正确"，不需要验证业务判断。
- `facts` 可序列化 → 以后低成本实现 PRD F-12 的"录制/回放"，v0.1 不做。
- 依赖方向由 Go 的 `internal/` 包和 import 规则保证，另加一条检查（`rule` 包禁止 import `conn`/`probe`）写进 CI。

## 5. 工程约定

| 项       | 约定                                                                                                                                                                                      |
| ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go 版本   | 当前稳定版，`go.mod` 锁定                                                                                                                                                                       |
| 构建      | Mac 本机 `go build ./cmd/kbdiag`；发布用 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64/arm64`；产物带版本号和 git commit                                                                                     |
| 格式与静态检查 | `gofmt` + `go vet` + `staticcheck`                                                                                                                                                      |
| 目录      | 代码放仓库根（`cmd/`、`internal/`），测试和注入脚本放 `e2e/`                                                                                                                                              |
| 版本号     | 从 `v2.0.0-alpha.1` 起，与旧 shell 版区分                                                                                                                                                       |
| SQL 命名  | 视图一律用 `sys_` 前缀；函数 `sys_`/`pg_` 两套并存时（如 `sys_blocking_pids`/`pg_blocking_pids`）优先 `sys_`；只有 `pg_` 版本的（如 `pg_relation_size`）用 `pg_`。每条 probe 的 SQL 由 L3 在真实实例上验证，引入规则见 `docs/queries.md` |

## 6. 测试分层

每一层测试都要回答：**如果这里有 bug，哪条测试会红？** 每层只测它负责的事：

| 层           | 测什么                                                                                                 | 不测什么    | 在哪跑         | 速度  |
| ----------- | --------------------------------------------------------------------------------------------------- | ------- | ----------- | --- |
| L1 规则单测     | `rule` 包：给定数据 → 应出哪些 finding、什么级别                                                                   | SQL 对不对 | Mac 本地 / CI | 秒级  |
| L2 渲染与契约    | `report` 包：text/json 输出、退出码、stdout/stderr 分离                                                        | 业务判断    | Mac 本地 / CI | 秒级  |
| L3 Probe 契约 | 每条 probe 在真实 KES 上能跑、类型能 Scan、不超时、在备库下行为正确；**并在已知夹具上断言取值**——KES 的语义怪癖（空串即 NULL、interval 格式）只有这一层能抓到 | 判断逻辑    | Lima VM     | 分钟级 |
| L4 场景测试     | 注入真实故障 → 断言 finding 指向被注入的对象                                                                        | —       | Lima VM     | 分钟级 |
| L5 降级矩阵     | 主库 / 备库 / 远程 TCP / `kbdiag_ro` 不授监控角色 / `kbdiag_ro` 授 `sys_monitor`                                 | —       | Lima VM     | 分钟级 |
| L6 用户验收     | 用户本人按验收步骤操作确认                                                                                       | —       | 用户          | —   |

**不使用 mock 数据库驱动**：判定逻辑靠 L1（纯数据），SQL 正确性靠 L3/L4（真数据库），中间没有 mock 层。

### 6.1 L1 规则单测

Go 表驱动测试，一个 rule 一张表。每张表必须包含以下几类用例，缺一类 code review 不放行：

| 类别    | 示例（以连接数规则为例）                                                           |
| ----- | ---------------------------------------------------------------------- |
| 正常/阴性 | 50/100 → 无 finding                                                     |
| 阈值边界  | 69/100、70/100、71/100；89、90、91                                          |
| 零与空   | `max_connections=0`（除零）、活跃会话列表为空                                       |
| 空值    | `application_name` 为 NULL、`query_start` 为 NULL（idle 会话）                |
| 极值    | 10 万个会话（截断提示是否出现）；xid age 接近 2^31                                      |
| 非法值   | 负数时长（时钟回拨）、未知的 `state` 字符串                                             |
| 语义对照  | idle-in-txn 但持排他锁 → **不得**标记为可安全终止                                     |
| 采集失败  | 输入 fact 的 status 为 `error`/`skipped` → 必须输出 UNKNOWN/SKIPPED，**不得**判 OK |

- **Go 原生 fuzz**（`go test -fuzz`）用于解析类函数：版本字符串解析、大小单位换算、facts JSON 反序列化——任意输入不 panic
- **覆盖率门槛**：`rule` 包语句覆盖率 ≥ 90%；覆盖率只是底线，不作为"测够了"的证据

### 6.2 L2 渲染与契约

- **Golden file**：每个 scenario 用一份固定 facts 渲染 text 和 json，与 `testdata/*.golden` 比对；改输出必须显式 `go test -update` 并在 diff 里被审。golden 文件名不属于发布契约
- **JSON 契约测试**：所有命令的 JSON 输出通过同一个 schema 校验；`finding.id` 和 probe_id 列表与已发布清单比对，防止悄悄改名
- **退出码表测试**：verdict → exit code 映射单独一张表测
- **流分离**：断言结果只在 stdout、日志只在 stderr

### 6.3 L3/L4 集成与场景测试

**形态**：Go 测试放在 `e2e/`（build tag `vm`），在 Mac 上运行：交叉编译 kbdiag → 拷到 `KB_TEST_NODE` 指定的 VM → 以 `kingbase` 用户运行真实二进制 → 解析 JSON 输出。测的是用户实际拿到的东西，不是内部函数。故障注入调用 `e2e/inject/*.sh`（§7）。

**每条场景测试的固定结构**：

```
Baseline 注入前先跑一次 kbdiag，记下已有的 finding（VM 上有与测试无关的背景状态，
         如 repmgr 会话、hot_standby_feedback 带来的槽 xmin）
Setup    inject/<名>.sh up，记录"注入指纹"（pid、表名、锁模式、事务 xid）；
         同时放入诱饵
Run      执行 kbdiag <命令> --json
Assert   1. 与基线相比新增了指定 finding.id，且该 finding 的级别正确
            （断言单个 finding 的级别，不断言整体 verdict——它会被背景状态左右）
         2. evidence / data 里包含注入指纹（证明报的是"我制造的那个问题"，不是巧合）
         3. 诱饵不在 evidence 里
Cleanup  inject/<名>.sh down；无论测试成败都执行
```

断言只能引用 PRD §5 登记过的字段；指纹字段缺失时判失败（不能出现零值 == 零值的假绿）。

### 6.4 防"假绿"

| 机制        | 做法                                                                             |
| --------- | ------------------------------------------------------------------------------ |
| **阴性测试**  | 每个 finding.id 配一条：不注入故障，断言该 finding **不出现**                                    |
| **诱饵测试**  | 注入真故障的同时，放一个"看起来像但不该被报"的会话——例如锁场景里放一个不阻塞任何人的 idle-in-txn 会话——断言诱饵不在 evidence 里 |
| **变异抽查**  | 发版前在 `rule` 包里人为改坏一个阈值比较（`>=` 改 `>`）或删掉一条规则，确认至少一条测试变红                         |
| **禁止弱断言** | 场景测试中禁止只断言"exit 0"或"输出包含某字符串"；code review 检查项                                  |

### 6.5 覆盖不到的场景：组合策略

原则：**不假装测过**——每个场景在 `docs/queries.md` 的追溯表里标明用了哪几种手段、哪部分没被真实验证。

| 手段                  | 适用           | 做法                                                                                                     |
| ------------------- | ------------ | ------------------------------------------------------------------------------------------------------ |
| **A. 阈值缩放**         | 条件真实可造，但量级太大 | 通过 flag/环境变量把阈值调低，注入小规模的真实故障。例：冻结年龄不可能真烧 10 亿 xid → 阈值调到 1000                                          |
| **B. Facts 回放**（以后） | 数据库/主机状态难以制造 | facts 文件**只能由 `--dump-facts` 从真实实例录制**，`--from-facts` 离线跑判定和渲染。手写/改写的 facts 标为"合成"，只能记"判定已测"，不能记"采集已测" |
| **C. 文件系统夹具**       | OS 层采集       | `/proc` 的真实快照存进 `testdata/proc/`，采集代码以可替换根目录读取                                                         |
| **D. 半自动破坏性测试**     | 需要停节点、断网     | 独立套件（build tag `destructive`），只在手动触发时跑，自带恢复步骤和前后健康检查                                                   |
| **E. 人工 runbook**   | 自动化成本远大于价值   | 写成验收步骤，由用户执行并记录结果                                                                                      |

同一场景尽量叠加两种以上手段覆盖不同环节；只靠 E 的，追溯表里明确标注"仅人工验证"。

## 7. 故障注入（`e2e/inject/`）

在 Mac 上运行，通过 `limactl shell <node> sudo -iu kingbase` 驱动 VM。用法：

```bash
KB_TEST_NODE=kes-node1 e2e/inject/<名>.sh up|down     # 参数错误 exit 64
```

- 注入的会话都带 `application_name=kbdiag_inj_<tag>`；down 按进程组 + tag 两路清理
- up 一直等到目标状态出现才返回（`wait_for` 按真实秒数计时），测试里不用自己 sleep
- 公共函数在 `lib.sh`（被 source，不直接执行）
- e2e 的 `inject()` 注入前先跑一次 down：上一次测试若在 teardown 前崩溃，残留会话会让指纹不唯一

| 脚本              | 造出的状态                                            | up 耗时                        |
| --------------- | ------------------------------------------------ | ---------------------------- |
| `lock.sh`       | 主库：holder 持 AccessExclusive，waiter 等 AccessShare 被挡；备库：两个会话抢 advisory lock 424242 | ~1s |
| `idle_txn.sh`   | idle in transaction；主库上带 backend_xid（备库分配不了 xid，只开事务）；四个时间戳各隔约 1 秒 | ~4s |
| `long_query.sh` | `pg_sleep(3600)` 长查询                             | ~1s                          |
| `prepared.sh`   | 未结束的 2PC 事务 `kbdiag_inj_2pc`                     | ~1s                          |
| `prepared_waiter.sh` | 一个会话等 `kbdiag_inj_2pc` 上的锁，挡它的是 prepared 事务（blocker pid 0）；要先起 `prepared.sh`，仅主库 | ~1s |
| `untracked.sh`  | 会话自己 `set track_activities=off` 后开事务，别人看到 state=`disabled`（模拟 ALTER ROLE ... SET） | ~1s |
| `track_off.sh`  | `track_activities=off`（ALTER SYSTEM + reload）    | ~1s                          |
| `slot.sh`       | 在备库上 SIGSTOP walreceiver，主库槽变 inactive 且保留 xmin  | ~31s（等 `wal_sender_timeout`） |
| `standby_slot.sh` | 在备库上建一个预留 WAL 的物理槽 `kbdiag_inj_slot`（inactive），仅备库 | ~1s |

**slot 为什么这样造**：wal_level=replica 建不了逻辑槽；node2 上 kbha 守护进程加 cron 每分钟会拉起停掉的实例；断网可能触发 repmgr 故障切换。暂停 walreceiver 不停实例、不断网，形态和"备库挂了"一致。

## 8. 运行方式

| 命令                                                  | 内容                                   | 何时跑       |
| --------------------------------------------------- | ------------------------------------ | --------- |
| `go vet ./... && go test ./...`                     | L1 + L2                              | 每次提交前；CI  |
| `KB_TEST_NODE=kes-node1 go test -tags vm ./e2e/...` | L3 + L4 + L5（主库视角）                   | 每个功能切片完成时 |
| `KB_TEST_NODE=kes-node2 go test -tags vm ./e2e/...` | 同上（备库视角）                             | 同上        |
| GitHub Actions                                      | 只跑 L1 + L2 和文档数检查（CI 里没有 KingbaseES） | push 时    |

**Mac 不能休眠**：Mac 一休眠 VM 就跟着挂起，醒来时 VM 时钟前跳，`wait_for` 提前超时、`now()-state_change` 失真，测试会随机失败（2026-09-24 实测）。无人值守跑时用 `caffeinate -dimsu go test ...` 包起来。

**诚实声明**：L3~L5 依赖本地 Lima VM，不在 CI 里——"CI 绿"不等于"集成测试过"，发版记录必须附 VM 实跑结果。

## 9. 验收（L6）

每个版本发布前产出验收步骤（每个场景：怎么制造 → 敲什么命令 → 应该看到什么），**由用户本人在 VM 上照着走一遍**，不以自动化测试结果代替验收。
