# kbdiag 2.0 需求说明书（PRD）

状态: active | 最后核对: 2026-09-26

**职责**：需求、范围、输出契约、版本目标、DS 场景表。查询条目和它们属于哪个版本以 `docs/queries.md` 为准；选型、架构、测试以 `docs/engineering.md` 为准。

---

## 1. 为什么重写

旧版（Shell，36 个顶层命令）失败在两件事上，新版每条设计都要能回答"它怎么避免重蹈覆辙"：

| 旧版病根 | 表现（有代码证据） | 新版对策 |
|---|---|---|
| **测试不可信** | 测试只断言"exit 0"或"输出含某中文标签"；`test_locks_hold_shows_locks` 从未真正持锁 | 场景测试必须"注入故障 → 断言结构化证据指向被注入的那个对象"；且去掉注入后测试必须失败（见 `docs/engineering.md`） |
| **架构是堆出来的** | 同一场景多入口（`perf bloat`/`idx bloat`/`space frag`）；JSON 全局缓冲区不能嵌套；两套 exit-code 约定；`warn()` 在文本模式写 stdout | 先定统一的"采集→判定→渲染"三段架构和唯一的输出契约，命令只是场景的组合，不允许命令自己拼输出 |

**一句话定位**：kbdiag 是 DBA 在 KingbaseES 出事时敲的第一条命令——它回答"现在哪里不对、证据是什么、下一步查什么/做什么"，并且在它不知道的时候明确说"不知道"。

## 2. 用户与使用环境

- **主要用户**：值班 DBA / 运维（熟悉 KingbaseES，不想记系统视图）
- **运行位置**：
  - 默认：在数据库主机上以 `kingbase` OS 用户运行，走本地 Unix socket 免密
  - 也支持：从跳板机用 host/port/user/password 远程连（此时 OS 层指标不可用，必须明示而不是静默跳过）
- **部署约束**：单一可执行文件，目标机不装任何运行时/依赖，拷过去就能跑；不假设 sudo
- **集群形态**：单机、repmgr 主备；备库上运行必须可用（只读、无 `sys_stat_statements` 写入等）
- **目标版本**：KingbaseES V8R6（测试环境实测 `V008R006C009B0014`，x86_64）。其他版本（V8R3、V9）是否支持见 §9 开放问题

## 3. 产品原则（所有设计决策的裁判标准）

1. **场景为王**：每个命令必须能追溯到至少一个 DS 场景。对不上场景的功能不做——不为"看起来全"而加命令。
2. **证据链而不是结论**：每条 finding 带 `症状 → 证据（原始数值+来源）→ 根因 → 下一步（验证命令 / 处置 SQL）`。
3. **不假装 OK**：查询失败、权限不足、超时，结果是 `UNKNOWN`/`skipped` 并写明原因，绝不输出 OK；只查到一部分（列被遮蔽）要在 `redacted` 里写明。
   - **不适用不等于没查到**：功能在当前角色或环境下没有意义（如备库上的 2PC），标 `not_applicable` 并指出该去哪查；它不参与 verdict，也不同于 `skipped`。
4. **默认只读**：所有诊断在只读事务里执行；唯一的写操作（终止会话）是单独命令、需显式参数。
5. **一个阈值一个来源**：同一指标在所有命令里用同一个阈值配置（修 GAP-6）。
6. **对自身影响可控**：只占 1 个连接；每条 SQL 有语句超时；不跑全表扫描级的重查询，重查询必须显式开关。

## 4. 命令模型

**v0.1 用扁平命令**（2026-09-23 定）：`kbdiag <命令> [flags]`，如 `sessions`、`locks`、`txn`。v0.1 只有 7 条单次查询命令，两级分组还用不上。下面的"场景族 + 问题"两级模型是草案，v0.2 起再评估要不要改成它。

**v0.1 命令清单**（条目以 `docs/queries.md` 的"版本"列为准，这里写各命令的边界）：

| 命令 | 回答的问题 | probe_id | 备库上的行为 |
|---|---|---|---|
| `sessions`（`--all` 列出全部会话，含 idle 和后台进程；只影响文本，JSON 总是全部会话） | 连接是谁占的（按用户/库/应用/客户端汇总），谁在干活、干了多久，谁 idle in txn | `session.activity` | 正常 |
| `session <pid>` | 这个会话在跑什么 SQL、在等什么、持有哪些锁、被谁挡住 | `session.activity`、`lock.list` | 正常 |
| `locks` | 谁挡的人最多、它持有什么锁；谁在等锁、直接被谁挡住（一层）、等了多久。文本先列挡路者（按挡住的会话数排），再列等锁的会话（等得最久的在前）；JSON 不变 | `lock.list` | 正常 |
| `txn` | 长事务、idle in txn、最老的 backend_xmin、未结束的 2PC | `session.activity`、`txn.prepared` | 2PC 部分 `not_applicable`，提示去主库查 |
| `waits` | 此刻各会话在等什么（汇总） | `wait.summary` | 正常 |
| `status` | 刚登上实例时的基本盘：身份（短版本号、数据目录、端口）、角色和复制（主库列出每个备库，备库看上游在不在收 WAL）、启动时间、连接数/可用数、各库大小、数据目录所在磁盘。只判两条：普通用户已经连不上（`inst.connections` FAIL），备库没在收 WAL（`inst.upstream` WARN）；没有参数 | `inst.info`、`inst.downstreams`、`inst.upstream`、`inst.databases`、`inst.disk` | 主库上 `inst.upstream` 为 `not_applicable`；远程运行时 `inst.disk` 为 `not_applicable` |
| `slots` | 复制槽是否活跃、保留多少 WAL、xmin 是否压着视界 | `slot.list` | 正常；WAL 保留量改用 `sys_last_wal_replay_lsn()` 计算 |

