# kbdiag 下一轮功能需求分析（2026-07-14）

## 背景

DBA 工具箱队列 #0–#8 已全部完成（2026-07-09），spec 2026-07-04 的差距清单基本清零。
本文档回答：**在现有 31 个命令之上，还缺什么？** 按真实运维场景倒推，而非按功能清单顺推。

## 现状盘点（能力边界）

- **看层**（9）：status / license / cluster / replication / check / space / backup / params / update
- **查层**（19）：sessions / locks / perf / sql / stmt / explain / wait / progress / jobs / partition / stat / obj / colstat / temp / idx / kill / conf / audit / logs
- **断层**（2）：diagnose / advisor
- **横切**（3）：watch / remote / all

**架构红线**（已确认决定，不可违背）：纯无状态、非交互、no sudo assumed、单机可降级。
趋势/历史快照类需求一律出局（trend 已否决过一次）。

## 差距分析：真实场景倒推

| 场景 | 现状 | 差距 |
|------|------|------|
| S1. HA 集群交付验收 / failover 演练前检查 | `cluster` 只显示当前拓扑，`conf diff` 只比参数 | 无 failover 就绪度判断：witness、quorum、repmgr.conf 一致性、standby 可提升性、归档在两端是否都通 |
| S2. 例行巡检交付（信创甲方普遍要月度巡检报告） | `all` 是命令输出的顺序堆叠，`diagnose` 是故障时的根因链 | 无面向交付的结构化报告：结论先行、分级汇总、可存档的单文件产物 |
| S3. 新环境部署 / 迁移上线前体检 | 所有检查都在 DB 层 | OS 层零覆盖：THP、swap、ulimit、overcommit、时钟同步、文件系统 —— 国产化环境装机最常见的坑恰恰在这层 |
| S4. 接入现有监控体系（Prometheus/夜莺） | 部分命令有 `--json` | 无标准 metrics 输出，接监控要自己写解析脚本 |
| S5. 故障时刻的现场快照留存 | 各命令即查即走 | 无一键打包"事发现场"（会话/锁/等待/日志尾部）供事后分析或上报原厂 |

## 候选需求

### R1. `cluster --failover-ready`（或 `cluster check`）— spec 遗留，查/断层

- **内容**：witness 节点存在性与连通、节点数与 quorum 合理性、各节点 repmgr.conf 关键参数一致性（priority/timeout/promote_command）、standby 的 `recovery_min_apply_delay`、复制槽与归档在主备两端的状态、`sys_replication_slots` 残留检测。输出 checklist：每项 OK/WARN/FAIL + 修复建议。
- **价值**：高。HA 交付验收必查项；failover 演练翻车多数因为配置漂移，事前可查。
- **成本**：中。repmgr 元数据表 + `remote` 已有的跨节点能力可复用；需 VM 双节点实测。
- **风险**：读 repmgr.conf 需要文件可读（kingbase 用户通常可读，需降级处理）。

### R2. `report` — 一键巡检报告，看层聚合

- **内容**：串接现有命令（check/space/backup/replication/idx/audit/stat…）生成单文件 Markdown 报告：健康总评（OK/WARN/FAIL 计数）→ 分节明细 → 建议清单（复用 advisor）。`--json` 同步支持。不采新数据，只做编排与呈现。
- **价值**：高。信创场景巡检报告是刚需交付物；也是 kbdiag 从"工程师自用"到"团队/甲方可见"的关键一步（呼应 team-rollout plan）。
- **成本**：中。纯编排，无新查询；难点在报告结构设计与耗时控制（复用 stat 共享采样窗口的经验）。
- **风险**：警惕做成第四层——它不是新诊断能力，只是看层的输出形态；结论必须直接引用 check/diagnose 的既有判断，不另起炉灶。

### R3. `oscheck`（或并入 `check --os`）— OS 层体检，看层

