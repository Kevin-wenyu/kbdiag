# kbdiag 2026-07 第二轮功能规格说明书：输出深度改造 + baseline + 升级体检 + metrics

日期：2026-07-18
状态：待用户审阅
前置：2026-07-14 需求分析（R1–R3、R5 已交付；原 R4 metrics 延后至本轮）

## 背景

用户核心反馈：**现有命令输出信息太少，只给结论，看不到详细数据。** 实测确认：34 个命令中仅 8 个响应 `-v/--verbose`，且多数只有一处使用——`--verbose` 是全局旗标却基本是空承诺。本轮以输出深度横切改造为最高优先级，同时交付上一轮场景倒推确定的三个新功能。

本设计已经过领域专家对抗审查（10 条意见），其中 5 条改变了设计（一次查询两次呈现、TSV 存储、节点身份校验、计数器重置防护、Prometheus counter 规范），其余作为实现注意事项写入各节。

## 范围与排期

| 轮次 | 内容 | 规模 |
|------|------|------|
| R1 | 输出深度横切改造（34 命令） | 大，分 3 批灰度 |
| R2 | `baseline` 变更前后对比（新命令） | 中 |
| R3 | `check --upgrade` 升级前体检 | 小 |
| R4 | `metrics` Prometheus 文本输出（新命令） | 小 |

每个 R 独立提交 + 全量回归通过后进入下一个。

**撤销记录**：cluster verify（切换后验证）撤销——用户对它的诉求实为 cluster ready 输出太薄；R1 补齐明细后重新评估是否仍需独立命令。

## R1 输出深度横切改造

### 两条规则

1. **默认输出禁止裸结论。** 每个 OK/WARN/FAIL 结论行必须内联实测值，有阈值的须同时给出阈值。
   - 反例：`[OK] Archiver: no failures`
   - 正例：`[OK] Archiver: no failures (archived 52, last 2026-07-17 22:04)`
   - 正例：`[WARN] Swappiness: 20 > threshold 10`
2. **所有命令 `-v` 展开底层明细表。** 明细是结论的完整源数据（如 replication 展开每个 standby 的 sent/write/flush/replay LSN 四元组与字节差）。

### 核心设计：一次查询，两次呈现

每个检查项**只查一次库**：命令捕获完整结果集（变量/临时文件），结论从该结果集计算，`-v` 渲染同一份已捕获数据。禁止"结论一条 SQL、明细另一条 SQL"的二次查询——避免两个时刻的数据自相矛盾，也保证 `-v` 不增加 DB 往返（DB 半挂时最需要 `-v`，工具不能自己先变慢）。

core.sh 新增渲染 helper（只渲染、不查询）：

```
detail_begin <title>        # -v 且 text 格式时输出明细块标题
detail_row  <col>...        # 渲染一行（内部处理对齐）
detail_end                  # 结束明细块
detail_raw  <multiline>     # 直接渲染预格式化块（如 EXPLAIN 树、日志原文）
```

### JSON 语义

`--format json` 时明细**总是**全量输出到各项的 `detail` 字段，`-v` 只控制 text 渲染。机器消费方无需带 `-v` 即拿到全量结构化数据。

### 分批灰度

默认输出格式变化会波及现有 377 条回归断言，按批推进，每批全量回归通过后进下一批：

1. **批 0**：core.sh helper + 测试框架断言约定（断言明细**内容**——已知列名与值，不只断言块存在）
2. **批 1 看层**（10）：status license cluster replication check space backup params report update
3. **批 2 查层**（21）：sessions locks perf sql stmt explain wait progress jobs partition stat obj colstat temp idx kill conf audit logs snapshot remote
4. **批 3 断层 + 横切**（3）：diagnose advisor watch

### 各命令 `-v` 明细清单

