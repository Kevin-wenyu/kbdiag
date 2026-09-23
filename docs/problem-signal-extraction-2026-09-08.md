## CLAUDE.md

1. 现象: 文档明确警告 Lima 分配的 SSH 端口不应硬编码，但同一文件里紧接着的"Login sequence"代码块直接把具体端口号写成字面量命令。
   证据: "SSH 端口由 Lima 每次启动动态分配，不要硬编码" (第 13 行) 与 "ssh -p 57103 kevin@127.0.0.1 # node1 (primary)" / "ssh -p 57123 kevin@127.0.0.1 # node2 (standby)" (第 33、35 行)
   置信度: 中

2. 现象: 文件末尾"测试节点"一节与开头"Login sequence"一节内容高度重复（同样的两个端口号、同样的 sudo/ksql 登录步骤）。
   证据: 第 33-39 行 与 第 171-174 行内容基本一致
   置信度: 低

3. 现象: "三层命令设计哲学"表格里，"看"层列出的命令包含 `version`，"查"层列出 `slow` `bloat` `vacuum` 作为独立命令名。
   证据: 第 64-65 行
   置信度: 低

4. 现象: 一系列 KingbaseES 特有行为被标注为"v2 开发中发现"，暗示这些坑是在开发过程中陆续踩出来的，而非预先掌握的知识。
   证据: "## KingbaseES 特有行为（v2 开发中发现）" (第 78 行)
   置信度: 中

5. 现象: 文中明确说明一种直觉写法（用 `coalesce` 兜底 `||` 拼接的 NULL）在 KingbaseES 上不成立，需改用显式 CASE 判断。
   证据: "不能靠 `coalesce(a||b, fallback)` 兜底，要用 CASE 显式判断" (第 85 行)
   置信度: 高

6. 现象: 文档要求"每次修复这类 shell 陷阱问题都要配一个回归测试，防止再犯"，暗示这类 `set -e`/`set -u` 相关的 bug 曾经反复出现。
   证据: "每次修复这类问题都要配一个回归测试，防止再犯" (第 93 行)
   置信度: 高

7. 现象: 文档专门用一段说明两套技能框架"按场景分工，不是互相替代"，并标注了确认日期，暗示此前对二者的分工边界并不清晰。
   证据: "按场景分工，不是互相替代（2026-08-20 确认）" (第 114 行)
   置信度: 中

8. 现象: 文档显式指出同一类"难缠 bug / 性能回归"能力在两个技能框架里都有对应技能，需要人为规定默认用哪个。
   证据: "取代 mattpocock 的 `diagnosing-bugs`（同类，二选一，默认这个）" (第 129 行)
   置信度: 高

9. 现象: "记录决策"能力同样在两个框架间被拆分，通用 ADR 归 agent-skills，术语层面维护仍归 mattpocock 的 domain-modeling。
   证据: 第 133 行
   置信度: 中

10. 现象: 文档专门用一段"命名冲突提醒"说明本环境同时装了 superpowers 和 mattpocock/skills 两套框架，二者都有同名技能。
    证据: "命名冲突提醒：本环境同时装了...都有 `code-review`/`prototype`/`domain-modeling` 之类的同名技能" (第 153 行)
    置信度: 高

11. 现象: 此前使用的本地 markdown 规格/计划文档（`docs/superpowers/specs/`、`docs/superpowers/plans/`）被要求保留为"历史归档"、"不迁移"。
    证据: 第 159 行
    置信度: 中

12. 现象: 领域文档体系（`CONTEXT.md` + `docs/adr/`）被描述为"尚未生成"，当前只能以 CLAUDE.md 里的一节内容代为权威来源。
    证据: 第 166-167 行
    置信度: 高

## CONTEXT.md

13. 现象: 文件标题自称"领域术语表"，但全文只定义了一个术语（Workload）。
    证据: 全文共 9 行，第 5-9 行
    置信度: 中

14. 现象: 术语表明确要求避免把该概念命名为"AWR/KWR"（理由是"绑死具体实现"），但词条本身又不得不提到底层实现可能来自 `sys_kwr` 扩展。
    证据: 第 8-9 行
    置信度: 低

## README.md

15. 现象: 中文版 `license` 命令描述比英文版多出"序列号"字段，英文版完全没有提及该字段。
    证据: 英文第 93 行 vs 中文第 265 行
    置信度: 中

16. 现象: 英文版 `conf` 命令描述强调"restart-pending status / cross-node comparison"，中文版描述为"配置审计；diff 比对节点差异"。
    证据: 英文第 126 行 vs 中文第 294 行
    置信度: 低

17. 现象: 英文版 DBA 层命令表中 `idx`/`kill` 顺序与中文版不一致。
    证据: 英文第 124-129 行 vs 中文第 294-299 行
    置信度: 低

18. 现象: `workload` 命令的中英文描述都使用了"AWR-style"/"AWR 风格"来形容该功能，与 CONTEXT.md 术语表要求避免的说法冲突。
    证据: 英文第 114 行；中文第 284 行
    置信度: 低

19. 现象: 全局参数一节声明除 `watch` 外所有命令都支持 `--format json` 输出，用词是绝对化的"every command"。
    证据: 第 70-71 行
    置信度: 低

## docs/dba-scenario-acceptance.md

20. 现象: 文档自称处于"场景设计阶段"，明确声明本轮不改代码、不改测试脚本，24 个场景尚未转化为可执行验收物。
    证据: 第 3 行
    置信度: 高

21. 现象: 文档明确区分"现有测试回答的问题"与"本文档想回答的问题"，暗示现有测试无法回答后者。
    证据: 第 4 行
    置信度: 高

22. 现象: 文档指出部分现有测试只断言退出码或"是否有输出"，不验证内容正确性。
    证据: 第 15 行，点名 `test_locks_wait_runs`、`test_locks_deadlock_output`
    置信度: 高

23. 现象: `test_diagnose_section_locks` 在健康环境（无锁等待）下不会产生"锁"这个 finding，该测试因而必然依赖其他巧合文本才能通过。
    证据: 第 16 行
    置信度: 高

24. 现象: 同一条测试不验证 kbdiag 报告的"锁"是否就是测试本身制造的那把锁，一个无关的误报也能让测试通过。
    证据: 第 16 行
    置信度: 高

25. 现象: `test_locks_hold_shows_locks` 的 setup 只建表插数据，不实际加锁，但测试断言仍能通过。
    证据: "这条测试从未真正持锁过" (第 17 行)
    置信度: 高

26. 现象: `_locks_hold` 命令在无锁状态下也会打印同一个表头字符串，导致该测试实际验证的是"命令有输出表头"而非"能否发现持锁会话"。
    证据: 第 17 行
    置信度: 高

27. 现象: `test/setup/make_lock.sql` fixture 存在，但检索显示从未被任何 `test_*.sh` 引用执行，是孤儿 fixture。
    证据: 第 20 行
    置信度: 高

28. 现象: `make_bloat.sql`/`setup_bloat_table()` 有被测试引用，但检索未发现与之对应的 bloat 场景断言用例。
    证据: 第 21 行
    置信度: 中

29. 现象: 文档用"是目前质量最高的一组"形容慢查询相关测试，隐含其余测试组的质量相对更低。
    证据: 第 22 行
    置信度: 中

