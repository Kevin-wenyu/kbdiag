# repl：同步复制被 repmgr 降级成异步

**状态**：active（用户 2026-10-07 选"加，报 WARN"；第 3 节选 A：读 repmgr.conf）

## 1. 事实（2026-10-07 node1 实采）

- repmgr.conf（`$bindir/../etc/repmgr.conf`，repmgrd 和 kbha 都用 `-f` 指向它）写的是 `synchronous='quorum'`。
- repmgrd 降级和恢复的方式：用 ALTER SYSTEM 改 `synchronous_standby_names`，所以这个参数的来源是 `kingbase.auto.conf`；`synchronous_commit=remote_apply` 在 `es_rep.conf` 里，不会变。
- hamgr.log：备库断开时 `SET synchronous TO "async"`（名单从 `ANY 1( node2)` 改成 `""`），**备库重连的同一秒** `SET synchronous TO "quorum"`，名单改回来。日志里一共 80 次切换，每次都是这样成对出现。
- 数据库里查不到"本来应该是同步"：esrep 的 `repmgr.conf` 表只存了 `sys_bindir`；`repmgr.events` 有 `child_node_disconnect`/`child_node_reconnect`，但没有"改了同步设置"的事件。
- 所以"已降级"恰好就是"同步备库断开"的那段时间。这段时间里已经会报出来的有：主库的 `cluster.detached`（WARN，inj-slot 实采到了）、`repl` 的 downstreams 是 0、备库上的 `inst.upstream`（WARN）。

## 2. 场景表（草稿）

| # | 场景 | 现在怎么看 | 缺什么 |
|---|---|---|---|
| SD1 | 主库上：提交现在还有没有同步副本保护 | `repl` 显示 `(none: asynchronous only)`，没有 finding | 没说清楚后果：此刻故障切换会丢已提交的事务 |
| SD2 | 为什么变成异步了：是本来就配成异步，还是被 repmgr 降级了 | 分不出来 | 要知道 repmgr 配的是什么（只在 repmgr.conf 里） |
| SD3 | 备库回来以后同步恢复了没有（repmgrd 挂了就不会恢复） | 分不出来：备库在 streaming，名单却一直是空的 | 同 SD2 |

不归它：备库断开本身 → `cluster.detached`、备库上的 `status`；落后多少 → `repl` 现有的展示。

## 3. 需要拍板：怎么判"降级了"

- **A 读 repmgr.conf（本机运行时）**：在 `cluster` 里做（只有它连 esrep，能从 `repmgr.conf` 表拿到 `sys_bindir`），读 `$bindir/../etc/repmgr.conf` 的 `synchronous`；配的是 sync/quorum 而名单是空的，就报 `cluster.sync_degraded` WARN。三个场景都能回答，SD3（repmgrd 没恢复）是唯一一个现在完全看不到的情况。代价是第三个"不是 SQL"的例外（前两个是 statfs）：远程运行或文件读不到时不判。
- **B 不读文件，只把后果补进已有的 finding**：主库的 `cluster.detached` 在名单是空的时候，symptom 加一句"repmgr 已把提交降成异步，此刻故障切换会丢已提交的事务"；`repl` 的文本也补这一句。不新增 finding id。能回答 SD1，回答不了 SD2、SD3。
- **C 不做**：`cluster.detached` 已经覆盖了这段时间。

**我的判断是 A**：SD3（备库回来了、repmgrd 却没恢复同步）是唯一一种现在没有任何 finding 会报、又会悄悄丢数据的状态，只有读配置才能看出来；读文件按 `inst.disk` 那套本机判断来，属于一处受控的例外。

## 4. 阶段（拍板后再细化）

1. 实现（按选定的方案）加 L1/L2 测试；
2. VM 验证：slot 注入看降级；SD3 要停掉 repmgrd 再让备库重连，造"没恢复"，造之前先问用户（会动集群守护进程）；
3. 审查、文档、kbdiag-docs 页面。

## 5. 进度

- 2026-10-07：写成，等用户拍板第 3 节。
- 2026-10-07：用户选 A（读 repmgr.conf，在 cluster 里报 `cluster.sync_degraded` WARN），状态 active。
- 2026-10-07：实现完成（未提交）：probe `cluster.sync`、rule `cluster.sync_degraded`、文本 `synchronous` 段、PRD §5.2 登记、README、CLAUDE.md、queries.md。单元测试：parser、本机判断、文件核对、14 条 rule 用例、6 个 golden、fuzz。两节点完整 e2e 通过；`slot.sh` 注入时实采到 `cluster.detached` + `cluster.sync_degraded`。VM 上暴露的两个问题已修：KES 清空后返回空串而不是 NULL；备库刚回来时 repmgrd 还没切回来的窗口（措辞改成"还没切回来"，`slot.sh` 的 down 等名单恢复）。下一步：code-reviewer 审查（进行中）→ Jev 分诊 → 修 → 提交 → kbdiag-docs cluster 页。SD3（repmgrd 没切回来）要停 repmgrd 才能在 VM 上造，没做，等用户决定。
- 2026-10-07：审查 8 条处理完（见 chronicle），用户定读不到文件保持 UNKNOWN；两节点 e2e 通过，提交。剩下：kbdiag-docs cluster 页；SD3 的 VM 实测等用户决定。
- 2026-10-07：kbdiag-docs cluster 页已更新（v2）。计划里的工作都做完了；SD3 的 VM 实测等用户决定，做不做都不挡发布。