| 命令 | `-v` 明细内容 |
|------|---------------|
| status | 进程明细（pid/启动时间/内存）、监听地址端口、数据目录、关键运行时参数 |
| license | license 文件全部字段原文 |
| cluster | repmgr 节点表全列、每节点连通测试结果、`cluster ready` 各检查项源数据 |
| replication | 每 standby：sent/write/flush/replay LSN、各段字节差、sync_state、slot 状态、wal_receiver 明细 |
| check | 每检查项完整源数据（连接明细、xid 年龄 Top 表、archiver 全统计、锁等待清单等） |
| space | df 每文件系统明细、Top 表/索引尺寸清单、WAL 段数与归档积压、表空间映射 |
| backup | sys_rman 最近备份记录（类型/时间/大小/状态）、slot 明细、archiver 全统计 |
| params | 增加 source/pending_restart/unit/boot_val 列 |
| sessions | 全列：client_addr、backend_start、事务时长、wait_event、query 截断全文 |
| locks | 完整锁等待链、每锁 mode/granted/relation/pid/持有时长 |
| perf | 各子命令底层视图全行输出 |
| sql | 完整 SQL 文本 + 完整执行计划 |
| stmt | 全统计列（stddev、rows、shared/temp blks 读写、WAL） |
| explain | 完整计划树 + 每节点成本/行数估算 |
| wait | 按事件分组的会话清单（pid/等待时长/query 截断） |
| progress | 各 progress 视图全列 |
| jobs | 每 job 最近 N 次运行记录（状态/耗时/错误） |
| partition | 每分区大小/行数/边界明细 |
| stat | 数据库级全统计列 + 每库分行 |
| obj | 原始 pg_class/索引/约束/统计行 |
| colstat | 完整 MCV/histogram 数组 |
| temp | 每会话 temp 文件数与字节明细 |
| idx | 每索引扫描次数/大小/定义全文 |
| kill | 目标会话完整信息（dry-run 时同样展示） |
| conf | 全部非默认参数对比表 |
| audit | 每检查项证据行（用户/权限清单、hba 规则原文等） |
| logs | 匹配日志原文行（截断保护） |
| snapshot | 打包清单（文件/大小/脱敏计数） |
| diagnose / advisor | 每个证据节点的完整源查询结果 |
| watch / remote | 透传被包装命令的 `-v` |
| update | 豁免规则②（无 DB 数据）；规则①适用（版本号、下载源、校验结果内联） |

### 测试与验收

- 每命令至少一条 `-v` 明细内容断言（含已知列名/值，非仅存在性）
- 每命令至少一条默认输出"结论行含数值"断言
- `--format json` 断言 `detail` 字段存在且结构合法
- 全量回归通过；报告/快照类命令（report/snapshot）内嵌数据同步升级

## R2 baseline 变更前后对比

### 命令

```
kbdiag baseline save [name]        # 默认 name = <hostname>-<YYYYMMDD-HHMMSS>
kbdiag baseline list
kbdiag baseline compare <a> [b]    # b 缺省 = 当前实时状态
kbdiag baseline rm <name>
```

### 存储：TSV 目录（不用 JSON）

```
~/.kbdiag/baselines/<name>/
  meta.kv       # k=v：hostname、role(primary/standby/standalone)、system_identifier、
                #        实例启动时间、各 stats_reset 时间戳、kbdiag 版本、采集时间
  params.tsv    # name  setting  source
  stat.tsv      # 累计计数器原始值（xact_commit/rollback、blks_hit/read、tup_*、连接数、库大小）
  stmt.tsv      # queryid  calls  total_time  mean_time  rows  shared_blks_read  query_80
  space.tsv     # 库大小、Top-20 表大小、WAL 目录大小、归档积压数
```

纯 bash/awk 读写；SQL 文本只存 queryid + 单行截断 80 字符（展示用，关联靠 queryid）；tab/换行在采集时剥离。

### compare 语义

1. **身份校验**：hostname 或 system_identifier 不符 → FAIL 退出并提示；`--force` 越过，输出标注"跨节点对比，结论仅供参考"。
2. **重置防护**：b 侧实例启动时间晚于 a 侧采集时间，或 stats_reset 变化 → 速率差标记"不可算（计数器已重置）"，绝不输出负速率。
3. **输出四节**：
   - 参数变更：新增/删除/值变化清单（含 source）
   - 吞吐速率差：由 (counter_b − counter_a)/Δt 计算 TPS、命中率变化
   - Top SQL 变化：mean_time 变化超 **±50%** 且两侧 **calls≥20**、**mean_time≥1ms** 才标记；对不上的 queryid 分列"新增 Top SQL"/"消失"，不做涨跌判断
   - 空间增长：库/Top 表/WAL 增量与增速