30. 现象: 全仓库只有一条测试用"另一条命令的裁决结果反向验证 diagnose 结论一致性"，暗示这种验证模式在其余测试里普遍缺失。
    证据: "全仓库唯一一条" (第 23 行)
    置信度: 高

31. 现象: 文档结论——诊断代码本身的证据链设计比测试所暴露出来的验收深度更完整，风险不在代码能力而在测试未真正验证过该能力。
    证据: 第 25 行
    置信度: 高

32. 现象: `check` 命令统计连接数用 `SELECT count(*) FROM sys_stat_activity`，未排除 kbdiag 自身连接，而 `_diag_connections`/`_diag_long_txn` 已有排除逻辑。
    证据: 第 76 行
    置信度: 高

33. 现象: 连接数耗尽时 kbdiag 自身诊断查询能否连上数据库，文档明确要求验收时"明确观察并记录"而非假设一定能连上，说明目前尚未被确认。
    证据: 第 116 行
    置信度: 中

34. 现象: `check` 的长事务查询用 `state IN ('active','idle in transaction')` 不区分两种状态，可能让 DBA 误解告警含义。
    证据: 第 130 行
    置信度: 中

35. 现象: "可安全终止"标记逻辑（`has_excl` 检测）是否真的能正确排除持有排他锁的会话，文档要求验证，暗示当前没有把握。
    证据: 第 131 行
    置信度: 高

36. 现象: 若"可安全终止"标记逻辑有 bug，DBA 可能照建议直接终止一个正持有锁、有业务副作用的连接，被定性为该功能里风险最高的一种误报。
    证据: 第 137 行
    置信度: 高

37. 现象: `_diag_locks` 对所有锁等待固定给 `WARN` 级别，不像 `_diag_long_txn` 按时长升级到 `CRITICAL`，文档将其记为粒度缺口。
    证据: 第 161 行
    置信度: 高

38. 现象: diagnose 对同一阻塞链的两个 finding（长事务与锁等待）之间是否存在互相矛盾的建议，文档要求验证，说明设计上有可能出现矛盾。
    证据: 第 189 行
    置信度: 低

39. 现象: `_diag_locks` 的 SQL 只做一层 JOIN，只能报告直接阻塞关系，不会把多级阻塞链自动拼接成完整链路。
    证据: 第 206 行
    置信度: 高

40. 现象: 若 kbdiag 输出没有明确提示"中间会话自身也在等待另一个会话"，DBA 可能误将中间会话当作根本解决方案而终止，未触及真正持锁的源头。
    证据: 第 212 行
    置信度: 高

41. 现象: kbdiag 现有实现无法回答死锁发生的具体时间、涉及的 PID、SQL 内容，只能回答"发生过、一共几次"。
    证据: 第 232 行
    置信度: 高

42. 现象: 死锁相关的日志行大概率不在 `kbdiag logs` 当前扫描范围内，但需要验收时才能确认。
    证据: 第 234 行
    置信度: 中

43. 现象: 等待方是 DDL 语句时，kbdiag 给出的建议文案与普通锁等待相同，没有针对性提示（如 `lock_timeout`、`CREATE INDEX CONCURRENTLY`）。
    证据: 第 257 行
    置信度: 高

44. 现象: `_diag_slow_queries` 的 finding 级别固定为 `INFO`，与其他判定型 finding 的级别体系（WARN/CRITICAL）不一致。
    证据: 第 275 行
    置信度: 高

45. 现象: `_perf_slow`/`_diag_slow_queries` 对超过 `TOP_N` 的结果做静默截断，不像 `_perf_bloat` 那样明确告知还有多少条未显示。
    证据: 第 296 行
    置信度: 高

46. 现象: 慢查询计数和历史均值统计在两处代码路径使用不同的计数口径（逐行 vs 按 queryid 聚合）。
    证据: 第 302 行
    置信度: 高

47. 现象: `advisor params` 给出的 `shared_buffers` 推荐值在测试 VM 实际内存下是否合理，需验收时确认，暗示存在推荐值超过物理内存的潜在风险尚未验证。
    证据: 第 321 行
    置信度: 低

48. 现象: Buffer 命中率是数据库启动以来的累计值而非滑动窗口，`check` 的输出目前没有注明这是累计值，容易导致持续误报。
    证据: 第 323 行
    置信度: 高

49. 现象: kbdiag 没有直接的磁盘延迟指标，只能看数据库层面的 wait event，不能看 OS 层的 iostat，被称为"架构性边界"。
    证据: 第 341 行
    置信度: 高

50. 现象: IO 等待事件的 `wait_event_type` 字符串在真实 KingbaseES 版本上是否与 PostgreSQL 生态一致，尚未核实。
    证据: 第 343 行
    置信度: 中

51. 现象: DS-14 场景被文档自身描述为"预期会暴露真实能力缺口"的场景，而非要求 kbdiag 通过的场景。
    证据: 第 353 行
    置信度: 高

52. 现象: kbdiag 全线命令都不直接采集 `/proc/loadavg`、CPU 使用率、`vmstat` 等系统指标，`check --os` 也不含实时 CPU 负载数值。
    证据: 第 357 行
    置信度: 高

53. 现象: `cmd_temp.sh` 的代码注释指出 KingbaseES 没有单会话实时 temp 列，只有累计的库级/语句级统计，需验证文案是否会让人误以为看到的是实时数据。
    证据: 第 381 行
    置信度: 高

54. 现象: DS-16 场景（"WAL 增长异常"）与 backup 域的"归档失败"场景存在重叠但触发路径不同，需要专门说明覆盖两种子路径。
    证据: 第 387 行
    置信度: 中

55. 现象: 子场景 B（纯写入量大但归档健康）不会被误判为归档失败，措辞里承认"理论上不会混淆"但仍需在同一次测试里分别触发才能确认。
    证据: 第 409 行
    置信度: 低

56. 现象: 小表（<1000 行）膨胀不会被误报，需要专门验证这道行数门槛是否生效，否则会是"很容易踩的噪音源"。
    证据: 第 432 行
    置信度: 中

57. 现象: "7 天未清理"陈旧判断逻辑本身，文档承认无法在验收阶段用真实等待时间实测，只能靠代码审查确认。
    证据: 第 444 行
    置信度: 高

58. 现象: `perf vacuum` 输出里"超过阈值"和"仅陈旧但未超阈值"这两类计数是否被清楚区分，还是混在一起让 DBA 分不清，需验收确认。
    证据: 第 447 行
    置信度: 中

59. 现象: 表膨胀 finding（`_diag_bloat`）与长事务 finding（`_diag_long_txn`）之间完全独立生成、互不引用，即使二者可能同根同源。
    证据: 第 453 行
    置信度: 高

60. 现象: `advisor vacuum` 判断 freeze 风险用硬编码的 5 亿阈值，不读取可配置的 `KB_WARN_XID` 环境变量，与 `check`/`diagnose` 使用的阈值来源不一致。
    证据: 第 467 行
    置信度: 高

61. 现象: 三处代码（`advisor vacuum`、`check`、`diagnose`）分别用不同 SQL 计算 `relfrozenxid` age，实现路径不同，理论上结果应相同但未交叉验证。
    证据: 第 474 行
    置信度: 中