开关感知：`sessions` 依赖 `track_activities`，关着时 probe 标 `skipped` 并写明开关名，而不是给出空的 SQL 文本。`track_activity_query_size` 只决定 SQL 文本截断到多长，不是开关，不影响 status。

草案：`kbdiag <场景族> [<具体问题>] [flags]`。场景族按 DBA 出事时的第一反应划分，而不是按实现模块划分。

三层深度（看/查/断）保留为**概念**，但不体现在命令分组上：
- **看**：`kbdiag check`（巡检，各族的健康摘要，一屏给答案）
- **查**：`kbdiag <族> <问题>`（单维度深查，可独立使用，也是"断"的验证路径）
- **断**：`kbdiag diagnose`（跨族关联，输出完整证据链）

### 4.1 场景族草案（与 DS 场景的映射）

| 场景族 | 问题（子命令） | 回答的问题 | 覆盖场景 |
|---|---|---|---|
| `conn` | `summary` / `idle-txn` | 连接数快满了吗？谁占的？谁开了事务不提交？ | DS-01~04 |
| `lock` | `chain` / `ddl` / `deadlock` | 谁堵了谁（完整多级链）？DDL 被谁卡住？最近有没有死锁？ | DS-05~09（修 GAP-1/2/3） |
| `sql` | `slow` / `top` / `temp` | 现在在跑的慢 SQL？历史 Top SQL？谁在写临时文件？ | DS-10/11/15（修 GAP-4） |
| `wait` | `summary` | 实例整体在等什么（IO/锁/LWLock/CPU）？ | DS-12/13/14 |
| `host` | `load` | 主机 CPU/内存/磁盘 IO 现在忙不忙（仅本机运行时） | DS-13/14（修 GAP-8） |
| `vacuum` | `bloat` / `stalled` / `freeze` | 哪些表/索引膨胀？autovacuum 为什么没清？冻结还有多少余量？ | DS-17~19 |
| `wal` | `growth` | WAL 为什么在涨（归档失败/复制槽/大事务）？ | DS-16/22 |
| `ha` | `repl` / `ready` | 复制延迟多少？备库断了是崩溃还是网络？能不能安全切换？ | DS-20~24（修 GAP-7） |
| `check` | — | 巡检：各族健康摘要 | DS-01/02/03/12/19/20 |
| `diagnose` | — | 断：跨族关联出根因链（如"膨胀 ← autovacuum 停滞 ← 长事务 pid X"） | 全部 |
| `kill`（v0.2） | `--pid` / `--idle-txn` | 唯一写操作：终止会话（带安全检查） | DS-03/04 |

**命名规则**：场景族是名词、子命令是问题；族内若只有一个问题，允许省略子命令。
**准入规则**：新增族或子命令必须在本表加一行并写明覆盖的 DS 编号；新增 DS 场景要先写进 §11 的 DS 表。

## 5. 输出契约（全命令唯一）

每个命令产出同一个结构，文本和 `--json` 只是它的两种渲染：

```text
Report
├── command
├── verdict: OK | WARN | FAIL | UNKNOWN
├── context: version、role(primary|standby)、location(local|remote)、user、collected_at
├── data: {<probe_id>: {status, reason, columns[], rows[][], truncated}}
├── findings[]
│   ├── id           稳定标识，如 lock.waiting（测试和文档靠它引用）
│   ├── level        OK | WARN | FAIL
│   ├── symptom      一句话症状
│   ├── evidence[]   {probe_id, fields: 原始值}，可机读
│   ├── cause        根因判断（可空：看/查层只报事实）
│   └── next[]       {kind: verify|fix, command 或 sql, note}
└── redacted[]       {probe_id, field, reason, rows_affected}
```