4. exit code：0=无显著变化，1=有标记项。遵循 R1 输出规则（结论带数值，`-v` 展开全量对比行）。

### 验收

- save→改参数/跑负载→compare 能正确报告参数变更与 SQL 耗时变化（VM 实测）
- 重启实例后 compare 正确标记"计数器已重置"
- node1 的 baseline 在 node2 上 compare 被身份校验拦截

## R3 `check --upgrade` 升级前体检

沿用 `check --os` 先例，不新增顶层命令。**能力边界声明**（写入 help 与输出头部）：*Generic pre-upgrade risk inventory. Passing does not certify version compatibility with any specific target release.*

| 检查项 | 级别语义 |
|--------|----------|
| 非默认参数清单 | 恒 INFO（盘点，不影响 exit code） |
| 已安装扩展与自定义对象清单 | 恒 INFO |
| prepared 事务残留 | 有 → FAIL |
| 长事务（>1h，可配） | 有 → WARN |
| 复制延迟 | 超 check 现有阈值 → WARN/FAIL |
| 磁盘余量 vs 数据量 | 余量 < 数据量×1.2 → WARN；< 数据量 → FAIL |
| 最近备份新鲜度（sys_rman） | >24h → WARN；无备份 → FAIL |
| license 有效期 | <30 天 → WARN；已过期 → FAIL |

exit 0/1/2 与 check 一致；INFO 项不计入。复用现有 check 框架与 R1 输出规则。

## R4 `metrics` Prometheus 文本输出

### 形态

`kbdiag metrics` 输出 Prometheus exposition 文本（含 HELP/TYPE）到 stdout，零常驻进程、零端口。可选 `--out <file>`：写临时文件后 `mv` 原子替换（textfile collector 目录用）。

### 指标集（前缀 `kbdiag_`，遵守 counter 命名规范）

| 指标 | 类型 | 说明 |
|------|------|------|
| kbdiag_up | gauge | DB 可连=1 |
| kbdiag_scrape_error | gauge | 采集失败=1（失败时仍输出此最小集） |
| kbdiag_scrape_duration_seconds | gauge | 本次采集耗时 |
| kbdiag_connections{state=} | gauge | 按状态连接数 |
| kbdiag_xact_commit_total / _rollback_total | counter | 原始累计值 |
| kbdiag_blks_hit_total / _read_total | counter | 命中率由 PromQL 计算，不导出预算值 |
| kbdiag_database_size_bytes{db=} | gauge | 库大小 |
| kbdiag_replication_lag_seconds{standby=} | gauge | 每 standby；追平时为 0（复用 R 轮已修的 LSN 守卫） |
| kbdiag_archiver_archived_total / _failed_total | counter | 归档统计 |
| kbdiag_locks_waiting | gauge | 等待锁数 |
| kbdiag_sessions_active | gauge | 活跃会话 |
| kbdiag_license_days_remaining | gauge | license 剩余天数 |

### 采集健壮性

- kbdiag 自身失败（DB hang、超时）：仍输出 `kbdiag_up 0` + `kbdiag_scrape_error 1` 最小集，`--out` 模式下同样原子替换——保证监控侧看到的是"采集失败"而非静默的旧值
- README 提供 cron 配方：`flock -n` 防堆积 + `--timeout` 上限

### 验收

- 输出通过 promtool check metrics 校验（本地无 promtool 则以格式断言代替）
- DB 停机时输出最小指标集且 exit 非 0
- `--out` 中断不产生半截文件

## 非目标

- cluster verify 独立命令（撤销，R1 后重新评估）
- 主备数据一致性校验（实现重、对生产有压力）
- metrics HTTP exporter 常驻模式
- 绑定目标版本的升级兼容知识库

## 文档要求

- USAGE/--help 与 README 全英文（既有约定）
- README 新增 baseline / metrics 章节与 cron 配方