62. 现象: primary 视角（write_lag）与 standby 视角（replay delay）的复制延迟数值理论上应互相印证，但目前可能"各自为战"、未被交叉验证过。
    证据: 第 491 行
    置信度: 低

63. 现象: repmgr 对"进程崩溃"和"网络分区"两种故障上报的具体 status 文案可能不同，需要实测确认，目前尚不清楚。
    证据: 第 511 行
    置信度: 中

64. 现象: `kbdiag replication`/`cluster` 目前只能看到"连接没了"的结果，不能区分是 standby 进程崩溃还是网络断开。
    证据: 第 514 行
    置信度: 高

65. 现象: active 但消费很慢的复制槽，只有 `check` 的 lag 判断能命中，`cluster ready` 的 inactive 判断不会命中，尚未实测确认两者互补关系。
    证据: 第 532 行
    置信度: 中

66. 现象: GAP-1，锁链路不自动拼接，多级阻塞链需要 DBA 自行在多个 finding 间关联，kbdiag 不提供。
    证据: 第 619 行
    置信度: 高

67. 现象: GAP-2，锁等待不区分严重程度，`_diag_locks` 固定给 WARN，不像 `_diag_long_txn` 按时长升级到 CRITICAL。
    证据: 第 620 行
    置信度: 高

68. 现象: GAP-3，DDL 阻塞场景没有专门建议文案，与普通锁等待建议相同。
    证据: 第 621 行
    置信度: 高

69. 现象: GAP-4，Top N 截断无提示，静默截断不像 `_perf_bloat` 那样告知还有多少条未显示。
    证据: 第 622 行
    置信度: 高

70. 现象: GAP-5，Buffer Hit 累计值未标注，长期运行实例容易被历史一次性物理读拖累出持续误报。
    证据: 第 623 行
    置信度: 高

71. 现象: GAP-6，Freeze 阈值来源不一致，同一实例可能在两个命令里得到不一致的"要不要处理"结论。
    证据: 第 624 行
    置信度: 高

72. 现象: GAP-7，Standby 失联不区分崩溃/网络分区，无法提示排查方向。
    证据: 第 625 行
    置信度: 高

73. 现象: GAP-8（架构性），无 OS 层实时资源指标，CPU 负载、内存压力、磁盘 IO 延迟均不在采集范围内。
    证据: 第 626 行
    置信度: 高

74. 现象: 这些已识别的能力缺口是否值得修复、优先级如何排，文档明确表示留待场景集通过验收后再讨论，本轮不动代码。
    证据: 第 628 行
    置信度: 高

75. 现象: 文档末尾列出三条后续路径并注明"待用户拍板"，说明走哪条路径尚未决定。
    证据: 第 632-634 行
    置信度: 高

## docs/test-report-2026-07-09.md

76. 现象: 报告总体结论宣称"349 个用例，347 通过，2 失败"，但分套件结果表格里每个套件"总数"逐一相加得到 343，不是 349。
    证据: 第 18 行 对照 第 20-39 行表格
    置信度: 中

77. 现象: "分套件结果"表格中出现一行左列单元格内容为"三层覆盖"、通过/总数栏为空，与其余行格式不符，像是说明文字被错放进表格单元格。
    证据: 第 39 行
    置信度: 高

78. 现象: 总体结论称"32 个命令套件中 30 个全绿"，但表格里实际列出的具名套件数量与非全绿套件数在数量上难以直接对上。
    证据: 第 18 行 对照第 20-39 行
    置信度: 低

79. 现象: 报告将 `stat` 命令 10.3 秒耗时瓶颈最初描述为"Top SQL 段聚合"，但后段"遗留问题清单"里又说"真实根因是吞吐采样与 temp files 段各睡一个采样窗口"，前后矛盾。
    证据: 第 72 行 对照 第 85 行
    置信度: 高

80. 现象: 报告称"advisor 的 ANALYZE 建议在修复 relname 歧义后首次真正可用"，暗示此前该功能不可用或不正确。
    证据: 第 77 行
    置信度: 高

81. 现象: 被测版本说明提到"含当日 audit 白名单 + 消重两次提交"，暗示同一天内为解决重复/消重问题提交了两次。
    证据: 第 4 行
    置信度: 低

82. 现象: `sessions`/`replication` 两个命令不支持 `--format json`，是测试用例按"期望所有常用命令支持"的目标行为编写、实现尚未跟上导致的失败。
    证据: 第 48 行
    置信度: 高

83. 现象: `replication` 命令的性能门禁测量结果为负值（-298ms），被定性为"测量瑕疵"。
    证据: 第 59、70 行
    置信度: 高

## docs/agents/domain.md

84. 现象: 文档指出如果模型使用的领域概念不在 `CONTEXT.md` 术语表里，这本身就是一个"信号"——要么是发明项目不用的语言，要么是术语表存在真实空缺。
    证据: 第 29 行
    置信度: 低

85. 现象: 文件结构示例中的 ADR 文件名以省略号占位，文档同时说明 `docs/adr/` 若不存在应"静默处理，不要标记其缺失"。
    证据: 第 19-20 行；第 10 行
    置信度: 低

## docs/agents/issue-tracker.md

86. 现象: 文档说明仓库里已有 20 篇以上预先存在的设计规格/执行计划文档，来自 GitHub issue tracker 建立之前就已在用的"superpowers"技能框架，被保留为历史归档而不再更新。
    证据: 第 30 行
    置信度: 中

## docs/agents/triage-labels.md

87. 现象: kbdiag 的 GitHub 仓库设置 triage 标签体系时没有预先存在的 issue/标签，因此直接照搬了 mattpocock/skills 的默认标签命名。
    证据: 第 15 行
    置信度: 低

## docs/superpowers/specs/2026-06-19-kbdiag-feature-design.md

88. 现象: 目录结构中三个新命令文件被标注为"NEW"，暗示存在此前未标注为 NEW 的既有版本框架，但本文件未说明自身处于产品的第几个版本。
    证据: 约第 28-30 行
    置信度: 低

## docs/superpowers/specs/2026-06-20-kbdiag-v3-design.md

89. 现象: 文档自称"v3"设计并断言"kbdiag v2 已具备"一系列命令，但目录中前一天日期的文档（06-19）本身并未自称"v2"或任何版本号。
    证据: 约第 7 行
    置信度: 中

90. 现象: 文档提出"HTML 报告留 v4，v3 不做"，但目录中不存在任何自称"v4"的设计文档。
    证据: 约第 15 行
    置信度: 低

## docs/superpowers/specs/2026-06-21-kbdiag-v1.4-design.md

91. 现象: 文档自称"kbdiag 当前版本为 v1.3"并给出 v1.0→v1.4 版本序列，与前一天文档自称"v3"的命名体系完全不同，两者并存于同一目录。
    证据: 约第 5 行；约第 301-307 行
    置信度: 高

92. 现象: 版本表中 v1.3 的命令列表与前一天文档（v3 design）中逐一新增的顶层命令列表高度重合。
    证据: 约第 306 行
    置信度: 中

## docs/superpowers/specs/2026-06-22-diagnose-design.md

93. 现象: "可安全终止"判定条件之一的实现方式尚未确定，需 SSH 进 VM 验证 KingbaseES 是否支持，预先给出了不确定情况下的兜底方案。
    证据: 约第 67 行
    置信度: 高

