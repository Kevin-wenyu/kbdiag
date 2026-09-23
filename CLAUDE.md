# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

kbdiag 2.0 — KingbaseES 命令行诊断工具，Go 重写（单个静态二进制，直连线协议），不进交互界面直接查实例状态。支持单机和主备集群（repmgr）。v0.1 只做简单的单次查询（会话、锁、事务、等待、实例概况、复制槽），连环分析放到后面。

当前进度看 `.omc/plans/` 里唯一的计划文件；发生过什么看 `chronicle/`。

## 活文档（本表是唯一权威副本）

| 文档 | 唯一职责 |
|---|---|
| `README.md` | 使用者：安装、命令、示例 |
| `CLAUDE.md` | 设计理由、开发约定、KES 坑、本表 |
| `docs/PRD.md` | 需求、范围、输出契约、版本目标和验收、DS 场景表 |
| `docs/queries.md` | 查询清单（唯一的"版本"列）、probe_id、DS、SQL 出处、开关、验证状态 |
| `docs/engineering.md` | 选型、架构、测试分层、故障注入、运行命令 |

`docs/agents/` 是给 agent 技能读的配置说明，不算活文档。配套的用户手册在独立仓库 `kbdiag-docs`（Hugo 站点，中英双语），归它自己的仓库管，也不算本仓库的活文档：v2 的页面写在它的 `v2` 分支上，每条命令过了 VM 测试再写，示例用 VM 实跑结果并注明 Go commit。文档和代码保持同步（用户 2026-09-24 定）：代码推到 kbdiag main 后，对应的文档页也推到 kbdiag-docs 的 `v2` 分支，不在本地攒着；推 `v2` 不会触发站点部署（两个部署流程都只认 main）。发布 `v2.0.0-alpha.1` 时合进它的 main，在这之前线上站点保持 v1 手册。`AGENTS.md` 是 Codex 的入口，只指向本文件，不算活文档。

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

### 三层深度（看 / 查 / 断）

保留为概念，不体现在命令分组上（PRD §4）：看 = 给一个确定事实；查 = 单维度深查，输出可机读，也用来验证"断"的结论；断 = 多维关联，输出症状→证据→根因→建议的链路。v0.1 只做看和查。

### `sessions` v0.1 输出形态（用户确认 2026-09-23）

- 默认按事务时长 `xact_age_s` 从长到短排列，空值排后，同值按 PID 升序。
- 默认结果包含后台进程；它们可能占据输出前几行。
- 默认最多显示 50 行。
- 文本表格标题显示当前展示行数（例如 `3 rows`）；截断提示另报剩余行数。JSON 的 `truncated` 表示未显示的行数。
- 命令名、列名/顺序、JSON 字段名、finding.id 和 probe_id 以 PRD §5/§5.1 为契约；本次 node1 实测未发现不符。

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