**data（命令的主体）**
- 列表命令的主体放在 `data` 里；findings 只放被阈值标出的行，不重复整张表。
- 键是 probe_id，格式 `<域>.<对象>`（如 `session.activity`、`lock.list`、`txn.prepared`、`slot.list`、`inst.info`、`inst.downstreams`），和 finding.id 共用域前缀，不带命令名。每个 probe_id 登记在 `docs/queries.md` 的追溯表里。
- 一个 probe 就是一条 SQL（唯一的例外是 `inst.disk`，它对数据目录做 statfs，只在本机运行时有）；**列名属于契约**，同一个 probe_id 在所有命令里列相同，命令只决定过滤哪些行。
- `rows` 是和 `columns` 对齐的数组；时长一律是秒（`*_s`），大小一律是字节（`*_bytes`），时间是带时区的 ISO 8601。
- `truncated` 是没显示的行数（修 GAP-4），0 表示没截断。
- `status`：
  - `ok`：采到了，`rows` 可以为空，表示"真的没有"。
  - `skipped`：想查但没查到（权限、超时、依赖的开关没开），`reason` 写明原因和具体开关。
  - `error`：SQL 报错，`reason` 带错误码。
  - `not_applicable`：这个 probe 在当前角色或环境下没有意义（如备库上查 2PC），`reason` 写明该去哪查。它**不参与 verdict**，也不同于 `skipped`。
- `status` 不是 `ok` 时，`rows` 为空数组，不能拿来判"没有问题"。

**redacted（部分降级）**：权限不足时 KES 不报错，而是把别人会话的部分列返回成 NULL 或 `<insufficient privilege>`（实测见 `docs/engineering.md` §3）。每个被遮蔽的列记一条：`probe_id`、`field`、`reason`、`rows_affected`（受影响的行数；聚合类 probe 按会话数计）。`reason` 目前有两种：`insufficient_privilege`（权限不足被遮蔽）；`track_activities_off`（那个会话自己关了活动跟踪，例如 `ALTER ROLE ... SET`，state 显示 `disabled`，而 kbdiag 自己的连接是开着的）。这两种行都不参与阈值判断；没有别的 WARN/FAIL 时 verdict 是 UNKNOWN。

**verdict 规则**
- 取 findings 里最高的 level（FAIL > WARN > OK）；没有 finding 就是 OK。
- 没有 WARN/FAIL，但命令依赖的 probe 是 `skipped`/`error`，或判定依赖的列被 `redacted`，verdict 为 UNKNOWN——没看全就不说 OK（原则 3）。
- `not_applicable` 不参与计算。
- 阈值默认值来自 `docs/queries.md` 的阈值表，代码里只有一处默认值（`rule.Defaults`），可以用 flag 覆盖（原则 5）。

**其他约定**
- **evidence 字段名属于契约**：每个 finding.id 的 evidence 字段在 finding 清单里登记，纳入 JSON schema；场景测试的断言只能依赖登记过的字段
- **退出码**（全命令统一，不允许命令自定义；沿用 Nagios 约定便于接监控）：`0` OK，`1` WARN，`2` FAIL，`3` UNKNOWN（没能下结论），`64` 用法错误（需覆盖 cobra 默认的 1，避免与 WARN 撞码），`69` 连不上数据库
- **role**：只表示恢复角色，取自 `sys_is_in_recovery()`；不读 repmgr，不判断 standalone。集群拓扑（`topology`）v0.2 以后再做
- **stdout/stderr**：结果只进 stdout；日志、进度、调试信息只进 stderr
- **JSON 稳定性**：字段只增不改；finding.id 和 probe_id 一经发布不改名
- **帮助文本**：英文（沿用旧约定）；finding 文案中文/英文由 `--lang` 决定，默认中文（待确认，见 §9）

### 5.1 v0.1 各命令的 JSON 示例

示例按测试 VM 上的注入场景（`e2e/inject/`）构造，用来固定字段形状；pid、大小、路径等数值是示意，实际输出以 L2 golden 为准。

#### 示例：sessions

主库上有一个 idle in transaction 会话（`idle_txn` 注入）。JSON 的 `session.activity` 总是全部会话（后台进程、idle 会话都在），只受 `--limit` 影响；文本默认只给汇总和不是 idle 的客户端会话，`--all` 才列全部（2026-09-26 起）。

```json
{
  "command": "sessions",
  "verdict": "WARN",
  "context": {"version": "KingbaseES V008R006C009B0014", "role": "primary", "location": "local", "user": "system", "collected_at": "2026-09-23T21:40:05+08:00"},
  "data": {
    "session.activity": {
      "status": "ok",
      "reason": null,
      "columns": ["pid", "usename", "datname", "application_name", "client_addr", "backend_type", "state", "backend_xid", "backend_xmin", "xact_age_s", "query_age_s", "state_age_s", "wait_event_type", "wait_event", "query"],
      "rows": [
        [236201, "system", "test", "kbdiag_inj_idle_txn", null, "client backend", "idle in transaction", 5855, 5855, 1830.4, 1830.4, 1830.4, "Client", "ClientRead", "select txid_current();"],
        [236188, "system", "test", "ksql", "192.168.105.1", "client backend", "active", null, 5855, 0.2, 0.2, 0.2, null, null, "select * from orders where id = 42"]
      ],
      "truncated": 0
    }
  },
  "findings": [
    {
      "id": "session.idle_in_txn",
      "level": "WARN",
      "symptom": "会话 236201 处于 idle in transaction 已 1830 秒",
      "evidence": [{"probe_id": "session.activity", "fields": {"pid": 236201, "state": "idle in transaction", "state_age_s": 1830.4, "backend_xid": 5855}}],
      "cause": null,
      "next": [{"kind": "verify", "command": "kbdiag session 236201", "note": "看它持有哪些锁、有没有挡住别人"}]
    }
  ],
  "redacted": []
}
```