94. 现象: diagnose 集成到 all 命令后会造成部分查询重复，文档选择暂时接受而非解决。
    证据: 约第 195 行
    置信度: 高

## docs/superpowers/specs/2026-06-22-license-design.md

95. 现象: 文档承认无法仅凭 validdays 数值区分"正式授权"与"试用授权"，两者在有限期情况下展示方式相同。
    证据: 约第 28 行
    置信度: 高

## docs/superpowers/specs/2026-06-22-stmt-design.md

96. 现象: 既有 diagnose 命令的慢查询检查只看当前活动会话，缺少历史维度，属于此前设计遗留的功能缺口。
    证据: 约第 10 行
    置信度: 高

97. 现象: 若 sys_stat_statements 扩展未安装，stmt 命令需要优雅降级，暗示该扩展在部分环境下可能不可用。
    证据: 约第 211 行
    置信度: 中

## docs/superpowers/specs/2026-06-23-roadmap-design.md

98. 现象: 文档承认项目当前"无安全网"，包括无 CI、无 git 守卫、无提交前静态检查。
    证据: 约第 13 行
    置信度: 高

99. 现象: 文档承认测试覆盖不均，"根因层仅有烟测"。
    证据: 约第 14 行
    置信度: 高

100. 现象: 文档承认两个既有命令（watch、stat）"远未达到设计潜力"。
     证据: 约第 15 行
     置信度: 高

101. 现象: 当前直接向 main 分支推送、无分支保护，一次误操作即丢历史。
     证据: 约第 29 行
     置信度: 高

102. 现象: GitHub Actions CI 因环境限制无法运行集成测试（Lima VM 在本机，runner 无法 SSH），只覆盖 shellcheck 和构建验证。
     证据: 约第 19 行
     置信度: 高

103. 现象: 代码中存在硬编码 /tmp 路径的问题（`lib/cmd_logs.sh:47`），需要修复为尊重环境变量。
     证据: 约第 104 行
     置信度: 高

## docs/superpowers/specs/2026-07-04-enterprise-dba-design.md

104. 现象: 文档状态标注为"草案，待确认"，尚未定稿。
     证据: 约第 4 行
     置信度: 高

105. 现象: 原 CLAUDE.md 记录的 SSH 端口 57103 已失效，实际端口 60035，需要更新文档。
     证据: 约第 11 行
     置信度: 高

106. 现象: WAL 归档从未成功过，但 kbdiag 现有 28 个命令里没有一个查询过 `sys_stat_archiver`，一个正在丢失 PITR 能力的实例会显示"一切正常"。
     证据: 约第 13 行
     置信度: 高

107. 现象: 已安装的多个企业级扩展（sysaudit、sysmac/sys_anon、kdb_partman、kdb_schedule、walminer）完全没有被 kbdiag 检查覆盖。
     证据: 约第 14 行
     置信度: 高

108. 现象: sys_stat_statements 其实可用（v1.11），需要确认 advisor/stat 里"扩展不存在时跳过"的降级分支是不是被误触发。
     证据: 约第 15 行
     置信度: 高

109. 现象: 此前规划的本地状态/历史快照存储架构被否决，依赖该架构的功能（trend、执行计划回归对比）因此取消。
     证据: 约第 23 行
     置信度: 高

110. 现象: 用户原话反馈指出现有功能"是不是有重复的，查询深度不够的"，作为本轮工作的触发前提。
     证据: 约第 29 行
     置信度: 高

111. 现象: --help 文本呈现的命令分层与 CLAUDE.md 文档描述的三层架构（看/查/断）不一致。
     证据: 约第 49 行
     置信度: 高

112. 现象: diagnose 命令被错误归类在 [OPS] 组，而它实际是文档定义的"断"层命令。
     证据: 约第 49 行
     置信度: 高

113. 现象: advisor 命令的层级归属边界模糊，既像"查"层又带"断"层性质，未被明确区分。
     证据: 约第 51 行
     置信度: 中

114. 现象: 文档用"两套真相"描述 --help 输出与 CLAUDE.md 文档不同步可能导致的风险。
     证据: 约第 52 行
     置信度: 高

115. 现象: `conf` 与 `params` 两个命令的默认行为几乎完全重复，导致用户困惑该用哪个。
     证据: 约第 54 行
     置信度: 高

116. 现象: 表膨胀/死元组比例在代码中有 7 处独立实现、给出 7 个不同阈值/口径，DBA 用不同命令查同一张表可能得到矛盾结论。
     证据: 约第 70 行
     置信度: 高

117. 现象: 同一个命令（cmd_diagnose.sh）内部的 `_diag_vacuum_debt` 和 `_diag_bloat` 使用了不同的膨胀阈值（20% 和 30%）。
     证据: 约第 67 行
     置信度: 高

118. 现象: 索引检查逻辑在 cmd_idx.sh 与 cmd_advisor.sh 中被逐字复制两份。
     证据: 约第 72 行
     置信度: 高

119. 现象: 等待事件分布查询在 cmd_wait.sh、cmd_perf.sh、cmd_stat.sh 三处被独立实现，基本是同一查询的变体。
     证据: 约第 74 行
     置信度: 高

120. 现象: Top SQL（AWR 风格慢查询排行）逻辑在 cmd_stmt.sh、cmd_diagnose.sh、cmd_stat.sh 三处被独立实现。
     证据: 约第 76 行
     置信度: 高

121. 现象: checkpoint/bgwriter 统计在 cmd_perf.sh 与 cmd_stat.sh 两处存在重叠，字段不完全一致。
     证据: 约第 78 行
     置信度: 高

122. 现象: checkpoint/bgwriter 重叠与 ANALYZE drift 公式重叠两项问题"尚未处理"，未纳入本轮任务范围。
     证据: 约第 43 行
     置信度: 高

123. 现象: audit 命令查询深度最浅（仅 4 项基础检查），完全未覆盖已安装的企业级安全扩展。
     证据: 约第 86 行
     置信度: 高

124. 现象: audit --report 功能因权限架构（三权分立）限制目前不可行，尚未实现。
     证据: 约第 113 行
     置信度: 高

125. 现象: 因权限拒绝而产生的检查结果被 audit 命令误报为"合规"而非"检查失败"。
     证据: 约第 115 行
     置信度: 高

## docs/superpowers/specs/2026-07-14-cluster-ready-design.md

126. 现象: 未采用 repmgr 自带的 node check 命令，原因之一是"输出格式不稳定且逐项判定阈值不可控"。
     证据: 约第 42 行
     置信度: 中

127. 现象: esrep 库与 test 库在布尔值返回格式上不一致（t/f 与 true/false），需要同时兼容两种格式。
     证据: 约第 17 行
     置信度: 高

128. 现象: 实测过程中发现真实故障（node1 归档积压超过 WARN 阈值），说明此前该项检查未被执行过。
     证据: 约第 21 行
     置信度: 高

## docs/superpowers/specs/2026-07-14-next-features-requirements.md

129. 现象: R1 命名写作"cluster --failover-ready（或 cluster check）"，而同日期设计文档最终采用的命令名是"cluster ready"，均不同。
     证据: 约第 30 行
     置信度: 中