- **内容**：THP 状态、swappiness/swap 使用、`ulimit -n/-u`（当前用户即 kingbase，正好是数据库进程的真实限制）、`vm.overcommit_memory`、时钟同步（chrony/ntp 状态或仅偏移检测）、数据目录文件系统类型与挂载参数、CPU 频率策略。全部只读、不需 root（读不到的项显示 SKIP 而非报错）。
- **价值**：中高。部署期问题一半在 OS 层，目前 kbdiag 完全盲区；对"迁移自 Oracle 的国产化项目"尤其常见。
- **成本**：低中。纯本地文件/命令读取，无 SQL；但需处理多发行版路径差异（麒麟/统信/CentOS）。
- **风险**：no-sudo 红线下部分项读不到（如某些 sysctl），必须 SKIP 降级，不能要求提权。

### R4. `metrics` — Prometheus 文本格式输出，横切

- **内容**：单次采样输出 exposition format（`kbdiag_up`、连接数、TPS、复制延迟、命中率、WAL 积压等 ~20 个核心指标），由外部 cron/agent 定时调用。**不做**驻留 exporter（违背非交互红线）。
- **价值**：中。有监控体系的用户接入成本降为零；无监控体系的用户用不上。
- **成本**：低。指标全部来自 stat/check/replication 既有查询，只是换输出格式。
- **风险**：低。注意指标命名一次定死，后改即破坏兼容。

### R5. `snapshot` — 故障现场打包，查层

- **内容**：一键采集 sessions/locks/wait/perf/日志尾部/关键参数到 `kbdiag-snapshot-<ts>.tar.gz`，供事后分析或提交原厂工单。
- **价值**：中。故障升级到原厂时"当时现场没留"是常见痛点。
- **成本**：低。纯编排 + 打包。
- **风险**：与"无状态"红线不冲突（产物交给用户，工具自身不读回）；注意日志可能含敏感 SQL，默认脱敏或提示。

## 对抗审查（内联，DBA 专家视角）

1. **"report 和 all/diagnose 重复"** —— 不重复但必须划清：`all` 给自己看（原始输出），`diagnose` 出事时用（根因链），`report` 没事时交差用（结论+分级汇总）。若 report 只是 all 换皮，就不值得做——差异化在"总评分级 + 建议汇总 + 可存档单文件"。
2. **"oscheck 越界了，kbdiag 是 DB 工具"** —— 反驳成立一半：不做 OS 性能分析（那是 sar/atop 的事），只做**与数据库直接相关的 OS 配置符合性检查**，每项都能说出"这项错了数据库会怎样"。范围钉死在这个标准上。
3. **"metrics 是伪需求，用户可以 --json 自己转"** —— 部分成立。降为可选项：只有当目标用户明确有 Prometheus 体系时才值得；否则延后。
4. **"snapshot 会诱导用户把它当备份/审计工具"** —— 文档明确定位：现场留存，非备份。敏感信息默认脱敏。
5. **顺序上**：R1 是 spec 明确遗留欠账，先清欠账再开新方向，符合项目一贯做法。

## 优先级建议

| 优先级 | 需求 | 理由 |
|--------|------|------|
| P0 | R1 failover 就绪检查 | spec 遗留欠账，价值高，基础设施（remote/conf diff）已备 |
| P1 | R2 report 巡检报告 | 交付刚需，纯编排成本可控，呼应 team-rollout |
| P2 | R3 oscheck | 补盲区，独立性强，可随时插队 |
| P3 | R5 snapshot | 低成本高性价比的小功能 |
| 延后 | R4 metrics | 等有真实监控接入需求再做 |

## 决议（2026-07-14 用户确认）

1. **本轮范围**：R1 + R2 + R3 + R5 全做（P0–P3），R4 metrics 延后。
2. **R2 报告格式**：Markdown 单文件。
3. **R3 命令形态**：`check --os`，并入现有健康检查框架，不新增顶层命令。

实现顺序按优先级：R1 failover 就绪检查 → R2 report → R3 check --os → R5 snapshot。
下一步：逐项出设计（docs/superpowers/specs/）+ 实施计划（docs/superpowers/plans/）。