#### 示例：session

`session 236155`：一个被挡住的会话（`lock` 注入的 waiter）。

```json
{
  "command": "session",
  "verdict": "WARN",
  "context": {"version": "KingbaseES V008R006C009B0014", "role": "primary", "location": "local", "user": "system", "collected_at": "2026-09-23T21:41:10+08:00"},
  "data": {
    "session.activity": {
      "status": "ok",
      "reason": null,
      "columns": ["pid", "usename", "datname", "application_name", "client_addr", "backend_type", "state", "backend_xid", "backend_xmin", "xact_age_s", "query_age_s", "state_age_s", "wait_event_type", "wait_event", "query"],
      "rows": [
        [236155, "system", "test", "kbdiag_inj_lock_waiter", null, "client backend", "active", null, 5860, 42.0, 42.0, 42.0, "Lock", "relation", "select count(*) from kbdiag_inj_lock;"]
      ],
      "truncated": 0
    },
    "lock.list": {
      "status": "ok",
      "reason": null,
      "columns": ["pid", "locktype", "relation", "mode", "granted", "wait_s", "blocked_by"],
      "rows": [
        [236155, "relation", "public.kbdiag_inj_lock", "AccessShareLock", false, 42.0, [236153]]
      ],
      "truncated": 0
    }
  },
  "findings": [
    {
      "id": "lock.waiting",
      "level": "WARN",
      "symptom": "会话 236155 等 public.kbdiag_inj_lock 的 AccessShareLock 已 42 秒，被 236153 挡住",
      "evidence": [{"probe_id": "lock.list", "fields": {"waiter_pid": 236155, "blocker_pids": [236153], "relation": "public.kbdiag_inj_lock", "lock_mode": "AccessShareLock", "wait_s": 42.0}}],
      "cause": null,
      "next": [{"kind": "verify", "command": "kbdiag session 236153", "note": "看挡路的会话在干什么"}]
    }
  ],
  "redacted": []
}
```

#### 示例：locks

`lock` 注入：holder 持 AccessExclusiveLock，waiter 在等。只显示一层直接阻塞者，多级链是 v0.2 以后的 `locks --tree`。每个等锁超过阈值的会话各出一条 `lock.waiting`；表里只列等锁的行，以及挡路会话在同一对象上持有的锁。

```json
{
  "command": "locks",
  "verdict": "WARN",
  "context": {"version": "KingbaseES V008R006C009B0014", "role": "primary", "location": "local", "user": "system", "collected_at": "2026-09-23T21:41:12+08:00"},
  "data": {
    "lock.list": {
      "status": "ok",
      "reason": null,
      "columns": ["pid", "locktype", "relation", "mode", "granted", "wait_s", "blocked_by"],
      "rows": [
        [236153, "relation", "public.kbdiag_inj_lock", "AccessExclusiveLock", true, null, []],
        [236155, "relation", "public.kbdiag_inj_lock", "AccessShareLock", false, 44.0, [236153]]
      ],
      "truncated": 0
    }
  },
  "findings": [
    {
      "id": "lock.waiting",
      "level": "WARN",
      "symptom": "会话 236155 等 public.kbdiag_inj_lock 的 AccessShareLock 已 44 秒，被 236153 挡住",
      "evidence": [{"probe_id": "lock.list", "fields": {"waiter_pid": 236155, "blocker_pids": [236153], "relation": "public.kbdiag_inj_lock", "lock_mode": "AccessShareLock", "wait_s": 44.0}}],
      "cause": null,
      "next": [{"kind": "verify", "command": "kbdiag session 236153", "note": "看挡路的会话在干什么"}]
    }
  ],
  "redacted": []
}
```

#### 示例：txn

在备库上运行：长事务部分正常；2PC 部分 `not_applicable`（实测备库看不到主库的 2PC）。

