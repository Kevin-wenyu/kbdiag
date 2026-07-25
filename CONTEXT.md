# kbdiag

KingbaseES 命令行诊断工具的领域术语表。命令按"看/查/断"三层组织，见 CLAUDE.md 的"三层命令设计哲学"。

## Language

**Workload**:
一段时间区间内的负载变化报告（等待事件、SQL 耗时、指标趋势的 diff），而非某一时刻的快照。底层可能来自 `sys_kwr` 扩展或 `sys_stat_sysmetric_history`（取决于目标库装了什么），命令层的叫法与具体实现解耦。
_Avoid_: History（太泛，与日志/慢 SQL 历史混淆）、AWR/KWR（绑死具体实现）
