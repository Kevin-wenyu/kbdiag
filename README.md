# kbdiag

KingbaseES 命令行 DBA 专家工具箱（v1.4）。不进交互界面，直接输出实例状态、集群拓扑、复制延迟、性能瓶颈和存储空间；深度分析索引健康、列统计信息，提供优化建议。支持单机和主备集群（repmgr）。

## 安装

从源码直接获取打包文件（单文件，无依赖）：

```bash
curl -fsSL https://raw.githubusercontent.com/Kevin-wenyu/kbdiag/main/dist/kbdiag \
  -o /tmp/kbdiag && chmod +x /tmp/kbdiag
```

## 快速开始

```bash
# 切换到 kingbase 用户后运行
sudo -i -u kingbase

# 全量检查（日常巡检）
/tmp/kbdiag all

# 健康告警（可接入监控脚本）
/tmp/kbdiag check
echo $?   # 0=全OK  1=有WARN  2=有FAIL

# DBA 深度视图（加 -v 显示底层数据）
/tmp/kbdiag check -v
/tmp/kbdiag perf slow -v
```

## 全局参数

```
kbdiag [global-flags] <command> [subcommand] [command-flags]

  -v, --verbose       Show full underlying data
  -q, --quiet         Only show WARN/FAIL (suppress OK/INFO)
  -n N, --top N       Limit result rows (default: 10)
  --format text|json  Output format (default: text)
  --no-color          Disable ANSI colors
  --timeout N         DB query timeout seconds (default: 10)
```

## 命令参考

### [OPS] 运维命令（无需 DBA 背景）

| 命令 | 说明 |
|------|------|
| `status` | 进程、连通性、角色、运行时长（含授权摘要） |
| `license` | 授权有效期、是否正式授权、序列号、用户名称等 |
| `cluster` | repmgr 集群拓扑（单机自动跳过） |
| `replication` | primary 显示 standby 连接；standby 显示 WAL 延迟 |
| `check` | 14 项健康阈值检查，exit code 0=OK / 1=WARN / 2=FAIL |
| `space [frag]` | 磁盘占用、表大小、WAL、归档；`frag` 显示表碎片率 |

### [DBA] 深度诊断命令（需 DBA 解读）

| 命令 | 说明 |
|------|------|
| `sessions` | 非 idle 会话列表 |
| `locks [hold\|wait\|deadlock]` | 锁分析：持锁者 / 等待者 / 死锁检测 |
| `perf [slow\|bloat\|vacuum\|wait\|io\|wal\|top]` | 慢查询 / 表膨胀 / vacuum 状态 / 等待事件 / IO / WAL / Top SQL |
| `sql [pid\|all]` | 指定会话的 SQL 文本 + EXPLAIN |
| `wait` | 等待事件分布 |
| `progress` | 长时操作进度（VACUUM、CREATE INDEX 等） |
| `params [pattern]` | 实例参数查询，支持模式匹配 |
| `stat` | 吞吐量指标（TPS、buffer hit rate），支持 delta 采样 |
| `obj <schema.table>` | 对象深度分析（大小、索引、约束） |
| `temp` | 临时文件和排序溢出分析 |
| `watch <N> <cmd>` | 每 N 秒重复执行任意 kbdiag 命令 |
| `conf [diff]` | 配置审计；`diff` 对比多节点配置差异 |
| `audit` | 安全与合规检查 |
| `logs` | 日志文件分析（慢查询、错误） |
| `remote <nodes> <cmd>` | 多节点批量诊断 |
| `kill [--terminate] [pid\|--long N\|--idle-txn N] [--dry-run] [--force]` | 取消/终止查询或会话 |
| `idx [unused\|dup\|bloat\|missing]` | 索引健康分析 |
| `colstat <schema.table> [--col <col>]` | 列统计信息深度分析（n_distinct、相关性、MCV）|
| `advisor [index\|vacuum\|params\|analyze] [--fix]` | 综合优化建议；`--fix` 输出可执行 SQL |

### [ALL]

| 命令 | 说明 |
|------|------|
| `all` | 运行 status / cluster / replication / sessions / locks / check / perf / space |

## 环境变量

### 连接参数

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KB_PORT` | `54321` | 数据库端口 |
| `KB_BIN_DIR` | `/home/kingbase/cluster/install/kingbase/bin` | 二进制目录 |
| `KB_DATA_DIR` | `/home/kingbase/cluster/install/kingbase/data` | 数据目录 |
| `KB_REPMGR_CONF` | `/home/kingbase/cluster/install/kingbase/etc/repmgr.conf` | repmgr 配置文件 |
| `KB_SUPERUSER` | `system` | 数据库超级用户 |
| `KB_DB` | `test` | 连接数据库名 |

### 告警阈值

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KB_WARN_CONN` / `KB_FAIL_CONN` | `70` / `90` | 连接数占比 % |
| `KB_WARN_LAG` / `KB_FAIL_LAG` | `30` / `300` | 复制延迟秒数 |
| `KB_WARN_TXN` / `KB_FAIL_TXN` | `300` / `1800` | 长事务秒数 |
| `KB_WARN_OLDEST_TXN` / `KB_FAIL_OLDEST_TXN` | `3600` / `14400` | 最老活跃事务秒数 |
| `KB_WARN_XID` / `KB_FAIL_XID` | `1000000000` / `1900000000` | XID 年龄（事务 wraparound） |
| `KB_WARN_DEAD_TUPLES` | `100000` | 单表 dead tuple 数 |
| `KB_SLOW_THRESHOLD` | `5` | 慢查询判定秒数 |
| `KB_WARN_HIT` / `KB_FAIL_HIT` | `95` / `90` | buffer hit rate % 下限 |
| `KB_WARN_CKPT` / `KB_FAIL_CKPT` | `1` / `1` | checkpoint 警告阈值 |
| `KB_WARN_TEMP` / `KB_FAIL_TEMP` | `104857600` / `1073741824` | 临时文件大小字节（100MB / 1GB） |
| `KB_WARN_SLOT` / `KB_FAIL_SLOT` | `104857600` / `1073741824` | 复制槽积压字节（100MB / 1GB） |
| `KB_WARN_BGW` / `KB_FAIL_BGW` | `30` / `60` | bgwriter checkpoints 阈值 |

## 要求

- 以 `kingbase` OS 用户运行（`sudo -i -u kingbase`）
- KingbaseES V8R6+
- repmgr 可选（未安装时自动跳过 `cluster` 命令）