130. 现象: 文档对"report 与 all/diagnose 重复"这一质疑以自问自答回应，承认若做成"all 换皮"则不值得做。
     证据: 约第 67 行
     置信度: 高

131. 现象: 文档对"oscheck 越界了，kbdiag 是 DB 工具"这一质疑只承认"成立一半"。
     证据: 约第 68 行
     置信度: 高

132. 现象: "metrics 是伪需求"的质疑被承认"部分成立"，优先级因此降为延后。
     证据: 约第 69 行
     置信度: 高

133. 现象: snapshot 功能存在被用户误用为备份/审计工具的风险。
     证据: 约第 70 行
     置信度: 中

134. 现象: R1 存在权限风险：读取 repmgr.conf 需要文件可读性，部分环境下可能无法满足，需要降级处理。
     证据: 约第 35 行
     置信度: 高

135. 现象: R3（oscheck）需要处理不同 Linux 发行版（麒麟/统信/CentOS）路径差异的实现复杂度。
     证据: 约第 48 行
     置信度: 中

136. 现象: no-sudo 红线下部分 OS 检查项可能读取不到数据，必须降级为 SKIP，不能要求提权。
     证据: 约第 49 行
     置信度: 高

## docs/superpowers/specs/2026-07-15-check-os-design.md

137. 现象: report 命令的 Health check 段需要"升级为" cmd_check --os，暗示此前 report 中调用的 check 不含 OS 检查段。
     证据: 约第 13 行
     置信度: 中

## docs/superpowers/specs/2026-07-15-report-design.md

138. 现象: report 结构清单中的 section 名为"check"，未标注是否包含 --os 段，与同日期 check-os-design.md 的表述未能相互印证。
     证据: 约第 20 行
     置信度: 中

## docs/superpowers/specs/2026-07-15-snapshot-design.md

139. 现象: snapshot 包内容表中 check.txt 来源标注为"cmd_check --os"，与同日 report-design.md 中"check"未标注 --os 的记录不一致。
     证据: 约第 21 行
     置信度: 中

## docs/superpowers/specs/2026-07-18-output-depth-and-next-features-design.md

140. 现象: 文档开头即声明自身是"v2"，此前的四功能版本（v1）已作废并重写。
     证据: 约第 3-5 行
     置信度: 高

141. 现象: 查层命令存在"看见问题却不吭声"的缺陷（如挂起 1 分钟的事务却不吭声），已通过实测演示确认。
     证据: 约第 11 行
     置信度: 高

142. 现象: 34 个命令中仅 8 个响应 -v 参数，结论背后的数据看不到。
     证据: 约第 12 行
     置信度: 高

143. 现象: baseline / check --upgrade / metrics 移出本轮范围，理由之一是"输出改造可能改变对它们的需求判断"。
     证据: 约第 21 行
     置信度: 中

144. 现象: "样板间先行"策略的目的是"防止方向性返工"，暗示团队对此前返工风险有顾虑。
     证据: 约第 21 行
     置信度: 中

145. 现象: colstat/temp 命令实施中发现旧版某项功能"从未工作过"，已改为其他实现方式。
     证据: 约第 73 行
     置信度: 高

146. 现象: 候选功能"cluster verify"被撤销，转而重新评估现有 cluster ready 命令补充明细的方案。
     证据: 约第 113 行
     置信度: 高

147. 现象: 现有 41 处 exit 相关断言需要逐批重新核对，暗示本次输出契约改造可能影响已有测试的正确性。
     证据: 约第 104 行
     置信度: 中

## docs/superpowers/specs/2026-07-25-capability-gap-research.md

148. 现象: 文档明确标注自身状态为"研究"而非规格文档，仅为后续规划讨论提供输入。
     证据: 约第 3 行
     置信度: 高

149. 现象: track_real_stats 是否可通过 SIGHUP 重载还是需要完全重启，属于"未验证"的未决事项。
     证据: 约第 88 行
     置信度: 高

150. 现象: sys_kwr 的保留期与相关 GUC 名称需要在目标实例上重新验证，不应直接采信网络搜索片段的结论。
     证据: 约第 99 行
     置信度: 高

151. 现象: sys_kwr 扩展在给定客户实例（包括本文所用测试 VM 本身）上是否安装"很可能未确认"。
     证据: 约第 100 行
     置信度: 中

152. 现象: 验证 sys_kwr 输出形状需要一台已安装该扩展的测试 VM，当前测试节点是否具备该条件"未确认"。
     证据: 约第 115 行
     置信度: 高

153. 现象: HypoPG 式索引建议功能的可行性依赖 hypopg（或 KingbaseES 等价物）扩展是否可用，这一前提尚未验证，若不可用该功能整体不成立。
     证据: 约第 117 行
     置信度: 高

## docs/superpowers/specs/2026-07-25-workload-requirements.md

154. 现象: 此前（2026-07-14 需求文档）已经否决过"trend 类"命令，本次需要专门论证当前方案"不算推翻旧决定"。
     证据: 约第 7 行
     置信度: 高

155. 现象: 2026-07-26 上机复核纠正了此前一处假设错误：kes-node2 并非"sys_kwr 未安装"的可用测试夹具。
     证据: 约第 101 行
     置信度: 高

156. 现象: 两条 WARN 分支（扩展可选装但未装、两条数据源都不满足）在当前测试环境下没有真实夹具可测，只能代码走查，不进黑盒回归套件。
     证据: 约第 106 行
     置信度: 高

157. 现象: kddm_* advisor 函数族的具体返回格式尚未探明，留待后续单独调研。
     证据: 约第 108-110 行
     置信度: 高

## docs/superpowers/specs/2026-07-25-workload-spec.md

158. 现象: 测试夹具 2026-07-26 重新核实后，发现与此前需求文档"grilled"讨论时的假设不同：kes-node2 不是"两个数据源都不可用"的夹具。
     证据: 约第 76 行
     置信度: 高

159. 现象: 两条"数据源不可用"的 WARN 分支没有真实测试夹具，只能代码评审验证，不纳入黑盒回归套件。
     证据: 约第 81 行
     置信度: 高

160. 现象: kddm_* advisor 函数族的输出形状尚未调研，留待 workload 上线后再进行后续调研。
     证据: 约第 106 行
     置信度: 高

## docs/superpowers/plans/2026-06-19-kbdiag-v2.md

161. 现象: 该文件标题为"kbdiag v2 Implementation Plan"（日期最早，06-19），随后次日文件标题为"v3"，再次日为"v1.4"，版本号命名顺序与产出时间顺序不一致。
     证据: 第 1 行
     置信度: 低

162. 现象: 全文任务清单（Task 1-5）的所有步骤复选框均为未勾选状态，Self-Review 也未标注实际执行结果。
     证据: 如第 65、71、132 行等
     置信度: 低

## docs/superpowers/plans/2026-06-20-kbdiag-v3.md

163. 现象: 该文件标题为"kbdiag v3 Implementation Plan"（06-20），次日（06-21）产出的文件标题却是"v1.4"，版本号回退。
     证据: 第 1 行
     置信度: 低

164. 现象: Global Constraints 声明"全部 PASS 才 commit"，但 Task 0a Step 6 说明 `test_status_quiet_hides_ok` 在当前阶段预期会 FAIL，"这是预期的"。
     证据: 约第 321 行
     置信度: 中