```json
{
  "command": "txn",
  "verdict": "OK",
  "context": {"version": "KingbaseES V008R006C009B0014", "role": "standby", "location": "local", "user": "system", "collected_at": "2026-09-23T21:45:00+08:00"},
  "data": {
    "session.activity": {
      "status": "ok",
      "reason": null,
      "columns": ["pid", "usename", "datname", "application_name", "client_addr", "backend_type", "state", "backend_xid", "backend_xmin", "xact_age_s", "query_age_s", "state_age_s", "wait_event_type", "wait_event", "query"],
      "rows": [],
      "truncated": 0
    },
    "txn.prepared": {
      "status": "not_applicable",
      "reason": "备库看不到主库的两阶段提交事务，请在主库上运行 kbdiag txn",
      "columns": ["gid", "owner", "database", "prepared_at", "age_s", "transaction"],
      "rows": [],
      "truncated": 0
    }
  },
  "findings": [],
  "redacted": []
}
```

#### 示例：waits

用不授监控角色的 `kbdiag_ro` 连接：别人会话的等待事件被遮蔽，没法判断，所以 verdict 是 UNKNOWN。

```json
{
  "command": "waits",
  "verdict": "UNKNOWN",
  "context": {"version": "KingbaseES V008R006C009B0014", "role": "primary", "location": "remote", "user": "kbdiag_ro", "collected_at": "2026-09-23T21:47:30+08:00"},
  "data": {
    "wait.summary": {
      "status": "ok",
      "reason": null,
      "columns": ["wait_event_type", "wait_event", "state", "sessions", "pids"],
      "rows": [
        [null, null, null, 7, [3120, 3121, 3125, 236153, 236155, 236188, 236201]],
        [null, null, "active", 1, [237010]]
      ],
      "truncated": 0
    }
  },
  "findings": [],
  "redacted": [
    {"probe_id": "wait.summary", "field": "wait_event_type", "reason": "insufficient_privilege", "rows_affected": 7},
    {"probe_id": "wait.summary", "field": "wait_event", "reason": "insufficient_privilege", "rows_affected": 7},
    {"probe_id": "wait.summary", "field": "state", "reason": "insufficient_privilege", "rows_affected": 7}
  ]
}
```

#### 示例：status

node1（主库），2026-09-26 18:28 实采（chronicle/2026-09-26.md）。`inst.info.version` 是短版本号，完整的 `version()` 不再输出；`usable_connections` = `max_connections - superuser_reserved_connections`；`inst.downstreams` 每个备库一行，`sync_state` 保留原值（repmgr 下是 `quorum`）；`inst.disk` 的 `used_bytes + avail_bytes` 小于 `total_bytes`，差值是给 root 保留的块，和 df 一致。文本输出另有排版（大小、时长换成人能读的单位），JSON 保留字节和秒。

```json
{
  "command": "status",
  "verdict": "OK",
  "context": {"version": "KingbaseES V008R006C009B0014", "role": "primary", "location": "local", "user": "system", "collected_at": "2026-09-26T18:28:09+08:00"},
  "data": {
    "inst.info": {
      "status": "ok",
      "reason": null,
      "columns": ["version", "start_time", "uptime_s", "connections", "max_connections", "superuser_reserved_connections", "data_directory", "port", "usable_connections"],
      "rows": [
        ["V008R006C009B0014", "2026-09-20T22:19:13+08:00", 504535.0, 6, 100, 3, "/home/kingbase/cluster/install/kingbase/data", 54321, 97]
      ],
      "truncated": 0
    },
    "inst.downstreams": {
      "status": "ok",
      "reason": null,
      "columns": ["application_name", "client_addr", "state", "sync_state"],
      "rows": [["node2", "192.168.105.11", "streaming", "quorum"]],
      "truncated": 0
    },
    "inst.upstream": {
      "status": "not_applicable",
      "reason": "primary",
      "columns": ["status", "sender_host", "sender_port", "slot_name", "last_msg_age_s"],
      "rows": [],
      "truncated": 0
    },
    "inst.databases": {
      "status": "ok",
      "reason": null,
      "columns": ["datname", "size_bytes"],
      "rows": [["esrep", 15614003], ["kingbase", 15024179], ["mydb", 14877187], ["security", 14844419], ["test", 340459571]],
      "truncated": 0
    },
    "inst.disk": {
      "status": "ok",
      "reason": null,
      "columns": ["total_bytes", "used_bytes", "avail_bytes"],
      "rows": [[213452304384, 15089946624, 198362357760]],
      "truncated": 0
    }
  },
  "findings": [],
  "redacted": []
}
```

#### 示例：status（备库）

node2（备库），同一次采集。`inst.upstream` 没有 repmgr 节点名，只有上游地址；`last_msg_age_s` 只展示不判定（空闲时主库每 `wal_receiver_status_interval` 发一次，8 秒是正常值）。

