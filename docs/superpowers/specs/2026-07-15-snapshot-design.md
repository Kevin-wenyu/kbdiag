# `snapshot` — 故障现场打包设计（R5）

需求来源：2026-07-14-next-features-requirements.md R5。定位：查层采集工具——只采集不判断，
供事后分析或提交原厂工单。**不是备份**。

## 接口

```
kbdiag snapshot [outfile.tar.gz]    # 默认 kbdiag-snapshot-<host>-<ts>.tar.gz
```

- exit 0=打包成功（哪怕部分 section 采集失败——故障现场就是要"能采多少采多少"），2=无法写出
- **DB 不可达时仍然工作**：SQL 类 section 记录错误占位，日志尾部与元数据照常打包（这正是最需要快照的时刻）
- `--format json`：输出文件路径 + 各 section 采集状态

## 包内容（目录 = kbdiag-snapshot-<host>-<ts>/）

| 文件 | 来源 | 为什么 |
|------|------|--------|
| metadata.txt | hostname/时间/kbdiag 版本/DB 版本/角色 | 工单基本信息 |
| status.txt / check.txt | cmd_status / cmd_check --os | 现场总体状态 |
| sessions.txt / locks.txt / wait.txt | cmd_sessions / cmd_locks / cmd_wait | 易失数据，事后无法重现 |
| perf.txt / progress.txt / temp.txt / stat.txt | 对应命令 | 性能现场 |
| replication.txt / cluster.txt | cmd_replication / cmd_cluster ready | 主备现场 |
| params.txt | sys_settings 全量 | 复现环境配置 |
| log_tail.txt | 最新日志末 2000 行 | 错误上下文 |

## 脱敏（默认开启，对抗审查结论落实）

所有捕获文本统一做字符串字面量掩码：`'...'` → `'***'`（日志和 SQL 文本里的业务数据主要藏在
字符串字面量里；诊断需要的是结构不是数据）。代价：极少数含引号的配置值同被掩码，可接受。
需要原文时用对应单命令现场查看。

## 实现要点

- mktemp -d 工作目录 + trap 清理；NO_COLOR 纯文本；每 section `|| true` 不因单项失败中断
- 日志发现：从 cmd_logs 抽出 `_logs_latest_file()` 复用（消重惯例）
- 全量参数直接 ksql 查 sys_settings（非 cmd_params 的 pattern 过滤视图）
- tar czf 后打印文件路径 + 大小；提示"snapshot is not a backup"

## 测试

test_snapshot.sh：生成成功且 tar 可解、含全部预期文件、log_tail 无未掩码字面量抽查、
metadata 含版本与角色、--format json 合法、standby 可运行、自定义路径生效。
