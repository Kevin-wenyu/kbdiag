# `cluster ready` — failover 就绪检查设计（R1）

需求来源：2026-07-14-next-features-requirements.md R1（spec 2026-07-04 Phase 4 遗留）。

## 接口

```
kbdiag cluster            # 现状不变：repmgr cluster show
kbdiag cluster ready      # failover 就绪 checklist，exit 0=OK 1=WARN 2=FAIL
```

`--json` 支持（json_begin "cluster_ready" + json_item）。repmgr 缺失时降级：warn standalone，exit 0。

## VM 实测结论（2026-07-14，kes-node1/2，V8R6 DG 模式）

- `repmgr.nodes`（esrep 库）跨节点复制可查；esrep 用户免密可直连 peer（`ksql -h <peer> -U esrep -d esrep`），比 conf diff 的 SSH 路径轻
- esrep 库是 PG 模式：布尔返回 `t`/`f`（test 库是 `true`/`false`），比较需同时兼容
- `repmgr daemon status --csv`：field3=role, field5=repmgrd running(1/0), field7=paused(1/0)
- `pg_is_wal_replay_paused()` 在 KingbaseES 存在（保留 pg_ 前缀）
- KingbaseES repmgr 扩展参数：`ha_running_mode='DG'`、`synchronous='quorum'`、`trusted_servers`（网关仲裁，替代 witness）、`virtual_ip`、`auto_cluster_recovery_level`
- 实测现网即发现真实问题：node1 归档积压 51 个 .ready 超 WARN 阈值——验证了该项检查的价值

## 检查清单（10 项）

| # | 项 | 数据源 | 判定 |
|---|----|--------|------|
| 1 | topology | repmgr.nodes | 恰好 1 primary，全部 active=t，否则 FAIL/WARN |
| 2 | promotion_candidate | repmgr.nodes | 所有 standby priority=0 → FAIL（无可提升节点）|
| 3 | arbitration | nodes + repmgr.conf | 2 节点无 witness 时 trusted_servers 未配 → WARN（脑裂仲裁缺失）|
| 4 | repmgrd | daemon status --csv | 任一节点未运行 → FAIL；paused → FAIL（自动 failover 已失效）|
| 5 | failover_mode | repmgr.conf | failover≠automatic → WARN；promote_command 缺失 → FAIL |
| 6 | standby_attached | sys_stat_replication vs nodes | 挂接数 < standby 数 → FAIL；非 streaming → WARN |
| 7 | slots | sys_replication_slots | inactive slot → WARN（WAL 滞留风险）|
| 8 | archive_backlog | 复用 _backup_pending_wal | 阈值 KB_WARN/FAIL_WAL_READY（10/100）|
| 9 | standby_readiness | ksql -h peer（esrep）| 不可达 → FAIL；hot_standby≠on → FAIL；recovery_min_apply_delay≠0 → WARN；replay paused → FAIL |
| 10 | vip | repmgr.conf + ip addr | 配了 virtual_ip：primary 上未绑 → FAIL；standby 上绑了 → FAIL（VIP 冲突）；未配 → SKIP |

另输出 INFO：synchronous 模式、最近 7 天 failover/断连事件数（repmgr.events）。

## 设计取舍

- **不包 `repmgr node check`**：其检查项（slots/archive_ready/downstream）与本清单重叠，但输出格式不稳定且逐项判定阈值不可控；自查 SQL 可控且可 JSON 化。`cluster ready` 输出尾部提示可用 `repmgr node check` 交叉验证（断层→查层的验证路径惯例）
- **peer 访问走 esrep SQL 而非 SSH**：repmgr 已保证 esrep 免密连接是集群工作的前提，不可达本身就是 FAIL 信号；SSH 是 conf diff 的历史路径，不新增依赖
- **单点执行**：在任一节点跑，检查全集群（nodes 表 + daemon status 天然全局，standby_readiness 逐个 standby 远程探测）
- **exit code 语义与 check 一致**（0/1/2），dispatch 用 `|| exit $?`

## 测试

新增 test/cases/test_cluster.sh：show 基线、ready 在 node1/node2 各跑一次（断言含 10 项关键词、exit∈{0,1}）、--json 可解析、JSON 与文本模式判定一致。