```json
{
  "command": "status",
  "verdict": "OK",
  "context": {"version": "KingbaseES V008R006C009B0014", "role": "standby", "location": "local", "user": "system", "collected_at": "2026-09-26T18:28:10+08:00"},
  "data": {
    "inst.info": {
      "status": "ok",
      "reason": null,
      "columns": ["version", "start_time", "uptime_s", "connections", "max_connections", "superuser_reserved_connections", "data_directory", "port", "usable_connections"],
      "rows": [
        ["V008R006C009B0014", "2026-09-23T15:44:58+08:00", 268991.0, 3, 100, 3, "/home/kingbase/cluster/install/kingbase/data", 54321, 97]
      ],
      "truncated": 0
    },
    "inst.upstream": {
      "status": "ok",
      "reason": null,
      "columns": ["status", "sender_host", "sender_port", "slot_name", "last_msg_age_s"],
      "rows": [["streaming", "192.168.105.10", 54321, "repmgr_slot_2", 8.0]],
      "truncated": 0
    },
    "inst.downstreams": {
      "status": "ok",
      "reason": null,
      "columns": ["application_name", "client_addr", "state", "sync_state"],
      "rows": [],
      "truncated": 0
    },
    "inst.databases": {
      "status": "ok",
      "reason": null,
      "columns": ["datname", "size_bytes"],
      "rows": [["esrep", 15614003], ["kingbase", 15024179], ["mydb", 14877187], ["security", 14844419], ["test", 340459571]],
      "truncated": 0
    },
    "inst.disk": {
      "status": "ok",
      "reason": null,
      "columns": ["total_bytes", "used_bytes", "avail_bytes"],
      "rows": [[213452304384, 12501807104, 200950497280]],
      "truncated": 0
    }
  },
  "findings": [],
  "redacted": []
}
```

备库不收 WAL 时（没有接收进程），findings 为：

```text
{"id": "inst.upstream", "level": "WARN",
 "symptom": "备库没有 WAL 接收进程，没在从主库收 WAL；主库这时挂掉，没有能接管的备库",
 "evidence": [{"probe_id": "inst.upstream", "fields": {"status": null}}],
 "cause": null,
 "next": [{"kind": "verify", "command": "kbdiag slots", "note": "到主库上跑，看这个备库的槽是不是 inactive"}]}
```

有接收进程但状态不是 `streaming` 时，symptom 写出状态，evidence 字段为 `status`、`sender_host`、`sender_port`、`slot_name`。`inst.connections`（FAIL）的 evidence 字段为 `connections`、`max_connections`、`superuser_reserved_connections`。

#### 示例：slots

`slot` 注入：备库 walreceiver 被暂停后，主库上的物理槽变成非活跃，而且 xmin 还压着视界。

```json
{
  "command": "slots",
  "verdict": "FAIL",
  "context": {"version": "KingbaseES V008R006C009B0014", "role": "primary", "location": "local", "user": "system", "collected_at": "2026-09-23T21:52:40+08:00"},
  "data": {
    "slot.list": {
      "status": "ok",
      "reason": null,
      "columns": ["slot_name", "slot_type", "active", "active_pid", "xmin", "catalog_xmin", "xmin_age", "restart_lsn", "retained_wal_bytes"],
      "rows": [
        ["repmgr_slot_2", "physical", false, null, 5859, null, 14, "0/A4013160", 50331648]
      ],
      "truncated": 0
    }
  },
  "findings": [
    {
      "id": "slot.inactive",
      "level": "FAIL",
      "symptom": "复制槽 repmgr_slot_2 未激活，保留 48 MB WAL，xmin 5859 压着视界",
      "evidence": [{"probe_id": "slot.list", "fields": {"slot_name": "repmgr_slot_2", "active": false, "xmin": 5859, "retained_wal_bytes": 50331648}}],
      "cause": null,
      "next": [{"kind": "verify", "command": "kbdiag status", "note": "在备库上运行：连不上说明备库实例挂了；inst.upstream 没有接收进程或不是 streaming 说明没在收 WAL；显示 streaming 时隔十几秒再跑一次，last_msg 还在涨说明接收进程卡住了"}]
    }
  ],
  "redacted": []
}
```

## 6. 功能性需求（MVP 以外的按版本排）

| 编号 | 需求 | 验收（对应 DS / GAP） |
|---|---|---|
| F-01 | 连接：本地 socket（兼容 `.s.KINGBASE.<port>` 命名）/ TCP + SCRAM | 两种方式都能在测试 VM 上连通 |
| F-02 | 角色识别：primary/standby（`sys_is_in_recovery()`），外加每个下游备库（`inst.downstreams`）和备库的上游（`inst.upstream`） | 备库上不适用的 probe 标 `not_applicable` 而非报错；repmgr 状态 v0.2 起 |
| F-03 | 连接族 | DS-01~04；DS-04 对照组（持排他锁的 idle-in-txn）**不得**被标记为可安全终止 |
| F-04 | 锁族：多级阻塞链、按等待时长分级、DDL 专项建议 | DS-05~09；修 GAP-1/2/3 |
| F-05 | SQL 族：Top N 截断必须提示"还有 M 条未显示" | DS-10/11/15；修 GAP-4 |
| F-06 | 等待 + 主机负载：区分"累计值"和"当前值"，读 `/proc` 给 CPU/IO 实时指标 | DS-12~14；修 GAP-5/8 |
| F-07 | vacuum 族：膨胀、停滞原因（关联阻挡 xmin 的会话）、冻结余量 | DS-17~19；修 GAP-6 |
| F-08 | WAL 族 | DS-16/22 |
| F-09 | HA 族：延迟、断连原因提示（崩溃 vs 网络）、切换就绪检查 | DS-20~24；修 GAP-7 |
| F-10 | `diagnose` 跨族关联 | 至少 3 条关联规则：长事务→膨胀/冻结、长事务→锁链、复制槽→WAL 堆积 |
| F-11 | **（v0.2）**`kill`：终止前复查安全条件（不持排他锁、不阻塞他人），不满足拒绝执行除非 `--force` | DS-04 对照组 |
| F-12 | 采集快照导出/回放：`--dump-facts` 从真实实例录制原始采集数据，`--from-facts` 离线重跑判定 | **v0.2 再做**（DS-12~14 第一次需要它时）；MVP 场景全部可真实注入，用不到 |