## docs/superpowers/plans/2026-06-21-kbdiag-v1.4.md

165. 现象: 该文件标题"kbdiag v1.4 Implementation Plan"，文件日期（06-21）晚于标题版本号更高的"v3"计划（06-20），版本号在时间上倒退。
     证据: 第 1 行；第 5 行
     置信度: 中

166. 现象: Task 4 Interfaces 描述 `cmd_advisor` 间接调用 `_idx_*`，而文末 Self-Review 明确写"not `_idx_*`"，两处描述互相矛盾。
     证据: 约第 1001 行 vs 约第 1611 行
     置信度: 中

## docs/superpowers/plans/2026-06-22-diagnose.md

167. 现象: Self-Review 表格引用"Task 2 `_diag_full` 系列函数"，但 Task 2 实际代码中并不存在名为 `_diag_full` 的函数。
     证据: 约第 1138 行 对照约第 441-538、617-625 行
     置信度: 中

## docs/superpowers/plans/2026-06-22-license.md

168. 现象: Task 3 Step 6 预先写明"如有失败修复后重跑 Step 1-2"，暗示预期首次执行可能不会一次性通过验收。
     证据: 约第 391 行
     置信度: 低

## docs/superpowers/plans/2026-06-22-output-headers.md

169. 现象: 该计划指出此前几乎所有 kbdiag 命令表格输出均因 `ksql_q()` 使用 `-t` 参数而缺少列标题，是横跨此前多份"已完成"计划的系统性 UX 缺陷。
     证据: 第 5-7 行
     置信度: 高

170. 现象: 新增测试断言使用 `assert_contains "$out" "X\|Y"` 写法意图"或"匹配，但 `assert_contains` 内部用 `grep -qF`（精确字符串匹配），不支持该语义。
     证据: 第 214、220、226、236、242 行等
     置信度: 中

171. 现象: Task 4 Step 4 把测试通过数固定为历史基线"previous count was 241"，作为后续回归判断参照。
     证据: 约第 495 行
     置信度: 低

## docs/superpowers/plans/2026-06-22-stmt.md

172. 现象: Task 1 Step 6 指示若 sys_stat_statements 未启用导致两个测试 FAIL，"先跳过这两个测试，继续后续 Task"。
     证据: 约第 284 行
     置信度: 高

173. 现象: Task 4 Step 1 测试函数第一版注释承认"`_pass` 可能已被上面的 assert 调用，需要确保测试计数正确"，随即改用更简单写法删除原有语句。
     证据: 约第 645 行
     置信度: 高

## docs/superpowers/plans/2026-06-22-team-rollout.md

174. 现象: 测试函数 `test_kbdiagrc_overrides_port` 的注释先描述测试目的是确认连接失败，紧接着第二行注释又说明"实际上我们只验证 .kbdiagrc 被 source"，验证范围被弱化。
     证据: 约第 37-38 行
     置信度: 高

## docs/superpowers/plans/2026-06-23-feature.md

175. 现象: 计划 Goal 部分将既有命令（`watch`、`stat`）直接称为"薄弱命令"。
     证据: 第 5 行
     置信度: 高

176. 现象: Task 3 新增测试注释明确说明不验证 Top SQL 数据是否正确，只要求"不崩溃"即算通过。
     证据: 约第 344-345 行
     置信度: 中

## docs/superpowers/plans/2026-06-23-platform.md

