# kbdiag 2.0 技术选型

**状态**：pending approval
**日期**：2026-09-23
**结论先行**：**Go + pgx/v5**，`CGO_ENABLED=0` 交叉编译成单个静态二进制。放行前先做一次 1 天以内的技术验证（§4），验证不过再回来重选。

---

## 1. 硬约束（来自 PRD，选型只能在这里面挑）

| 约束                                         | 来源                       |
| ------------------------------------------ | ------------------------ |
| C1 单一产物，目标机不装运行时                           | PRD N-01，用户确认            |
| C2 直连数据库线协议，不调 `ksql` 子进程                  | 用户确认                     |
| C3 在 Mac 上开发，交叉编译到 linux/amd64、linux/arm64 | 现有开发环境                   |
| C4 判定逻辑能脱离数据库做单测                           | PRD N-05，旧版病根            |
| C5 代码主要由 AI 写、由 DBA 背景的用户审                 | 用户说过"我已经审核不了"——代码必须朴素、好读 |

C5 是最容易被忽略、但对这个项目最重要的一条：**选型要优化"审得动"，而不是"写得炫"**。

## 2. 候选对比

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

## 3. 依赖清单（按复杂度惩罚原则，每个依赖对应一条当下需求）

| 依赖                        | 用途                | 对应需求           | 不用它的代价                                     |
| ------------------------- | ----------------- | -------------- | ------------------------------------------ |
| `github.com/jackc/pgx/v5` | 数据库线协议、SCRAM 认证   | C2             | 自己实现协议，不现实                                 |
| `github.com/spf13/cobra`  | 两级子命令、自动生成英文 help | PRD §4 场景族+子命令 | 用标准库 `flag` 手写 dispatch + help，约 200 行重复代码 |
| 测试：只用标准库 `testing`        | —                 | —              | —                                          |

**不引入**：ORM、配置文件库（配置只有 flag + `KB_*` 环境变量，标准库足够）、日志框架（`log/slog` 标准库）、断言库（testify 等，标准库 + `go-cmp` 按需再议）、mock 框架（架构上不需要，见 §5）。

## 4. 放行前技术验证（Spike，≤1 天，结果写回本文档）

VM 实测（2026-09-23，kes-node1）已发现一个真实风险：

| #   | 验证项          | 已知事实                                                                                                                             | 风险  | 通过标准                                                                                                                                                                                                                                                                     |
| --- | ------------ | -------------------------------------------------------------------------------------------------------------------------------- | --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| S1  | 本地 socket 连接 | socket 文件名是 `/tmp/.s.KINGBASE.54321`，**不是** PG 的 `.s.PGSQL.54321`；pgx 默认按 PG 命名拼路径，会连不上                                          | 高   | 通过 pgx 的自定义 `DialFunc` 直连该路径，`local trust` 免密成功                                                                                                                                                                                                                          |
| S2  | TCP + SCRAM  | `password_encryption = scram-sha-256`，hba 远程为 scram                                                                              | 低   | 127.0.0.1 用密码连通                                                                                                                                                                                                                                                          |
| S3  | 类型与语义        | VM 实测：`database_mode=oracle`、`ora_input_emptystr_isnull=on`（`''` 当 NULL）、`IntervalStyle=kingbase`、`sys.date` OID=8020（非 PG 内置类型） | 中   | 10 条代表性 probe（`sys_stat_activity`、`sys_locks`、`sys_stat_replication`、`age(datfrozenxid)`、`pg_relation_size`）全部能 Scan；逐条确认：空串列读回 NULL 时 Go 侧处理正确；interval 一律在 SQL 里转成秒数（`extract(epoch ...)`）再返回，不在 Go 里解析 KES 的 interval 文本；遇到 `sys.date` 等非内置 OID 时在 SQL 里显式 cast 成标准类型 |
| S4b | 锁超时          | VM 默认 `lock_timeout=0`                                                                                                           | 高   | 会话设 `lock_timeout=500ms`：有 AccessExclusive 排队时，需要表锁的 probe 在 500ms 内失败并进 `skipped`，锁族 probe（只读 `sys_locks`，不拿表锁）照常出结果                                                                                                                                                    |
| S4  | 只读事务         | —                                                                                                                                | 低   | `SET default_transaction_read_only=on` 后 probe 正常、DML/DDL 被拒（注意：只读事务**拦不住** `pg_terminate_backend`，所以 `kill` 的防护必须靠代码层——诊断路径的代码里不出现终止调用，而不是靠只读事务）                                                                                                                        |
| S5  | 部署           | VM 是 CentOS 7 系（GCC 4.8.5 编译）                                                                                                    | 低   | Mac 上交叉编译的二进制 scp 到 VM 直接运行，`kingbase` 用户无需任何额外文件                                                                                                                                                                                                                        |
| S6  | 满连接          | `max_connections=100`，`superuser_reserved_connections=3`                                                                         | 中   | 普通连接占满后，`system` 用户仍能连上                                                                                                                                                                                                                                                  |

**任何一项不过**：S1/S3 不过且无法绕过 → 回到本文档重新评估（备选 Rust + tokio-postgres，同样纯协议实现，但 S1 问题两者一样，所以大概率是驱动层可绕过的问题）。

## 5. 架构（选型的一部分：Go 让这个结构写起来最自然）

旧版病根之一是"架构是堆出来的"。新版只有三段，单向依赖，命令不能绕过：

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
  config/             阈值：唯一来源，flag > 环境变量 > 默认值
```

**为什么这样切**：

- `rule` 是纯函数 → 单测只需要构造数据，**不需要 mock 数据库**。旧方案里"mock 驱动做单测"是错的方向：mock SQL 返回值测出来的只是"我假设的数据库行为"，恰恰是测试不可信的来源。
- `probe` 只做采集 → 它的正确性只能靠真实数据库验证（集成测试），而且只需验证"SQL 能跑、类型能 Scan"，不需要验证业务判断。
- `facts` 可序列化 → v0.2 时低成本实现 PRD F-12 的"录制/回放"，MVP 不做。
- 依赖方向由 Go 的 `internal/` 包和 import 规则保证，加一条 `go vet` 之外的检查（`rule` 包禁止 import `conn`/`probe`）写进 CI。

## 6. 工程约定

| 项       | 约定                                                                                                                                                                                                                  |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go 版本   | 当前稳定版，`go.mod` 锁定                                                                                                                                                                                                   |
| 构建      | `make build`（Mac 本机）/ `make dist`（linux amd64+arm64）；产物带版本号和 git commit                                                                                                                                             |
| 格式与静态检查 | `gofmt` + `go vet` + `staticcheck`，pre-commit 执行                                                                                                                                                                    |
| 目录      | 新代码放仓库根（`cmd/`、`internal/`），旧 shell 在 tag `shell-final` 后删除（PRD Q3）                                                                                                                                                 |
| 版本号     | 从 `v2.0.0-alpha.1` 起，与旧 shell 版区分                                                                                                                                                                                   |
| SQL 命名  | 视图一律用 `sys_` 前缀；函数 `sys_`/`pg_` 两套并存时（VM 实测 `sys_blocking_pids`/`pg_blocking_pids`、`sys_wal_replay_pause`/`pg_wal_replay_pause` 都有）优先 `sys_`；只有 `pg_` 版本的（如 `pg_relation_size`）用 `pg_`。每条 probe 的 SQL 由 L3 在真实实例上验证 |