## 7. 非功能性需求

| 编号 | 需求 | 指标 |
|---|---|---|
| N-01 | 部署 | 单文件静态二进制，linux/amd64 + linux/arm64，不依赖 glibc 版本/libpq |
| N-02 | 自身开销 | 只占 1 个连接；每条 SQL `statement_timeout` 默认 2s、`lock_timeout` 默认 500ms（VM 实测默认 `lock_timeout=0`：DS-09 有 AccessExclusive 排队时，需要表锁的 probe 会被堵死）；命令总超时 `--timeout` 默认 10s；`check` 在健康实例上 ≤ 3s |
| N-03 | 满连接下可用 | `system` 超级用户走 `superuser_reserved_connections`（VM 实测 = 3）；连不上时退出码 69 + 明确原因 |
| N-04 | 只读保证 | 诊断连接 `SET default_transaction_read_only = on`；只有 `kill` 例外 |
| N-05 | 可测试性 | 判定逻辑不接触数据库，能用纯数据跑单测（见 `docs/engineering.md` 架构） |
| N-06 | 可观测 | `--debug` 在 stderr 打出每条 probe 的 SQL、耗时、行数 |

## 8. 非目标（明确不做）

- 不做常驻进程/监控/历史存储（只做"此刻"的快照；趋势靠外部监控或 `watch`）
- 不做 TUI/GUI，不开交互式 SQL 会话
- 不做自动修复（只给处置 SQL，由人执行；`kill` 是唯一例外且需显式调用）
- **不兼容旧命令名**：新版是新产品，旧 shell 版打 tag 冻结，不提供别名层（理由：别名层会把旧的"按模块分组"结构带进新架构）
- 不支持 Windows 目标机

## 9. 开放问题

| # | 问题 | 我的建议 | 理由 |
|---|---|---|---|
| Q1 | MVP 范围 | **已定（2026-09-23）**：v0.1 是 7 条单次查询命令（见 §10），不做 check/diagnose；`kill` 推到 v0.2 以后 | 用户纠正"简单查询优先"（§12）；取代原先"conn+lock+check+diagnose"的建议 |
| Q2 | 版本支持范围 | 只保证 V8R6；其他版本出现时再加 probe 版本分支 | 手头只有 V8R6 测试环境，承诺测不了的版本就是重复旧版"测试不可信"的错 |
| Q3 | 旧代码处置 | 同仓库：旧 shell 打 tag `shell-final` 后整体删除，新代码从零开始 | 保留历史可追溯，又不让旧代码继续干扰 |
| Q4 | finding 文案语言 | 默认中文，`--lang en` 可选；help 英文 | 用户是中文 DBA；help 英文沿用旧约定 |
| Q5 | 远程模式范围 | MVP 就支持 TCP，但 `host` 族在远程模式下进 `skipped` | 直连协议天然支持远程，成本低；OS 指标在远程不可得要明说 |

## 10. 版本路线

这里只写每个版本的目标和验收；具体做哪些查询，以 `docs/queries.md` 的"版本"列为准。

| 版本 | 目标 | 验收 |
|---|---|---|
| v0.1 | 出事时的第一问：会话、锁、事务、等待、实例概况、复制槽，全是单次查询，不做跨维度关联 | 每条命令的注入场景、阴性场景、诱饵在主库和备库上都通过；只读账号的降级输出正确；用户本人按验收步骤走一遍。对外 tag `v2.0.0-alpha.1` |
| v0.2 | 巡检类（参数、空间、对象、维护、归档）和复制延迟、repmgr 集群状态 | 待 v0.1 验收后排定 |
| 之后 | 连环分析：`diagnose` 跨维度关联、事前/事中/事后三种模式（§12） | 待定 |

## 11. DS 场景表

24 个 DBA 故障场景和 8 个旧版能力缺口（GAP），由旧 shell 版逐场景验收整理而来，原文在 tag `shell-final` 里。每条命令都要能追溯到至少一个 DS（原则 1）。