177. 现象: Task 2 Step 5 要求 shellcheck 零错误，处理办法之一是"行内抑制（仅用于误报）"，与"修复代码"并列为等价路径；本文件是项目第一次引入 shellcheck，说明此前所有 lib/*.sh 从未经过静态检查。
     证据: 约第 156-161 行
     置信度: 低

## docs/superpowers/plans/2026-06-23-quality.md

178. 现象: 计划 Goal 部分明确将此前的测试覆盖水平定性为"烟测"，目标是升级为"断言测试"。
     证据: 第 5 行
     置信度: 高

179. 现象: 新增测试用 `grep -i 'CREATE INDEX'` 过滤 `advisor index --fix` 输出，却断言该行以 `^DROP INDEX` 开头，过滤词与断言词不一致。
     证据: 约第 197-208 行
     置信度: 中

180. 现象: Task 8 将 `lib/cmd_logs.sh` 中硬编码的 `/tmp/kbdiag_logs.XXXXXX` 改为环境变量，但该硬编码未出现在最早描述 cmd_logs 实现的计划文件中，说明是计划外某次改动引入的。
     证据: 第 499-522 行 对照 2026-06-20-kbdiag-v3.md 中 cmd_logs() 实现
     置信度: 低

## docs/superpowers/plans/2026-07-08-dba-toolbox-loop.md

181. 现象: 迭代 0 记录"循环体内 ssh 会吃掉喂给 while read 的 heredoc stdin，导致只跑第一个命令就退出"，用 `</dev/null` 修复。
     证据: 约第 88-90 行
     置信度: 高

182. 现象: 迭代 2 记录 diagnose 测试套件在本轮改动前的基线就已存在 15 个失败用例（"存量环境问题"），排入任务队列第 8 项暂缓处理。
     证据: 约第 129-132 行
     置信度: 高

183. 现象: 迭代 2.5 记录 GitHub CI 自 07-06 起每次 push 都处于失败状态，由用户报障才触发修复，且"本地 shellcheck 过 ≠ CI 过，版本行为有差"。
     证据: 约第 140-142 行
     置信度: 高

184. 现象: 迭代 3 承认此前新增的 14 个 diagnose 分段测试存在设计缺陷——断言随被测环境健康状况漂移，产生假失败。
     证据: 约第 150-152 行
     置信度: 高

185. 现象: 迭代 3 挖出真实产品 bug：`_diag_render` 末行写法在 `--full` 模式下导致函数返回 1，在 `set -e` 下使命令以退出码 1 结束；原文称"本循环第三次踩同类 bug——已成头号地雷模式"。
     证据: 约第 157-160 行
     置信度: 高

186. 现象: 迭代 4 挖出 dispatch 存量坑：`shift 2` 在只给命令名时不生效，其他多参命令（如 kill）仅是"侥幸避开"同类问题。
     证据: 约第 174-178 行
     置信度: 高

187. 现象: 迭代 4 记录一处"perf 计时坑"：带引号/括号的命令经 SSH 字符串拼接打断计时表达式，曾测出 -600ms 负值。
     证据: 约第 179-180 行
     置信度: 高

188. 现象: 迭代 6"中途检查点"承认此前声明的"批次达上限、汇报后待用户定向"约定在执行中未被遵守，用户 60s 未响应即自行继续跑完。
     证据: 约第 205-209 行
     置信度: 高

189. 现象: 迭代 6"bug 2"明确统计"本会话第 4 次踩同一类坑"：`(( )) && ` 写法在 `set -e` 下导致脚本静默中断。
     证据: 约第 226-232 行
     置信度: 高

190. 现象: 迭代 6 记录一处 SQL bug：`_partition_list`/`_partition_children` 用 `ORDER BY 2` 引用了不存在的第 2 列，KingbaseES 报错。
     证据: 约第 221-225 行
     置信度: 高

191. 现象: 迭代 7 记录 `_audit_hba()` 首版因 NULL 拼接问题使 local 规则的 source 列错误显示成 `/`，事后才发现并修复。
     证据: 约第 248-249 行
     置信度: 高

192. 现象: 迭代 8 实锤一个存量真 bug：`_advisor_analyze` 因 relname 歧义每次执行都报错但被静默吞掉，"ANALYZE 建议自上线以来从未生效过"。
     证据: 约第 260-263 行
     置信度: 高

193. 现象: 迭代 8 确认 advisor 测试套件存量 3 个失败用例，根因是测试自身编写错误（过滤词/断言词不一致），与 2026-06-23-quality.md 中的同类问题相印证。
     证据: 约第 265-268 行
     置信度: 高

194. 现象: 迭代 8 结尾提到"spec 2026-07-04 的消重遗留清零"，该 spec 文档不在本次审阅的 12 份 plans 文件清单内（属于 specs 目录），说明 plans 与 specs 之间存在跨目录隐性依赖。
     证据: 约第 271 行
     置信度: 中

## .superpowers/sdd/progress-output-headers.md

195. 现象: Task1-4 都标注"review clean"，但同时各自附带若干条"minor nits"，未说明是否已处理。
     证据: 第 2-5 行
     置信度: 中

196. 现象: Task 5 被记录为"complete"且给出测试数与"pushed to main"，但目录内没有 task-5 对应的 brief 或 report 文件。
     证据: 第 6 行
     置信度: 高

197. 现象: Task1-4 只给出 commit 区间没有测试数，只有 Task5 给出"270/270"具体数字。
     证据: 第 2-6 行对比
     置信度: 低

## .superpowers/sdd/progress.md

198. 现象: 文件内先后记录了两套独立的编号体系（team-rollout 的 Task1-5 与 platform+quality+feature 的 T1-T8），并存于同一文件。
     证据: 第 1 行 与 第 8 行
     置信度: 高

199. 现象: team-rollout Task 3 记为 complete 但附带"review clean after fix"字样。
     证据: 第 4 行
     置信度: 中

200. 现象: Quality-T1 记录同样带有"after fix"字样。
     证据: 第 17 行
     置信度: 中

201. 现象: Feature-T3 记录同样带有"after fix"字样。
     证据: 第 29 行
     置信度: 中

202. 现象: Platform-T2 标注"review clean"，同时并列 2 条具体 Minor 问题，未说明是否已修复。
     证据: 第 13 行
     置信度: 中

203. 现象: Feature-T2 标注"review clean"，同时列出 2 条 Minor 问题，未说明是否已修复。
     证据: 第 28 行
     置信度: 中

204. 现象: Quality plan 记录 T1-T8 共 8 个子任务全部 COMPLETE，但目录中没有 Quality-T2 至 T7 各自独立的 brief 文件，只有一份合并报告。
     证据: 第 18-24 行 对照目录文件清单
     置信度: 高

205. 现象: Platform-T1、Platform-T3 在 progress.md 中记为 COMPLETE，但目录内没有对应的 task-brief/report 文件。
     证据: 第 12、14 行
     置信度: 高

## .superpowers/sdd/task-1-brief.md

206. 现象: 该文件标题"Task 1"内容是 watch 参数解析修复，同目录"task-1-report.md"标题也是"Task 1"但内容是 diagnose 测试扩展，两者不同。
     证据: 第 1 行 对照 task-1-report.md 标题
     置信度: 高

207. 现象: brief 中对 shellcheck 结果预期"若有误报，加行内注解抑制"，预先留出豁免空间。
     证据: 约第 99 行
     置信度: 低

## .superpowers/sdd/task-1-headers-report.md

208. 现象: 报告称"28 passed, 0 failed"，但部分新增测试（perf_slow/perf_top/perf_vacuum）只验证命令能运行不报错，并未验证表头是否存在。
     证据: 约第 53 行
     置信度: 高

209. 现象: 报告说明该修复只涉及有表格输出的查询，标量查询未改动。
     证据: 约第 52 行
     置信度: 低

## .superpowers/sdd/task-1-report.md

210. 现象: 最初实现（commits d140095, 2f10b8a）添加的 17 个测试被判定为"spec-ignorant and overly defensive"并被完全移除重做。
     证据: 约第 81-83 行
     置信度: 高

211. 现象: commit d140095 未出现在 progress.md 任何记录中，也未出现在任何 review diff 文件名中。
     证据: 第 55 行
     置信度: 中

212. 现象: 测试计数描述存在未解释的数字："9 original + 15 new = 24 total (was 9, became 26 with errors, now fixed)"，24 与 26 的关系未说明。
     证据: 约第 118 行
     置信度: 中

213. 现象: "Next Steps"写道测试"can now be executed"，暗示报告撰写时测试尚未在目标远程环境实际执行，只完成语法检查。
     证据: 第 71 行；约第 72-74 行
     置信度: 高

214. 现象: 同一份文件内先后记录了两个不同任务（diagnose Test Expansion 与 watch arg parsing fix），文件名只标注"task-1-report.md"未做区分。
     证据: 第 1 行 与 约第 133 行
     置信度: 高

## .superpowers/sdd/task-2-7-report.md

215. 现象: 报告标题为"Quality Tasks 2–7"，将 6 个子任务合并为一份报告，未见各任务独立的 brief 文件。
     证据: 第 1 行
     置信度: 高

216. 现象: 报告表格列出的 6 个 commit hash 均未出现在本目录任何单独的 review diff 文件名中，该区间只有一份合并 diff。
     证据: 第 9-14 行 对照目录 diff 文件清单
     置信度: 高

## .superpowers/sdd/task-2-brief.md

217. 现象: 该文件标题"Task 2"内容为 stat AWR 扩展，同目录"task-2-report.md"标题也是"Task 2"但内容是 GitHub Actions CI，两者完全不同。
     证据: 第 1 行 对照 task-2-report.md 标题
     置信度: 高

218. 现象: brief 将"锁获取 delta"列为三个目标段之一，但给出的实现代码只包含另外三个函数（等待事件/临时文件/checkpoints），没有锁计数相关函数或测试。
     证据: 第 1、9 行 对照 第 57-149 行
     置信度: 高

## .superpowers/sdd/task-2-report.md

219. 现象: `require_kingbase_user` 被修改为对 --help 等参数短路放行以让 CI 上的 smoke test 通过，暗示原权限检查逻辑与 CI 环境存在冲突。
     证据: 约第 40 行
     置信度: 高

220. 现象: "Concerns"部分指出 `cmd_logs.sh` 的 `--level` 选项被解析但从未被使用，声明这是修复之前就存在的遗留问题。
     证据: 第 55 行
     置信度: 高

## .superpowers/sdd/task-2-stat-awr-report.md

221. 现象: 该文件标题与内容对应 task-2-brief.md，但文件名改为"task-2-stat-awr-report.md"以与另一份同编号文件区分。
     证据: 文件名 对照 第 1 行标题
     置信度: 中

222. 现象: 报告"What was done"只提及三个函数，未提及 brief 中要求的"锁获取 delta"功能。
     证据: 第 16-19 行
     置信度: 高

## .superpowers/sdd/task-3-brief.md

223. 现象: brief 给出的 SQL 与打印格式包含 `calls, total_ms, mean_ms, query` 四列。
     证据: 第 38-51 行
     置信度: 高

224. 现象: brief 中的测试在两种分支下都直接调用 `_pass`，未对输出内容做实质性正向断言。
     证据: 第 16-24 行
     置信度: 高

## .superpowers/sdd/task-3-report.md

225. 现象: 初次实现（commit c58b96d）输出列缺少 mean_ms，总耗时单位是"秒"，与同任务 brief 目标列（单位毫秒）不一致，后续 commit 才修正。
     证据: 第 60-63 行 对照 第 83-85 行
     置信度: 高

226. 现象: "Fix 2"承认初版测试"只调用 `_pass`，从未验证无错误输出"，随后才补上断言。
     证据: 第 104 行
     置信度: 高

227. 现象: 同一报告文件内先后出现两个不同的"Commit Hash"标注（c58b96d 与 0012cc7）。
     证据: 第 69、125 行
     置信度: 中

## .superpowers/sdd/task-4-brief.md

228. 现象: brief 标题"Task 4"内容是 8 个命令的表头修复，同目录"task-4-report.md"标题同为"Task 4"但内容是 scripts/deploy.sh 批量部署，两者完全不同。
     证据: 第 1 行 对照 task-4-report.md 标题
     置信度: 高

229. 现象: brief 预期测试基线为 241，对应 headers-report 最终测试数为 270，相差 29，brief 未说明来源。
     证据: 第 87 行 对照 task-4-headers-report.md 第 7 行
     置信度: 低

## .superpowers/sdd/task-4-headers-report.md

230. 现象: "Concerns"承认 `|| echo 0` 写法"cosmetically inelegant but functionally correct"，承认代码质量不佳但功能正确，是已知但接受的技术债。
     证据: 第 33 行
     置信度: 高

231. 现象: "Concerns"对 `cmd_conf.sh` 默认输出是否显示表头给出假设性推断而非实际验证确认。
     证据: 第 34 行
     置信度: 中

## .superpowers/sdd/task-4-report.md

232. 现象: 部署测试"intentionally local/unit-style (no real SSH)"，明确说明未对真实远程主机进行测试。
     证据: 第 22-23 行
     置信度: 高

233. 现象: "Code Review Fixes (Post-Task 4)"记录了两个后续修复的问题，是在原任务标记"DONE"之后由代码评审发现并修复的。
     证据: 第 26-44 行
     置信度: 高

## .superpowers/sdd/task-8-brief.md

234. 现象: brief 第 6 步要求任务完成后执行 `git push`。
     证据: 第 52-54 行
     置信度: 高

## .superpowers/sdd/task-8-report.md

235. 现象: 报告结尾写明该 commit "Not pushed"，理由是"per task constraints"，与 brief 第 6 步要求的 git push 不一致，实际推迟到之后统一推送。
     证据: 第 36 行
     置信度: 高

## .superpowers/sdd/*.diff（结构性观察，未逐行读取内容）

236. 现象: 实际统计到的 review diff 文件数为 25 个，而非题面所述的 24 个。
     证据: `ls -la .superpowers/sdd/*.diff` 输出
     置信度: 高

237. 现象: diff 文件大小分布极不均匀，最小 104 字节/7 行，最大 121188 字节/3131 行，相差约三个数量级。
     证据: `ls -la` 与 `wc -l` 输出
     置信度: 高

238. 现象: 存在一个起止 commit 相同的 diff 文件 review-e1b36be..e1b36be.diff，内容仅 7 行。
     证据: 文件名；`wc -l` 显示 7 行
     置信度: 高

239. 现象: 存在两个以 db49007 为起点但终点不同的 diff 文件，其中 commit 2304add 未出现在 progress.md 或任何已读报告中。
     证据: 文件名列表；progress.md 全文搜索无 "2304add"
     置信度: 中

240. 现象: 存在两个以 505fd80 为起点但终点不同的 diff 文件（118 行与 154 行）。
     证据: 文件名列表；行数
     置信度: 高

241. 现象: commit 124dc75（文件名 review-6a930da..124dc75.diff）未出现在 progress.md 或其他已读报告中。
     证据: 文件名；progress.md 全文搜索无 "124dc75"
     置信度: 中

242. 现象: 存在两个体量明显大于其他 diff 的"跨全程"文件（2666 行与 3131 行）。
     证据: 文件名与行数；对照 progress 文件记录的起止 commit
     置信度: 中

243. 现象: progress.md 记录的 Quality-T2 至 T6 均未有各自独立对应的 review diff 文件，该区间只有一份合并 diff。
     证据: progress.md 第 18-23 行 对照 diff 文件名清单
     置信度: 高

244. 现象: task 编号（brief/report 文件名）不连续，目录中不存在 task-5、task-6、task-7 的独立文件，而 progress.md 中记录的对应任务均标记为 complete。
     证据: 目录 ls 清单 对照 progress.md 第 20-23 行、progress-output-headers.md 第 6 行
     置信度: 高

## lib/cmd_update.sh

245. 现象: EXIT trap 引用的局部变量 `tmp` 曾在 `set -u` 下被判定为 unbound variable，代码用 `${tmp:-}` 规避。
     证据: 约第 26-27 行；对应 git 提交 fce95aa
     置信度: 高

## .claude/agents/kbdiag-shell-dev.md

246. 现象: 文档列出两类 set -e/set -u 下的 shell 陷阱，并要求"每次修复这类问题都要配一个回归测试，防止再犯"。
     证据: 约第 34 行
     置信度: 高

## test/unit/test_status_port_mismatch.sh

247. 现象: cmd_status 曾只按 KB_DATA_DIR 匹配进程，未核对 KB_PORT 与实际监听端口，多实例主机上会出现"裸的"连接失败提示且不指出真实端口。
     证据: 约第 2-6 行
     置信度: 高

## test/unit/test_dispatch_sync.sh

248. 现象: kbdiag.sh（开发入口）曾与 build.sh（发布打包器）分发表不同步，导致 8 个命令通过 `bash kbdiag.sh <cmd>` 不可达，持续"未知的一段时间"才被人工审查发现（内部代号"R0 drift bug"）。
     证据: 约第 4-7 行
     置信度: 高

## .github/workflows/ci.yml

249. 现象: CI 的 shellcheck 步骤只对 `lib/*.sh` 运行，未覆盖 `kbdiag.sh`、`build.sh`、`scripts/*.sh`、`test/*.sh`。
     证据: 第 13 行
     置信度: 高

250. 现象: CI 的 "Unit tests" job 只运行 `test/unit/run.sh`，未调用覆盖全部 36 个命令模块的 `test/run_tests.sh`。
     证据: 第 19-20 行
     置信度: 高

## test/run_tests.sh

251. 现象: 集成测试套件运行前先 scp 部署到两个真实节点（kes-node1/kes-node2）再执行，依赖本地 Lima 虚拟机 + 真实数据库，而非 CI 环境。
     证据: 第 10-11 行
     置信度: 高

## test/perf_check.sh

252. 现象: 性能预算检查脚本同样依赖真实节点部署与 SSH 计时，且未被 `.github/workflows/ci.yml` 的任何 job 调用。
     证据: 第 20-21 行
     置信度: 高

---

总信号数：252 条
