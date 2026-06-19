# kbdiag

KingbaseES 命令行诊断工具。不进交互界面，直接输出实例状态、集群拓扑、复制延迟、性能瓶颈和存储空间，支持单机和主备集群（repmgr）。

## 安装

```bash
curl -fsSL https://github.com/Kevin-wenyu/kbdiag/releases/latest/download/kbdiag \
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

## 命令参考

| 命令 | 说明 |
|------|------|
| `status` | 进程、连通性、角色、运行时长 |
| `cluster` | repmgr 集群拓扑（单机自动跳过） |
| `replication` | primary 显示 standby 连接；standby 显示 WAL 延迟 |
| `sessions` | 非 idle 会话 |
| `locks` | 等待锁链 |
| `check` | 6 项健康阈值检查，exit code 0/1/2 |
| `perf [slow\|bloat\|vacuum\|index]` | 慢查询 / 表膨胀 / vacuum 状态 / 冗余索引 |
| `space` | 磁盘占用、表大小、WAL、归档 |
| `all` | 运行以上全部 |

所有命令支持 `-v` / `--verbose` 标志，展示底层完整数据（适合 DBA 故障排查）。

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
| `KB_WARN_DEAD_TUPLES` | `100000` | 单表 dead tuple 数 |
| `KB_SLOW_THRESHOLD` | `5` | 慢查询判定秒数 |

## 要求

- 以 `kingbase` OS 用户运行（`sudo -i -u kingbase`）
- KingbaseES V8R6+
- repmgr 可选（未安装时自动跳过 `cluster` 命令）