| DS | 场景 | 要点 |
|---|---|---|
| 01 | 连接数正常 | 全局阴性对照 |
| 02 | 连接数接近 WARN | |
| 03 | 连接数达到 FAIL | 满连接下工具自身仍要能连上（N-03） |
| 04 | 大量 idle in transaction | 对照组：持排他锁的 idle-in-txn **不得**标为可安全终止 |
| 05 | 普通锁等待 | 短暂的正常等待不能升级为严重（GAP-2） |
| 06 | 长事务阻塞 | 锁等待的根因是长事务 |
| 07 | 多级锁等待 | 中间会话自己也在等，杀它不解决问题（GAP-1） |
| 08 | 死锁 | 要能定位到 PID/SQL |
| 09 | DDL 被业务事务阻塞 | 要给 DDL 专项建议（GAP-3） |
| 10 | 单条慢 SQL | |
| 11 | 多条慢 SQL 并发 | 截断要提示还剩多少条（GAP-4） |
| 12 | 缓存命中率异常 | 累计值和当前值要分开（GAP-5） |
| 13 | IO 等待异常 | 需要 OS 层指标（GAP-8） |
| 14 | CPU/负载异常但 SQL 不明显 | 需要 OS 层指标（GAP-8） |
| 15 | 临时文件溢出 | |
| 16 | WAL 增长异常 | 归档失败/复制槽/大事务 |
| 17 | 死元组/膨胀 | |
| 18 | autovacuum 长时间未执行 | 要和 DS-06 的长事务关联起来（GAP-1） |
| 19 | 冻结年龄风险 | 阈值来源要统一（GAP-6） |
| 20 | 备库复制延迟 | |
| 21 | 备库断连 | 要区分进程崩溃还是网络分区（GAP-7） |
| 22 | 复制槽堵塞 | |
| 23 | repmgrd 异常 | |
| 24 | 备库不具备 promote 条件 | |

| GAP | 旧版缺口 |
|---|---|
| 1 | 锁链路不自动拼接，同根同源的 finding 要 DBA 自己关联 |
| 2 | 锁等待不按时长区分严重程度 |
| 3 | DDL 被阻塞时没有专门建议 |
| 4 | Top N 静默截断 |
| 5 | 缓存命中率是启动以来的累计值，文案不提示 |
| 6 | 冻结阈值在两个命令里来源不同 |
| 7 | 备库失联不区分崩溃和网络分区 |
| 8 | 没有 OS 层实时资源指标 |

## 12. 需求探索结论

**已确认的环境事实**（VM 实测 2026-09-23，另有注明除外）：
- V8R6 内核基线是 PG12（`server_version_num=120001`），`data_checksums=on`
- 自带 `sys_kwr` 1.8，含会话级历史采样 KSH（`perf.ksh_*`）；默认 `sys_kwr.enable=off`、`collect_ksh=off`，`track_os=on`
- 客户生产库 **KWR 常开，KSH 通常不开**（用户确认）
- `sys_hypo`、`sys_stat_statements` 可用

**案例 1：某基金公司 insert 偶发 34s（2026-09-08）**。平时毫秒级的 insert，50 个并发同时卡了 34 秒。当时依次查了数据库日志、KWR（等待占比 55%，`buffer_content`）、OS IO（100%，同时段有 checkpoint）、WAL 解析（FSM FPI 加叶子页分裂），根因仍没能坐实。待验证的假设是：单调递增的索引键 + 大量删除页 + 当时有东西压住了 xmin 视界（长事务、复制槽、备库 `hot_standby_feedback`、2PC），导致删除页不能复用。

启示：
1. 快照式实时诊断在这个案例里帮不上：故障已经过去，只持续了 34 秒
2. 最缺的是出事时的会话级证据；KSH 本来能提供，但没开
3. 深度要有边界：WAL 解析和读源码是内核研发的活；kbdiag 该做的是把证据收全、把容易漏掉的关联点出来、给出排查方向
4. 由此得到按时间分三种模式的构想：**事前**（可观测性就绪检查 + 结构性风险扫描）、**事中**（实时诊断）、**事后**（按时间窗取证：KWR、日志切片、checkpoint、OS、LSN 范围）

**用户纠正：简单查询优先**（2026-09-23）：
> "查询也可以是单次的，就是看看有没有锁、会话信息等，可能只是看看，就知道了；但有时候是需要连环分析。我们先把简单的做了，再考虑深度的、复杂场景。"

所以 v0.1 只做单次查询；三种模式和案例 1 这类深度分析放到 v0.1 之后。

**待讨论**：更多不同类型的案例，用来检验三种模式的划分；KSH 的开销，以及事前模式要不要建议客户打开它；KWR 到底快照了哪些内容（有没有活动会话和 xmin），这决定事后模式能还原到什么程度。
