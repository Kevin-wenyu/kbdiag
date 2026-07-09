# kbdiag 功能测试报告

- **日期**: 2026-07-09
- **被测版本**: kbdiag v1.0.0-56-gaebb670（main 分支，含当日 audit 白名单 + 消重两次提交）
- **测试方式**: `test/run_tests.sh` 全量自动化套件（本地 Mac 编排，SSH 驱动 VM 真实执行）+ `test/perf_check.sh` 性能预算门禁

## 测试环境

| 项 | 值 |
|----|-----|
| 数据库 | KingbaseES V008R006C009B0014 (x86_64, gcc 4.8.5) |
| 拓扑 | repmgr 一主一备：node1 primary (192.168.105.10) / node2 standby (192.168.105.11) |
| 复制状态 | 双节点 running，复制延迟 0 bytes |
| 运行身份 | kingbase OS 用户，本地 socket 免密连接（ksql test system，端口 54321） |

## 总体结论

**349 个用例，347 通过，2 失败，通过率 99.4%。** 32 个命令套件中 30 个全绿。2 个失败均为同一类**已知功能缺口**（`sessions`/`replication` 不支持 JSON 输出，详见下文），非回归。性能门禁 25 项全部在预算内。

## 分套件结果

| 套件 | 通过/总数 | 套件 | 通过/总数 |
|------|-----------|------|-----------|
| advisor | 20/20 | obj | 13/13 |
| audit | 10/10 | params | 10/10 |
| backup | 7/7 | partition | 4/4 |
| check | 13/13 | perf | 28/28 |
| colstat | 13/13 | progress | 7/7 |
| conf | 8/8 | remote | 7/7 |
| deploy | 4/4 | **replication** | **7/8** |
| diagnose | 34/34 | **sessions** | **7/8** |
| explain | 9/9 | space | 11/11 |
| idx | 13/13 | sql | 10/10 |
| jobs | 4/4 | stat | 15/15 |
| kill | 9/9 | status | 6/6 |
| license | 9/9 | stmt | 13/13 |
| locks | 11/11 | temp | 8/8 |
| logs | 10/10 | update | 7/7 |
| 三层覆盖 | | watch | 14/14 |

三层命令设计（看/查/断）全部有套件覆盖：OPS 层 7 命令、DBA 层 10 命令、根因层 2 命令（diagnose 34 用例、advisor 20 用例为最重的两个套件）。

## 失败项分析

**`test_sessions_json_valid`、`test_replication_json_valid`（2 个）**

- 现象：`kbdiag --format json sessions` 输出的仍是表格文本，`--format json replication` 输出为空，均非合法 JSON。
- 根因：这两个命令未接入 `OUTPUT_FMT` 分支——JSON 输出目前只有 8 个命令实现：`advisor` `check` `colstat` `diagnose` `idx` `kill` `license` `stmt`。测试用例是按目标行为写的（期望所有常用命令支持 `--format json`），实现尚未跟上。
- 定性：**功能缺口，非回归**（与当日改动无关的存量状态）。
- 建议：要么给 `sessions`/`replication` 补 JSON 输出（复用 core.sh 的 json helpers，工作量小），要么明确 JSON 支持范围并删掉这两个用例。倾向前者——这两个命令的输出天然结构化，是被监控系统消费概率最高的两个。

## 性能门禁（node1，全部通过）

| 命令 | 实测 (ms) | 预算 (s) | 命令 | 实测 (ms) | 预算 (s) |
|------|----------:|---------:|------|----------:|---------:|
| status | 482 | 10 | jobs | 320 | 10 |
| license | 365 | 10 | partition | 661 | 10 |
| cluster | 432 | 15 | **stat** | **10309** | **20** |
| replication | -298* | 10 | stmt | 697 | 15 |
| check | 2263 | 30 | explain | 252 | 10 |
| space | 420 | 15 | temp | 391 | 10 |
| backup | 847 | 15 | idx | 800 | 20 |
| params | 214 | 10 | conf | 232 | 10 |
| sessions | 312 | 10 | audit | 1755 | 20 |
| locks | 235 | 10 | logs | 982 | 30 |
| perf | 1045 | 30 | diagnose | 1386 | 30 |
| wait | 319 | 10 | advisor | 1465 | 30 |
| progress | 391 | 10 | | | |

\* `replication -298ms` 为 perf_check.sh 跨秒计时的测量瑕疵（实际亚秒级完成），不影响门禁判定，后续可改用毫秒级时间戳消除。

值得关注：**`stat` 10.3s 已用掉预算的一半**，是全部命令中最慢的（瓶颈在 Top SQL 段的 sys_stat_statements 聚合），其余命令都在 2.3s 以内。若未来预算收紧或生产库 statements 量大，stat 是第一优化对象。

## 当日新增功能验证（迭代 7/8）

- `audit` 连接来源白名单（sys_hba_file_rules 四类风险分析）：3 个新用例通过，测试环境的 `local trust` 与 `0.0.0.0/0` 全开放规则均被正确标记并给出原因。
- 消重后的 `perf wal` / `stat` checkpoint 段 / `advisor analyze` / `colstat` drift：全部套件绿，advisor 的 ANALYZE 建议在修复 relname 歧义后首次真正可用。

## 遗留问题清单

| # | 问题 | 严重度 | 建议 |
|---|------|--------|------|
| 1 | `sessions`/`replication` 不支持 `--format json` | 低（功能缺口） | 补实现，复用 json helpers |
| 2 | perf_check.sh 计时可能出现负值 | 低（工具瑕疵） | 改毫秒级时间戳 |
| 3 | `stat` 耗时 10.3s，距预算最近 | 低（性能余量） | 优化 Top SQL 段或单独设预算 |
